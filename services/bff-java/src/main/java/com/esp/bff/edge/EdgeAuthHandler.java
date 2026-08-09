package com.esp.bff.edge;

import com.esp.bff.auth.JwtService;
import io.jsonwebtoken.Claims;
import io.netty.channel.ChannelFutureListener;
import io.netty.channel.ChannelHandlerContext;
import io.netty.channel.SimpleChannelInboundHandler;
import io.netty.handler.codec.http.FullHttpRequest;
import io.netty.handler.codec.http.HttpHeaderNames;
import io.netty.handler.codec.http.HttpHeaders;
import io.netty.handler.codec.http.HttpResponseStatus;
import io.netty.util.ReferenceCountUtil;
import io.opentelemetry.api.GlobalOpenTelemetry;
import io.opentelemetry.api.trace.Span;
import io.opentelemetry.api.trace.StatusCode;
import io.opentelemetry.api.trace.Tracer;
import io.opentelemetry.context.Context;
import io.opentelemetry.context.propagation.TextMapGetter;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.nio.charset.StandardCharsets;
import java.util.List;

/**
 * Netty 边缘鉴权处理器(ADR-12 安全,替代 SCG GlobalFilter)。
 * 职责:解析 HTTP 请求 → JWT 校验 → 注入可信 X-User-Id/X-Role → 交由转发处理器。
 * - 白名单:/api/auth/、/healthz、/actuator/。
 * - 剥离外网伪造的 X-User-Id/X-Role,由 BFF 注入可信值。
 * - 校验失败 401。
 *
 * traceId 贯通(P1-5):
 * - 从入站 HTTP header 提取 W3C traceparent 作为 parent span。
 * - 创建 bff.edge.request server span,makeCurrent() 使下游能关联。
 * - 将 OTel Context 存入 InletEvent,DownstreamForwarder 用此注入下游 header。
 *
 * 高吞吐路径:鉴权后的请求投递到 Disruptor 入口队列(见 InletDisruptor),
 * 由消费者异步转发下游,Netty worker 线程不阻塞在下游 IO 上。
 */
public class EdgeAuthHandler extends SimpleChannelInboundHandler<FullHttpRequest> {

    private static final Logger log = LoggerFactory.getLogger(EdgeAuthHandler.class);
    // 放行:认证、健康、actuator、API 门户(springdoc /v3/api-docs + /docs 聚合 Swagger UI +
    // /swagger-ui 静态资源 + /openapi-go-rust.yaml Go/Rust 手写清单)。门户为只读 API 文档,无敏感数据;
    // 实际业务接口仍受本 Handler 保护。与 AuthFilter 白名单保持一致。
    private static final List<String> WHITELIST = List.of(
            "/api/auth/", "/healthz", "/actuator/", "/.well-known/",
            "/v3/api-docs", "/docs", "/swagger-ui", "/openapi-go-rust.yaml");

    private static final Tracer TRACER = GlobalOpenTelemetry.getTracer("bff-edge");

    /** W3C traceparent 从 Netty HttpHeaders 提取的 getter。 */
    private static final TextMapGetter<HttpHeaders> HEADER_GETTER = new TextMapGetter<>() {
        @Override
        public Iterable<String> keys(HttpHeaders carrier) {
            return carrier.names();
        }
        @Override
        public String get(HttpHeaders carrier, String key) {
            if (carrier == null) return null;
            return carrier.get(key);
        }
    };

    private final JwtService jwt;
    private final InletDisruptor inlet;

    public EdgeAuthHandler(JwtService jwt, InletDisruptor inlet) {
        this.jwt = jwt;
        this.inlet = inlet;
    }

