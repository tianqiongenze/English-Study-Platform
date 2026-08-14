package com.esp.bff.auth;

import com.nimbusds.jose.JWSAlgorithm;
import com.nimbusds.jose.jwk.JWKSet;
import com.nimbusds.jose.jwk.RSAKey;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

import java.security.interfaces.RSAPublicKey;
import java.util.UUID;

/**
 * JWKS endpoint(ADR-12 RS256):暴露 BFF 公钥的 JWK Set,供下游服务(learning-java 等)动态获取公钥校验 JWT。
 * 路径:GET /.well-known/jwks.json
 *
 * <p>生产部署后下游可通过 HTTP GET 拉取并缓存此 JWKS,无需硬编码公钥 PEM。
 * 当前 key ID 为固定值 "esp-rs256-1";密钥轮换时改为新 kid 并保留旧 kid 公钥一段时间。</p>
 */
@RestController
public class JwksController {

    private static final String KID = "esp-rs256-1";
    private final JwtService jwtService;

    public JwksController(JwtService jwtService) {
        this.jwtService = jwtService;
    }

    @GetMapping("/.well-known/jwks.json")
    public ResponseEntity<String> jwks() {
        RSAPublicKey pub = (RSAPublicKey) jwtService.getPublicKey();
        RSAKey rsaKey = new RSAKey.Builder(pub)
                .algorithm(JWSAlgorithm.RS256)
                .keyID(KID)
                .build();
        JWKSet jwkSet = new JWKSet(rsaKey);
        return ResponseEntity.ok()
                .contentType(MediaType.APPLICATION_JSON)
                .body(jwkSet.toString());
    }
}
