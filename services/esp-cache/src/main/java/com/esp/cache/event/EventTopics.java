package com.esp.cache.event;

/**
 * 业务 EDA 事件 Pulsar topic 短名常量(复用 esp-cache PulsarClientHolder)。
 * 详见 ADR-EDA。
 */
public final class EventTopics {

    /** bff-java → learning-java:用户注册成功,触发 mastery 初始化(补死代码断链)。 */
    public static final String USER_REGISTERED = "user.registered";

    /** teacher-java → learning-java:教师终审完成,更新对应 kp mastery 分数。 */
    public static final String TEACHER_FINALIZED = "teacher.finalized";

    /** teacher-java 内部削峰:autoGrade 请求异步入队,consumer 异步执行评阅。 */
    public static final String AUTO_GRADE_REQUESTED = "teacher.autograde.requested";

    /** gateway-go → learning-java:口语会话结束,经 Disruptor 记学情事件(驱动复习调度)。 */
    public static final String SESSION_ENDED = "session.ended";

    /** writing-py → teacher-java:写作批改完成(统计;user_id 透传补全后可回写 mastery)。 */
    public static final String WRITING_GRADED = "writing.graded";

    /**
     * P2-2:learning-java HTTP 入口 → learning-worker:学情事件投递。
     * <p>拆分独立 Deployment 后,HTTP 入口(learning-java)不再进程内 Disruptor 消费,
     * 改为把 (userId,eventType,payload) 发到本 topic;learning-worker 订阅后进 Disruptor 批量落库。
     * <p>单进程一体化部署时(esp.disruptor.enabled=true)不使用本 topic,直接进程内 Disruptor。
     */
    public static final String LEARNING_EVENT_REPORT = "learning.event.report";

    private EventTopics() {}
}
