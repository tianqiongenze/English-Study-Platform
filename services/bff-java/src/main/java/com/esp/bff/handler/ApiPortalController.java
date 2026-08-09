package com.esp.bff.handler;

import org.springframework.core.io.ClassPathResource;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

import java.io.InputStream;

/**
 * 统一 API 在线测试门户静态资源端点(会话22-2 债3)。
 *
 * springdoc 已自动暴露 /v3/api-docs + /swagger-ui.html(/docs 聚合入口,见 application.yml
 * springdoc.swagger-ui.urls)。聚合清单里引用了 Go/Rust 手写路由清单
 * (docs/api/portal/openapi-go-rust.yaml),构建时打进 classpath:/static/openapi-go-rust.yaml,
 * 由本控制器对外暴露为 /openapi-go-rust.yaml,供 Swagger UI 拉取。
 */
@RestController
public class ApiPortalController {

    @GetMapping(value = "/openapi-go-rust.yaml", produces = "application/yaml")
    public ResponseEntity<byte[]> goRustOpenApi() {
        try (InputStream is = new ClassPathResource("static/openapi-go-rust.yaml").getInputStream()) {
            return ResponseEntity.ok().contentType(MediaType.parseMediaType("application/yaml")).body(is.readAllBytes());
        } catch (Exception e) {
            return ResponseEntity.notFound().build();
        }
    }
}
