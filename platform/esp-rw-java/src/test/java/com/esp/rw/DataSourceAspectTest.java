package com.esp.rw;

import org.aspectj.lang.ProceedingJoinPoint;
import org.aspectj.lang.reflect.MethodSignature;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;
import org.mockito.Mockito;

import java.lang.reflect.Method;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

class DataSourceAspectTest {

    @AfterEach
    void clear() { RoutingHolder.clear(); }

    /** 用注解标注的方法,供切面读取。 */
    @DataSource(DataSourceType.READ)
    void readMethod() {}

    @DataSource(value = DataSourceType.READ, consistent = true)
    void consistentReadMethod() {}

    @DataSource(DataSourceType.WRITE)
    void writeMethod() {}

    private ProceedingJoinPoint mockPjp(Method method, AtomicReference<DataSourceType> seenDuringProceed) throws Throwable {
        ProceedingJoinPoint pjp = mock(ProceedingJoinPoint.class);
        MethodSignature sig = mock(MethodSignature.class);
        when(sig.getMethod()).thenReturn(method);
        when(pjp.getSignature()).thenReturn(sig);
        when(pjp.proceed()).thenAnswer(inv -> {
            seenDuringProceed.set(RoutingHolder.get());
            return null;
        });
        return pjp;
    }

    @Test
    void readAnnotationSetsReadDuringProceedAndRestores() throws Throwable {
        AtomicReference<DataSourceType> seen = new AtomicReference<>();
        Method m = DataSourceAspectTest.class.getDeclaredMethod("readMethod");
        RoutingHolder.set(DataSourceType.WRITE);  // prev=WRITE
        ProceedingJoinPoint pjp = mockPjp(m, seen);
        new DataSourceAspect().around(pjp);
        assertEquals(DataSourceType.READ, seen.get(), "proceed 期间应为 READ");
        assertEquals(DataSourceType.WRITE, RoutingHolder.get(), "恢复后 prev=WRITE 但被 clear");
        // prev=WRITE 时 finally 会 clear,所以 get() 应是默认 WRITE
        verify(pjp, times(1)).proceed();
    }

    @Test
    void consistentReadForcesWriteDuringProceed() throws Throwable {
        AtomicReference<DataSourceType> seen = new AtomicReference<>();
        Method m = DataSourceAspectTest.class.getDeclaredMethod("consistentReadMethod");
        RoutingHolder.set(DataSourceType.WRITE);
        ProceedingJoinPoint pjp = mockPjp(m, seen);
        new DataSourceAspect().around(pjp);
        assertEquals(DataSourceType.WRITE, seen.get(), "consistent READ 应强制 WRITE");
        verify(pjp, times(1)).proceed();
    }

    @Test
    void writeAnnotationSetsWriteDuringProceed() throws Throwable {
        AtomicReference<DataSourceType> seen = new AtomicReference<>();
        Method m = DataSourceAspectTest.class.getDeclaredMethod("writeMethod");
        RoutingHolder.clear();  // prev 默认 WRITE
        ProceedingJoinPoint pjp = mockPjp(m, seen);
        new DataSourceAspect().around(pjp);
        assertEquals(DataSourceType.WRITE, seen.get());
        verify(pjp, times(1)).proceed();
    }

    @Test
    void nestedPrevReadRestoreReadAndNoClear() throws Throwable {
        AtomicReference<DataSourceType> seen = new AtomicReference<>();
        Method m = DataSourceAspectTest.class.getDeclaredMethod("writeMethod");
        RoutingHolder.set(DataSourceType.READ);  // prev=READ
        ProceedingJoinPoint pjp = mockPjp(m, seen);
        new DataSourceAspect().around(pjp);
        assertEquals(DataSourceType.WRITE, seen.get(), "proceed 期间 WRITE");
        assertEquals(DataSourceType.READ, RoutingHolder.get(), "恢复后 prev=READ 不被 clear");
        verify(pjp, times(1)).proceed();
    }

    @Test
    void nestedPrevWriteClearsAfterRestore() throws Throwable {
        AtomicReference<DataSourceType> seen = new AtomicReference<>();
        Method m = DataSourceAspectTest.class.getDeclaredMethod("writeMethod");
        RoutingHolder.clear();  // prev 默认 WRITE
        ProceedingJoinPoint pjp = mockPjp(m, seen);
        new DataSourceAspect().around(pjp);
        assertEquals(DataSourceType.WRITE, seen.get());
        assertEquals(DataSourceType.WRITE, RoutingHolder.get(), "clear 后默认 WRITE");
        verify(pjp, times(1)).proceed();
    }
}
