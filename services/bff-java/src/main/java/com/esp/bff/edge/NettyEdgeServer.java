package com.esp.bff.edge;

import com.esp.bff.auth.JwtService;
import io.netty.bootstrap.ServerBootstrap;
import io.netty.channel.ChannelInitializer;
import io.netty.channel.ChannelOption;
import io.netty.channel.EventLoopGroup;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.SocketChannel;
import io.netty.channel.socket.nio.NioServerSocketChannel;
import io.netty.handler.codec.http.HttpObjectAggregator;
import io.netty.handler.codec.http.HttpServerCodec;
import jakarta.annotation.PostConstruct;
import jakarta.annotation.PreDestroy;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

/**
 * Netty 边缘 HTTP 接入服务器(ADR-08/ADR-13,替代 Spring Cloud Gateway)。
 * - 监听边缘端口(默认 8090),独立于 Spring MVC 的容器端口。
 * - 接入 pipeline:HttpServerCodec → 聚合 → EdgeAuthHandler(JWT + Disruptor 投递)。
 * - WS 旁路:前端 /ws/** 直连 gateway-go,不经此 Netty(由 Ingress/前端直连 80 端口实现)。
 *
 * 设计:Netty 只做"解码 + 鉴权 + 投队列",重活(下游转发)交给 Disruptor 消费者,
 * 边缘 worker 线程极致轻量,扛突发并发。
 *
 * 注意:Netty 边缘端口与 Spring MVC 端口分离 ——
 *   - Netty edge port: ${edge.port:9090}(对外接入)
 *   - Spring MVC port: ${server.port:8090}(/api/auth、/actuator、本地控制器)
 * 两者在同一 JVM,本地路径由 EdgeAuthHandler 回环到 Spring MVC(见 LocalForwarder)。
 */
@Component
public class NettyEdgeServer {

    private static final Logger log = LoggerFactory.getLogger(NettyEdgeServer.class);

    @Value("${edge.port:9090}")
    private int edgePort;

    private final JwtService jwt;
    private final InletDisruptor inlet;

    private EventLoopGroup boss;
    private EventLoopGroup worker;
    private io.netty.channel.Channel serverChannel;

    public NettyEdgeServer(JwtService jwt, InletDisruptor inlet) {
        this.jwt = jwt;
        this.inlet = inlet;
    }

    @PostConstruct
    public void start() {
        // 异步启动,不阻塞 Spring 容器初始化
        new Thread(this::run, "netty-edge-bootstrap").start();
    }

    private void run() {
        boss = new NioEventLoopGroup(1);
        worker = new NioEventLoopGroup(0); // 0 = 默认 = 2*CPU
        try {
            ServerBootstrap b = new ServerBootstrap();
            b.group(boss, worker)
                    .channel(NioServerSocketChannel.class)
                    .childOption(ChannelOption.SO_BACKLOG, 8192)
                    .childOption(ChannelOption.SO_KEEPALIVE, true)
                    .childHandler(new ChannelInitializer<SocketChannel>() {
                        @Override
                        protected void initChannel(SocketChannel ch) {
                            ch.pipeline()
                                    .addLast("codec", new HttpServerCodec())
                                    .addLast("aggregator", new HttpObjectAggregator(8 * 1024 * 1024)) // 8MB
                                    .addLast("auth", new EdgeAuthHandler(jwt, inlet));
                        }
                    });
            serverChannel = b.bind(edgePort).sync().channel();
            log.info("NettyEdgeServer started on port {}", edgePort);
            serverChannel.closeFuture().sync();
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        } catch (Exception e) {
            log.error("NettyEdgeServer start failed", e);
        }
    }

    @PreDestroy
    public void stop() {
        if (serverChannel != null) serverChannel.close();
        if (boss != null) boss.shutdownGracefully();
        if (worker != null) worker.shutdownGracefully();
        log.info("NettyEdgeServer stopped");
    }
}
