package com.esp.web.common.test;

/**
 * GlobalExceptionHandlerTest 用非匹配包控制器:包 com.esp.web.common.test
 * 不匹配测试 basePackages(com.esp.web.common.test.controllers)。
 */
public class OtherController {

    public void boom() {
        throw new IllegalStateException("other boom");
    }
}
