package com.esp.rw;

import org.aspectj.lang.ProceedingJoinPoint;
import org.aspectj.lang.annotation.Around;
import org.aspectj.lang.annotation.Aspect;
import org.aspectj.lang.reflect.MethodSignature;
import org.springframework.stereotype.Component;

import java.lang.reflect.Method;

/**
 * @DataSource 注解 AOP 切面:方法入口设置路由 Key,出口清理。
 * consistent=true -> 强制 WRITE(写后强一致读走主库,避免主从延迟读旧值)。
 *
 * 支持嵌套调用:保存前一个路由 Key,方法返回后恢复(prev=WRITE 时清理防 ThreadLocal 泄漏)。
 */
@Aspect
@Component
public class DataSourceAspect {

    @Around("@annotation(com.esp.rw.DataSource)")
    public Object around(ProceedingJoinPoint pjp) throws Throwable {
        MethodSignature sig = (MethodSignature) pjp.getSignature();
        Method method = sig.getMethod();
        DataSource ds = method.getAnnotation(DataSource.class);
        DataSourceType type;
        if (ds.value() == DataSourceType.READ && ds.consistent()) {
            type = DataSourceType.WRITE; // 写后强一致读走主库
        } else {
            type = ds.value();
        }
        DataSourceType prev = RoutingHolder.get();
        RoutingHolder.set(type);
        try {
            return pjp.proceed();
        } finally {
            RoutingHolder.set(prev);
            if (prev == DataSourceType.WRITE) {
                RoutingHolder.clear();
            }
        }
    }
}
