package com.esp.cache.event;

/**
 * 用户注册成功事件(bff-java → learning-java)。
 * learning-java 收到后调用 LearningService.initMastery(userId, allKpIds, 0.0)。
 */
public class UserRegisteredEvent {
    private long userId;
    private String email;
    private String role;

    public UserRegisteredEvent() {}

    public UserRegisteredEvent(long userId, String email, String role) {
        this.userId = userId;
        this.email = email;
        this.role = role;
    }

    public long getUserId() { return userId; }
    public void setUserId(long userId) { this.userId = userId; }
    public String getEmail() { return email; }
    public void setEmail(String email) { this.email = email; }
    public String getRole() { return role; }
    public void setRole(String role) { this.role = role; }
}
