package com.esp.learning.auth;

import com.esp.common.jwt.BaseJwtValidator;
import io.jsonwebtoken.Claims;
import io.jsonwebtoken.JwtException;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

/**
 * JWT 校验(ADR-12 RS256):learning-java 用公钥校验 BFF 签发的 JWT。
 * 公钥来源:env JWT_RSA_PUBLIC_KEY_PEM(X.509 SPKI PEM)。
 * JWKS 动态获取为后续增强(当前走 PEM 直校验,注释说明 JWKS fetch 为后续)。
 *
 * <p>重构:继承 esp-common-java BaseJwtValidator,消除与 teacher-java 的 PEM 解析重复。
 * 保留原包路径不变,learning 服务代码零改动(类型兼容)。
 */
@Component
public class JwtValidator extends BaseJwtValidator {

    public JwtValidator(@Value("${jwt.rsa.public-key-pem:}") String publicKeyPem) {
        super(publicKeyPem);
    }
}
