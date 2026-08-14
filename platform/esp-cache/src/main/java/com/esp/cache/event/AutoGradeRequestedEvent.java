package com.esp.cache.event;

/**
 * 自动评阅请求事件(teacher-java 内部削峰)。
 * autoGrade HTTP 入口只发此消息立即返回,consumer 异步执行原评阅逻辑,
 * 解耦故障域 + 削峰(避免长事务阻塞 HTTP 线程)。
 */
public class AutoGradeRequestedEvent {
    private long assignmentId;
    private Long classId;

    public AutoGradeRequestedEvent() {}

    public AutoGradeRequestedEvent(long assignmentId, Long classId) {
        this.assignmentId = assignmentId;
        this.classId = classId;
    }

    public long getAssignmentId() { return assignmentId; }
    public void setAssignmentId(long assignmentId) { this.assignmentId = assignmentId; }
    public Long getClassId() { return classId; }
    public void setClassId(Long classId) { this.classId = classId; }
}
