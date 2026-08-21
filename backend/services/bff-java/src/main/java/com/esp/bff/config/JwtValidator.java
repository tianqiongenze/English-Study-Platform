package com.esp.bff.config;

import com.esp.common.jwt.BaseJwtValidator;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

/**
 * JWT RS256 校验器(ADR-12 安全)。
 * <p>
 * BFF 既持有私钥签发(JwtService),也持公钥校验(本类),用于 Spring MVC 路径
 * (如 /api/auth/** 回环、actuator)的边缘鉴权。Netty 边缘层由 EdgeAuthHandler 处理。
 * <p>
 * 公钥来源:env JWT_RSA_PUBLIC_KEY_PEM(X.509 SPKI PEM,从 K8s Secret esp-rsa-jwt-sign 注入)。
 * 参照 learning-java/teacher-java 的 JwtValidator,继承 esp-common-java BaseJwtValidator。
 * <p>
 * 补充:AuthFilter 构造器依赖 BaseJwtValidator bean,本类提供该 bean(此前缺失致 Spring DI
 * 失败 CrashLoop)。
 */
@Component
public class JwtValidator extends BaseJwtValidator {

    public JwtValidator(@Value("${jwt.rsa.public-key-pem:}") String publicKeyPem) {
        super(publicKeyPem);
    }
}
