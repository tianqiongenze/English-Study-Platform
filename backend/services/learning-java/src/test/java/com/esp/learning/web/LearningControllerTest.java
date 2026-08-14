package com.esp.learning.web;

import com.esp.learning.service.DifficultyAdjuster;
import com.esp.learning.service.EbbinghausScheduler;
import com.esp.learning.service.LearningService;
import com.esp.learning.auth.JwtValidator;
import com.esp.learning.disruptor.LearningEventDisruptor;
import com.esp.learning.repository.UserMasteryRepository;
import com.esp.cache.CacheService;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.webmvc.test.autoconfigure.WebMvcTest;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.http.MediaType;
import org.springframework.test.context.TestPropertySource;
import org.springframework.test.web.servlet.MockMvc;

import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.*;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.*;

/**
 * LearningController Web 层测试:验证 REST 契约、状态码与 JSON 结构。
 * 通过 @MockitoBean 隔离 Disruptor / Repository,保证单测快速、无外部依赖。
 * P0-2:userId 一律来自 JwtAuthFilter 注入的 request attribute(模拟
 * .requestAttr("userId", 7)),不再信任 X-User-Id header / body user_id。
 */
@WebMvcTest(LearningController.class)
@TestPropertySource("classpath:application-test.properties")
class LearningControllerTest {

    @Autowired
    MockMvc mvc;

    @MockitoBean LearningService learningService;
    @MockitoBean UserMasteryRepository masteryRepo;
    @MockitoBean LearningEventDisruptor disruptor;
    @MockitoBean EbbinghausScheduler scheduler;
    @MockitoBean DifficultyAdjuster adjuster;
    @MockitoBean JwtValidator jwtValidator;
    @MockitoBean CacheService cacheService;

    @Test
    void reportEvent_accepted_whenDisruptorPublishes() throws Exception {
        when(learningService.reportEvent(anyLong(), anyString(), anyString())).thenReturn(true);

        mvc.perform(post("/api/learning/event")
                        .requestAttr("userId", 7)
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"user_id\":7,\"event_type\":\"answer\",\"payload\":\"{}\"}"))
                .andExpect(status().isAccepted())
                .andExpect(jsonPath("$.accepted").value(true));
    }

    @Test
    void reportEvent_503_whenDisruptorFull() throws Exception {
        when(learningService.reportEvent(anyLong(), anyString(), anyString())).thenReturn(false);

        mvc.perform(post("/api/learning/event")
                        .requestAttr("userId", 7)
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"user_id\":7,\"event_type\":\"answer\"}"))
                .andExpect(status().isServiceUnavailable())
                .andExpect(jsonPath("$.accepted").value(false));
    }

    @Test
    void difficulty_returnsNextAndSpeed() throws Exception {
        when(learningService.nextDifficulty(3, true)).thenReturn(4);
        when(learningService.speedMultiplier(4)).thenReturn(1.28);

        mvc.perform(post("/api/learning/difficulty")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"current\":3,\"correct\":true}"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.next_difficulty").value(4))
                .andExpect(jsonPath("$.speed_multiplier").value(1.28))
                .andExpect(jsonPath("$.scaffolding").value(false));
    }

    @Test
    void placement_usesDefaults_whenBodyEmpty() throws Exception {
        doNothing().when(learningService).initMastery(anyLong(), anyList(), anyDouble());

        mvc.perform(post("/api/learning/placement")
                        .requestAttr("userId", 7)
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{}"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.initialized").value(0));
    }

    /** P0-2 断言:userId 取自 JWT attribute(42),伪造 body user_id=7 被忽略。 */
    @Test
    void reportEvent_ignoresBodyUserId_usesJwtSubject() throws Exception {
        when(learningService.reportEvent(anyLong(), anyString(), anyString())).thenReturn(true);

        mvc.perform(post("/api/learning/event")
                        .requestAttr("userId", 42)
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"user_id\":7,\"event_type\":\"answer\"}"))
                .andExpect(status().isAccepted())
                .andExpect(jsonPath("$.accepted").value(true));
        verify(learningService).reportEvent(eq(42L), anyString(), anyString());
    }

    /** P0-2 断言:伪造 X-User-Id header 不生效,userId 仍取 JWT attribute。 */
    @Test
    void reportEvent_ignoresForgedHeader_usesJwtSubject() throws Exception {
        when(learningService.reportEvent(anyLong(), anyString(), anyString())).thenReturn(true);

        mvc.perform(post("/api/learning/event")
                        .requestAttr("userId", 7)
                        .header("X-User-Id", 999)
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"event_type\":\"answer\"}"))
                .andExpect(status().isAccepted())
                .andExpect(jsonPath("$.accepted").value(true));
        verify(learningService).reportEvent(eq(7L), anyString(), anyString());
        verify(learningService, never()).reportEvent(eq(999L), anyString(), anyString());
    }

    /** P0-2 断言:JWT attribute 缺失(未鉴权直穿)→ 401。 */
    @Test
    void reportEvent_401_whenNoJwtAttribute() throws Exception {
        mvc.perform(post("/api/learning/event")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"user_id\":7,\"event_type\":\"answer\"}"))
                .andExpect(status().isUnauthorized())
                .andExpect(jsonPath("$.accepted").value(false));
        verifyNoInteractions(learningService);
    }
}
