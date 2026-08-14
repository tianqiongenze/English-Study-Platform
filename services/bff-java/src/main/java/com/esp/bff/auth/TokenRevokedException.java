package com.esp.bff.auth;

/**
 * JWT 已被撤销(注销/轮换拉黑)时抛出。P1-9。
 */
public class TokenRevokedException extends RuntimeException {
    public TokenRevokedException(String message) {
        super(message);
    }
}
