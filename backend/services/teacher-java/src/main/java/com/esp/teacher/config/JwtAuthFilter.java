package com.esp.teacher.config;

import com.esp.common.jwt.BaseJwtAuthFilter;
import com.esp.common.jwt.BaseJwtValidator;
import org.springframework.stereotype.Component;

import java.util.List;

/**
 * JWT 边缘校验(D2 修复,ADR-12):teacher-java 教师端接口鉴权。
 * 继承 esp-common-java BaseJwtAuthFilter,消除与 learning-java 的 Filter 重复。
 *
 * <p>保留原包路径不变,teacher 服务代码零改动(类型兼容)。
 * 服务可追加自有白名单路径(通过 additionalWhitelist 构造参数)。
 */
@Component
public class JwtAuthFilter extends BaseJwtAuthFilter {

    public JwtAuthFilter(BaseJwtValidator jwt) {
        super(jwt, List.of());
    }
}
