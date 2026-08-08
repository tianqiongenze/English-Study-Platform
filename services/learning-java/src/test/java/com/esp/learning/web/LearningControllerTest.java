package com.esp.learning.web;

import com.esp.learning.service.DifficultyAdjuster;
import com.esp.learning.service.EbbinghausScheduler;
import com.esp.learning.service.LearningService;
import com.esp.learning.disruptor.LearningEventDisruptor;
import com.esp.learning.repository.UserMasteryRepository;
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
 * 通过 @MockBean 隔离 Disruptor / Repository,保证单测快速、无外部依赖。
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

    @Test
    void reportEvent_accepted_whenDisruptorPublishes() throws Exception {
        when(learningService.reportEvent(anyLong(), anyString(), anyString())).thenReturn(true);

        mvc.perform(post("/api/learning/event")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"user_id\":7,\"event_type\":\"answer\",\"payload\":\"{}\"}"))
                .andExpect(status().isAccepted())
                .andExpect(jsonPath("$.accepted").value(true));
    }

    @Test
    void reportEvent_503_whenDisruptorFull() throws Exception {
        when(learningService.reportEvent(anyLong(), anyString(), anyString())).thenReturn(false);

        mvc.perform(post("/api/learning/event")
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
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{}"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.initialized").value(0));
    }

    /** P1-6 断点3:X-User-Id header 优先于 body user_id。 */
    @Test
    void reportEvent_usesHeaderUserId_whenPresent() throws Exception {
        when(learningService.reportEvent(anyLong(), anyString(), anyString())).thenReturn(true);

        mvc.perform(post("/api/learning/event")
                        .header("X-User-Id", 42)
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"user_id\":7,\"event_type\":\"answer\"}"))
                .andExpect(status().isAccepted())
                .andExpect(jsonPath("$.accepted").value(true));
        verify(learningService).reportEvent(eq(42L), anyString(), anyString());
    }

    /** P1-6 断点3:无 X-User-Id header 时 fallback 到 body user_id。 */
    @Test
    void reportEvent_fallsBackToBodyUserId_whenNoHeader() throws Exception {
        when(learningService.reportEvent(anyLong(), anyString(), anyString())).thenReturn(true);

        mvc.perform(post("/api/learning/event")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"user_id\":7,\"event_type\":\"answer\"}"))
                .andExpect(status().isAccepted())
                .andExpect(jsonPath("$.accepted").value(true));
        verify(learningService).reportEvent(eq(7L), anyString(), anyString());
    }
}
