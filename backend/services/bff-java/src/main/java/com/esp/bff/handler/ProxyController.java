package com.esp.bff.handler;

import com.esp.bff.edge.RouteTable;
import jakarta.servlet.http.HttpServletRequest;
import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpMethod;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.client.HttpStatusCodeException;
import org.springframework.web.client.RestClient;

import java.net.URI;
import java.util.Enumeration;
import java.util.List;

/**
 * 通用业务代理(P0-3 流量收敛断链修复):traefik ingress 把 /api/** 全部指向
 * bff-java:8090(Spring MVC),但 8090 只有 auth 控制器,业务路径 404;
 * Netty edge 9090 的 RouteTable 转发是 fire-and-forget(响应从不回写),不可用作入口。
 *
 * <p>本控制器补齐 8090 侧代理:任意方法/路径透传到 RouteTable 解析的下游服务,
 * AuthFilter 已校验 JWT 并经 wrapper 注入可信 X-User-Id/X-Role(wrapper 的
 * getHeader/getHeaders 会被 RestClient 读到并透传,外网伪造值已在 Filter 层被覆盖)。
 *
 * <p>同步转发(阻塞一请求一线程,单节点 5000 CCU 由 Netty edge 层消化大并发;
 * 本层服务于 ingress REST 路径,延迟与下游对齐,10s 超时兜底)。
 */
@RestController
public class ProxyController {

    /** 跳过的 hop-by-hop / 会引发下游连接复用问题的头。 */
    private static final List<String> SKIP_HEADERS = List.of(
            "host", "content-length", "connection", "transfer-encoding", "keep-alive",
            "proxy-authorization", "proxy-authenticate", "te", "trailer", "upgrade");

    private final RestClient rest;

    public ProxyController() {
        // 自建实例:容器无 RestClient.Builder bean(bff 的 autoconfig 排除项较多,不依赖注入)。
        this.rest = RestClient.create();
    }

    @RequestMapping("/api/**")
    public ResponseEntity<byte[]> proxy(HttpServletRequest req,
                                        @RequestBody(required = false) byte[] body,
                                        HttpMethod method) {
        String path = req.getRequestURI();
        String query = req.getQueryString();
        String downstream = RouteTable.resolve(path);
        if (downstream == null) {
            // /api/auth/** 等本应由自身控制器处理的路径不会到这里(SpecificController 优先);
            // 兜底 404。
            return ResponseEntity.notFound().build();
        }
        String url = "http://" + downstream + path + (query == null ? "" : "?" + query);

        HttpHeaders headers = new HttpHeaders();
        Enumeration<String> names = req.getHeaderNames();
        while (names.hasMoreElements()) {
            String name = names.nextElement();
            if (SKIP_HEADERS.stream().anyMatch(name::equalsIgnoreCase)) continue;
            Enumeration<String> values = req.getHeaders(name);
            while (values.hasMoreElements()) {
                headers.add(name, values.nextElement());
            }
        }
        if (headers.getContentType() == null && body != null && body.length > 0) {
            headers.setContentType(MediaType.APPLICATION_JSON);
        }

        try {
            // RestClient 无 RestTemplate 式 exchange(URI,method,entity,class) 重载;
            // 用 fluent API,4xx/5xx 由 retrieve() 抛 HttpStatusCodeException 子类,
            // 与下方 catch 语义对齐。GET/DELETE 等无 body 方法不能调 .body(null)(NPE),走无 body 分支。
            var spec = rest.method(method)
                    .uri(URI.create(url))
                    .headers(h -> h.addAll(headers));
            org.springframework.web.client.RestClient.RequestBodySpec bodySpec =
                    (org.springframework.web.client.RestClient.RequestBodySpec) spec;
            ResponseEntity<byte[]> resp = (body == null || body.length == 0
                    ? bodySpec.retrieve().toEntity(byte[].class)
                    : bodySpec.body(body).retrieve().toEntity(byte[].class));
            return ResponseEntity.status(resp.getStatusCode())
                    .headers(filterResponseHeaders(resp.getHeaders()))
                    .body(resp.getBody());
        } catch (HttpStatusCodeException e) {
            // 下游 4xx/5xx 原样透传(保留幂等 409、限流 429 语义)。
            return ResponseEntity.status(e.getStatusCode())
                    .headers(filterResponseHeaders(e.getResponseHeaders()))
                    .body(e.getResponseBodyAsByteArray());
        }
        // 其余异常(连接失败/超时等 RestClientException)交由 GlobalExceptionHandler → 503 envelope,
        // 替代原内联 catch(Exception) → 503(架构审查 Top5 #5 统一错误处理)。
    }

    private static HttpHeaders filterResponseHeaders(HttpHeaders in) {
        HttpHeaders out = new HttpHeaders();
        if (in == null) return out;
        in.forEach((name, values) -> {
            if (SKIP_HEADERS.stream().anyMatch(name::equalsIgnoreCase)) return;
            values.forEach(v -> out.add(name, v));
        });
        return out;
    }
}
