package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// createTestContext creates a gin context for testing (no DB needed).
func createTestContext(w *httptest.ResponseRecorder, method, path string, body []byte) *gin.Context {
	gin.SetMode(gin.TestMode)
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, strings.NewReader(string(body)))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = req
	return ctx
}

func TestRelayBackoffSec(t *testing.T) {
	cases := []struct {
		retry int
		want  int
	}{
		{1, 1},
		{2, 5},
		{3, 30},
		{4, 300},
		{7, 300},
		{0, 300}, // 未达 1 的退化分支走 default(实际不会从 drainOutbox 触发,但函数须稳定)
	}
	for _, c := range cases {
		if got := relayBackoffSec(c.retry); got != c.want {
			t.Errorf("relayBackoffSec(%d) = %d, want %d", c.retry, got, c.want)
		}
	}
}

// TestRelayMaxRetryDead 验证退避封顶后由 dead 分支接管:retry_count 达 relayMaxRetry 不再退避。
func TestRelayMaxRetryDead(t *testing.T) {
	if relayMaxRetry != 8 {
		t.Fatalf("relayMaxRetry changed to %d; test/契约需同步", relayMaxRetry)
	}
	// 当 nextRetry >= relayMaxRetry 时,drainOutbox 走 relayDeadSQL(本测试仅锁常量,因 dead 路径依赖 DB)。
	for r := relayMaxRetry; r < relayMaxRetry+3; r++ {
		got := relayBackoffSec(r)
		if got != 300 {
			t.Errorf("relayBackoffSec(%d) = %d, want 300 (dead 分支前最后档)", r, got)
		}
	}
}

// ---- grade:判分逻辑(纯函数) ----

func TestGrade_Vocab_AlwaysCorrect(t *testing.T) {
	correct, score := grade("vocab", nil, nil)
	if !correct || score != 1 {
		t.Errorf("vocab: correct=%v score=%g, want true/1", correct, score)
	}
}

func TestGrade_SingleChoice_Correct(t *testing.T) {
	answerB, _ := json.Marshal(map[string]any{"correct_option": 2})
	submitted := map[string]any{"option_index": float64(2)}
	correct, score := grade("reading", submitted, answerB)
	if !correct || score != 100 {
		t.Errorf("correct option: correct=%v score=%g, want true/100", correct, score)
	}
}

func TestGrade_SingleChoice_Wrong(t *testing.T) {
	answerB, _ := json.Marshal(map[string]any{"correct_option": 0})
	submitted := map[string]any{"option_index": float64(1)}
	correct, score := grade("reading", submitted, answerB)
	if correct || score != 0 {
		t.Errorf("wrong option: correct=%v score=%g, want false/0", correct, score)
	}
}

func TestGrade_SingleChoice_NoOptionIndex(t *testing.T) {
	answerB, _ := json.Marshal(map[string]any{"correct_option": 0})
	submitted := map[string]any{"other_field": "value"}
	correct, score := grade("grammar", submitted, answerB)
	if correct || score != 0 {
		t.Errorf("no option_index: correct=%v score=%g, want false/0", correct, score)
	}
}

func TestGrade_SingleChoice_NotAMap(t *testing.T) {
	answerB, _ := json.Marshal(map[string]any{"correct_option": 0})
	correct, score := grade("cloze", "not_a_map", answerB)
	if correct || score != 0 {
		t.Errorf("non-map submitted: correct=%v score=%g, want false/0", correct, score)
	}
}

func TestGrade_SingleChoice_NilSubmitted(t *testing.T) {
	answerB, _ := json.Marshal(map[string]any{"correct_option": 0})
	correct, score := grade("reading", nil, answerB)
	if correct || score != 0 {
		t.Errorf("nil submitted: correct=%v score=%g, want false/0", correct, score)
	}
}

func TestGrade_AllTypes(t *testing.T) {
	answerB, _ := json.Marshal(map[string]any{"correct_option": 1})
	submitted := map[string]any{"option_index": float64(1)}
	for _, itemType := range []string{"reading", "listening", "grammar", "cloze"} {
		correct, score := grade(itemType, submitted, answerB)
		if !correct || score != 100 {
			t.Errorf("%s: correct=%v score=%g, want true/100", itemType, correct, score)
		}
	}
}

// ---- randID:随机 hex 长度/格式 ----

func TestRandID(t *testing.T) {
	id := randID()
	// 16 bytes hex = 32 chars
	if len(id) != 32 {
		t.Errorf("randID length: got %d, want 32", len(id))
	}
	// 须全为 hex 字符
	for _, c := range id {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Errorf("randID contains non-hex char: %c", c)
			break
		}
	}
}

