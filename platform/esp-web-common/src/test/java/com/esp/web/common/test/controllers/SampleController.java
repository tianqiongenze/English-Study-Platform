package com.esp.web.common.test.controllers;

/**
 * GlobalExceptionHandlerTest 用控制器(包路径与测试 basePackages 匹配)。
 */
public class SampleController {

    public void boom() {
        throw new IllegalStateException("boom");
    }

    public void rpcBoom() {
        throw new RuntimeException("rpc boom");
    }
}
