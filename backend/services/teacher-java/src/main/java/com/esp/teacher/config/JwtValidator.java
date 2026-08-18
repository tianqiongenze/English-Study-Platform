package com.esp.teacher.config;

import com.esp.common.jwt.BaseJwtValidator;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

/**
 * JWT 校验(ADR-12 RS256):teacher-java 用公钥校验 BFF 签发的 JWT。
 * 公钥来源:env JWT_RSA_PUBLIC_KEY_PEM(X.509 SPKI PEM,由 secret esp-rsa-jwt envFrom 注入)。
 * D2 修复:此前 teacher 端口完全裸奔,本类 + JwtAuthFilter 补齐鉴权。
 *
 * <p>重构:继承 esp-common-java BaseJwtValidator,消除与 learning-java 的 PEM 解析重复。
 * 保留原包路径不变,teacher 服务代码零改动(类型兼容)。
 */
@Component
public class JwtValidator extends BaseJwtValidator {

    public JwtValidator(@Value("${jwt.rsa.public-key-pem:}") String publicKeyPem) {
        super(publicKeyPem);
    }
}