func TestRandID_Unique(t *testing.T) {
	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		id := randID()
		if seen[id] {
			t.Fatalf("duplicate randID: %s", id)
		}
		seen[id] = true
	}
}

// ---- userID/roleOf:从 gin context 解析 ----

func TestUserID(t *testing.T) {
	w := httptest.NewRecorder()
	c := createTestContext(w, "GET", "/", nil)
	c.Request.Header.Set("X-User-Id", "42")
	if got := userID(c); got != 42 {
		t.Errorf("userID = %d, want 42", got)
	}
}

func TestUserID_Invalid(t *testing.T) {
	w := httptest.NewRecorder()
	c := createTestContext(w, "GET", "/", nil)
	c.Request.Header.Set("X-User-Id", "abc")
	if got := userID(c); got != 0 {
		t.Errorf("userID invalid: got %d, want 0", got)
	}
}

func TestRoleOf_Admin(t *testing.T) {
	w := httptest.NewRecorder()
	c := createTestContext(w, "GET", "/", nil)
	c.Request.Header.Set("X-Role", "admin")
	if got := roleOf(c); got != "admin" {
		t.Errorf("roleOf = %q, want admin", got)
	}
}

func TestRoleOf_DefaultLearner(t *testing.T) {
	w := httptest.NewRecorder()
	c := createTestContext(w, "GET", "/", nil)
	if got := roleOf(c); got != "learner" {
		t.Errorf("roleOf default = %q, want learner", got)
	}
}

// ---- requireAdmin:角色校验中间件 ----

func TestRequireAdmin_AsAdmin(t *testing.T) {
	w := httptest.NewRecorder()
	c := createTestContext(w, "GET", "/", nil)
	c.Request.Header.Set("X-Role", "admin")

	called := false
	handler := requireAdmin(func(c *gin.Context) {
		called = true
		c.Status(200)
	})
	handler(c)

	if !called {
		t.Error("admin handler should be called")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
}

func TestRequireAdmin_AsLearner(t *testing.T) {
	w := httptest.NewRecorder()
	c := createTestContext(w, "GET", "/", nil)
	// No X-Role header → default "learner"

	called := false
	handler := requireAdmin(func(c *gin.Context) {
		called = true
	})
	handler(c)

	if called {
		t.Error("learner should not call admin handler")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("status: got %d, want 403", w.Code)
	}
}

// ---- importBatch:批量上限校验 ----

func TestImportBatch_EmptyBatch(t *testing.T) {
	w := httptest.NewRecorder()
	c := createTestContext(w, "POST", "/api/content/import", []byte(`[]`))
	c.Request.Header.Set("X-Role", "admin")

	importBatch(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("empty batch: got %d, want 400", w.Code)
	}
}

func TestImportBatch_BadJSON(t *testing.T) {
	w := httptest.NewRecorder()
	c := createTestContext(w, "POST", "/api/content/import", []byte(`not json`))
	c.Request.Header.Set("X-Role", "admin")

	importBatch(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("bad JSON: got %d, want 400", w.Code)
	}
}

// ---- grade:额外边界 ----

func TestGrade_EmptyAnswer(t *testing.T) {
	// answerB 为空 JSON → CorrectOption defaults to 0
	// submitted option_index=0 matches default → correct
	correct, score := grade("reading", map[string]any{"option_index": float64(0)}, []byte(`{}`))
	if !correct || score != 100 {
		t.Errorf("empty answer default 0: correct=%v score=%g, want true/100", correct, score)
	}
	// submitted option_index=1 doesn't match default 0
	correct2, score2 := grade("reading", map[string]any{"option_index": float64(1)}, []byte(`{}`))
	if correct2 || score2 != 0 {
		t.Errorf("empty answer mismatch: correct=%v score=%g, want false/0", correct2, score2)
	}
}

func TestGrade_Float64Comparison(t *testing.T) {
	answerB, _ := json.Marshal(map[string]any{"correct_option": 3})
	submitted := map[string]any{"option_index": float64(3)}
	correct, score := grade("grammar", submitted, answerB)
	if !correct || score != 100 {
		t.Errorf("float64 comparison: correct=%v score=%g, want true/100", correct, score)
	}
}

// ---- userID:边界 ----

func TestUserID_EmptyHeader(t *testing.T) {
	w := httptest.NewRecorder()
	c := createTestContext(w, "GET", "/", nil)
	// No X-User-Id header
	if got := userID(c); got != 0 {
		t.Errorf("empty header: got %d, want 0", got)
	}
}

func TestUserID_NegativeUID(t *testing.T) {
	w := httptest.NewRecorder()
	c := createTestContext(w, "GET", "/", nil)
	c.Request.Header.Set("X-User-Id", "-1")
	if got := userID(c); got != -1 {
		t.Errorf("negative uid: got %d, want -1", got)
	}
}
