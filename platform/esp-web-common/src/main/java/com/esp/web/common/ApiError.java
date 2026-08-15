package com.esp.web.common;

/**
 * 统一错误信封类型 —— envelope {@code {code, message, traceId}} 的单一真源。
 *
 * <p>替代各服务 GlobalExceptionHandler 内联的 {@code Map.of("code",...,"message",...,"traceId",...)}:
 * 字段拼写/语义在此类唯一定义,Jackson 序列化后即 JSON 响应体。
 *
 * <ul>
 *   <li>code —— 稳定错误码(INTERNAL_ERROR / UPSTREAM_UNAVAILABLE)</li>
 *   <li>message —— 面向运维的简短描述(不泄漏内部堆栈)</li>
 *   <li>traceId —— OTel 当前 span traceId(未启用 agent 时为 null),跨端串联排查</li>
 * </ul>
 */
public class ApiError {

    private final String code;
    private final String message;
    private final String traceId;

    private ApiError(String code, String message, String traceId) {
        this.code = code;
        this.message = message;
        this.traceId = traceId;
    }

    public static ApiError of(String code, String message, String traceId) {
        return new ApiError(code, message, traceId);
    }

    public String getCode() {
        return code;
    }

    public String getMessage() {
        return message;
    }

    public String getTraceId() {
        return traceId;
    }
}