    @Override
    protected void channelRead0(ChannelHandlerContext ctx, FullHttpRequest req) {
        String path = req.uri();
        boolean local = RouteTable.isLocal(path);

        // === traceId 贯通:提取上游 W3C traceparent,创建 bff.edge.request span ===
        Context extractedContext = GlobalOpenTelemetry.getPropagators()
                .getTextMapPropagator()
                .extract(Context.current(), req.headers(), HEADER_GETTER);
        Span edgeSpan = TRACER.spanBuilder("bff.edge.request")
                .setParent(extractedContext)
                .startSpan();
        Context spanContext = extractedContext.with(edgeSpan);

        try (io.opentelemetry.context.Scope scope = spanContext.makeCurrent()) {
            edgeSpan.setAttribute("http.method", req.method().name());
            edgeSpan.setAttribute("http.url", req.uri());

            // 白名单 / 本地处理:不校验 JWT,但务必剥离伪造的 X-User-Id/X-Role
            if (local || isWhitelisted(path)) {
                stripSpoofedHeaders(req);
                forward(ctx, req, local, spanContext);
                return;
            }

            String auth = req.headers().get(HttpHeaderNames.AUTHORIZATION);
            if (auth == null || !auth.startsWith("Bearer ")) {
                edgeSpan.setStatus(StatusCode.ERROR, "missing/invalid authorization");
                respond401(ctx);
                ReferenceCountUtil.release(req);
                return;
            }
            try {
                Claims c = jwt.parse(auth.substring(7));
                String uid = c.getSubject();
                String role = c.get("role", String.class);
                stripSpoofedHeaders(req);
                req.headers().set("X-User-Id", uid);
                req.headers().set("X-Role", role == null ? "student" : role);
                forward(ctx, req, local, spanContext);
            } catch (Exception e) {
                log.debug("JWT parse failed: {}", e.getMessage());
                edgeSpan.setStatus(StatusCode.ERROR, "jwt parse failed");
                respond401(ctx);
                ReferenceCountUtil.release(req);
            }
        } finally {
            edgeSpan.end();
        }
    }

    /** 投递到 Disruptor 入口队列异步转发;队列满时快速失败 503(背压保护)。 */
    private void forward(ChannelHandlerContext ctx, FullHttpRequest req, boolean local, Context otelContext) {
        final String channelId = ctx.channel().id().asLongText();
        final FullHttpRequest retained = req.retain();
        // 构造负载事件:DisruptorWorker 内部把字段拷进预分配复用槽(translator)。
        InletEvent payload = new InletEvent();
        payload.set(channelId, retained, otelContext);
        boolean offered = inlet.publish(payload);
        if (!offered) {
            ReferenceCountUtil.release(retained);
            respond503(ctx);
        }
    }

    private boolean isWhitelisted(String path) {
        for (String w : WHITELIST) if (path.startsWith(w)) return true;
        return false;
    }

    /** 剥离外网伪造的 X-User-Id/X-Role,确保只有 BFF 注入的值可信。 */
    private void stripSpoofedHeaders(FullHttpRequest req) {
        HttpHeaders h = req.headers();
        h.remove("X-User-Id");
        h.remove("X-Role");
    }

    private void respond401(ChannelHandlerContext ctx) {
        io.netty.handler.codec.http.DefaultFullHttpResponse resp =
                new io.netty.handler.codec.http.DefaultFullHttpResponse(
                        io.netty.handler.codec.http.HttpVersion.HTTP_1_1,
                        HttpResponseStatus.UNAUTHORIZED,
                        ctx.alloc().buffer().writeBytes("unauthorized".getBytes(StandardCharsets.UTF_8)));
        resp.headers().set(HttpHeaderNames.CONTENT_TYPE, "text/plain");
        ctx.writeAndFlush(resp).addListener(ChannelFutureListener.CLOSE);
    }

    private void respond503(ChannelHandlerContext ctx) {
        io.netty.handler.codec.http.DefaultFullHttpResponse resp =
                new io.netty.handler.codec.http.DefaultFullHttpResponse(
                        io.netty.handler.codec.http.HttpVersion.HTTP_1_1,
                        HttpResponseStatus.SERVICE_UNAVAILABLE,
                        ctx.alloc().buffer().writeBytes("inlet full".getBytes(StandardCharsets.UTF_8)));
        resp.headers().set(HttpHeaderNames.CONTENT_TYPE, "text/plain");
        ctx.writeAndFlush(resp).addListener(ChannelFutureListener.CLOSE);
    }

    @Override
    public void exceptionCaught(ChannelHandlerContext ctx, Throwable cause) {
        log.warn("edge handler error", cause);
        ctx.close();
    }
}
