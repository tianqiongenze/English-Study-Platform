package main

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestUidStr(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{-1, "-1"},
		{9223372036854775807, "9223372036854775807"},
	}
	for _, c := range cases {
		if got := uidStr(c.in); got != c.want {
			t.Errorf("uidStr(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseTop(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"10", 9},   // 默认 top=10 -> ZRevRange 0..9
		{"1", 0},    // top=1 -> 0
		{"100", 99}, // 上限边界
		{"0", 9},    // 非法(<=0)回退默认 10
		{"-5", 9},   // 负数回退
		{"101", 9},  // 超 100 回退
		{"abc", 9},  // 非数字回退
		{"", 9},     // 空串回退
		{"50", 49},  // 正常中间值
	}
	for _, c := range cases {
		if got := parseTop(c.in); got != c.want {
			t.Errorf("parseTop(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// ---- rankings.go:periodBucket ----

func TestPeriodBucket(t *testing.T) {
	jan1 := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC) // 周四, W03
	cases := []struct {
		now    time.Time
		period string
		want   string
	}{
		{jan1, "daily", "20260115"},
		{jan1, "monthly", "202601"},
		{jan1, "weekly", "2026-W03"},
		{jan1, "unknown", "2026-W03"}, // default = weekly
	}
	for _, c := range cases {
		if got := periodBucket(c.now, c.period); got != c.want {
			t.Errorf("periodBucket(%s, %s) = %q, want %q", c.now.Format("2006-01-02"), c.period, got, c.want)
		}
	}
}

// ---- rankings.go:lbKey ----

func TestLbKey(t *testing.T) {
	jan15 := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	key := lbKey("xp", "global", "all", "daily", jan15)
	if key != "esp:gamif:lb:xp:global:all:20260115" {
		t.Errorf("lbKey = %q", key)
	}
}

// ---- rankings.go:xpFromEvent ----

func TestXpFromEvent(t *testing.T) {
	cases := []struct {
		name           string
		typ            string
		payload        map[string]any
		wantXP         float64
		wantProgress   float64
	}{
		{"emotion filtered", "emotion", nil, 0, 0},
		{"crdb prefix filtered", "crdb.sync", nil, 0, 0},
		{"empty type filtered", "", nil, 0, 0},
		{"answer correct", "answer", map[string]any{"correct": true}, 8, 1},
		{"answer wrong", "answer", map[string]any{"correct": false}, 2, 0},
		{"answer correct string", "answer", map[string]any{"correct": "true"}, 8, 1},
		{"practice correct", "practice_attempt", map[string]any{"answer": "correct"}, 3, 1},
		{"practice wrong", "practice_attempt", map[string]any{"answer": "wrong"}, 1, 0},
		{"content.item_answered correct", "content.item_answered", map[string]any{"correct": true}, 5, 1},
		{"content.item_answered wrong", "content.item_answered", map[string]any{"correct": false}, 1, 0},
		{"content.item_answered perfect score", "content.item_answered", map[string]any{"correct": true, "score": float64(100)}, 7, 1},
		{"content.item_answered high score wrong", "content.item_answered", map[string]any{"correct": false, "score": float64(100)}, 3, 1},
		{"session_completed", "session_completed", nil, 10, 1},
		{"unknown event gets 1", "unknown_foo", nil, 1, 0},
		{"speech.completed", "speech.completed", nil, 8, 1},
		{"writing.submitted", "writing.submitted", nil, 10, 1},
		{"answer capped at maxEventXP", "answer", map[string]any{"correct": true}, 8, 1}, // 8 < 20, not capped
		{"unknown event capped at 1", "zzz_new_event", nil, 1, 0},
	}
	for _, c := range cases {
		xp, prog := xpFromEvent(c.typ, c.payload)
		if xp != c.wantXP || prog != c.wantProgress {
			t.Errorf("%s: xp=%g prog=%g, want xp=%g prog=%g", c.name, xp, prog, c.wantXP, c.wantProgress)
		}
	}
}

// ---- rankings.go:categoryOf ----

func TestCategoryOf(t *testing.T) {
	cases := []struct {
		typ     string
		payload map[string]any
		want    string
	}{
		{"content.item_answered", map[string]any{"category": "reading"}, "reading"},
		{"content.item_answered", map[string]any{"item_type": "grammar"}, "grammar"},
		{"content.item_answered", map[string]any{"category": "", "item_type": ""}, "content"},
		{"content.item_answered", nil, "content"},
		{"answer", nil, "answer"},
		{"practice_attempt", map[string]any{}, "practice_attempt"},
	}
	for _, c := range cases {
		if got := categoryOf(c.typ, c.payload); got != c.want {
			t.Errorf("categoryOf(%q, %v) = %q, want %q", c.typ, c.payload, got, c.want)
		}
	}
}

// ---- rankings.go:regionAncestors ----

func TestRegionAncestors(t *testing.T) {
	cases := []struct {
		region string
		want   []string
	}{
		{"CN", []string{"CN"}},
		{"CN/Guangdong", []string{"CN", "CN/Guangdong"}},
		{"CN/Guangdong/Shenzhen", []string{"CN", "CN/Guangdong", "CN/Guangdong/Shenzhen"}},
		{"", nil},
		{"/", nil},
	}
	for _, c := range cases {
		got := regionAncestors(c.region)
		if len(got) != len(c.want) {
			t.Errorf("regionAncestors(%q): len=%d, want %d (got=%v)", c.region, len(got), len(c.want), got)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("regionAncestors(%q)[%d] = %q, want %q", c.region, i, got[i], c.want[i])
			}
		}
	}
}

// ---- rankings.go:compositeScore / xpFromScore ----

func TestCompositeScore(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 30, 45, 123456789, time.UTC)
	score := compositeScore(10, now)
	if score <= 0 {
		t.Errorf("compositeScore(10, now) = %g, want > 0", score)
	}
	// XP 相同,更早的时间 score 更大(早到优先)
	// 需要两个时间的 ts%scoreScale 不同(同一秒内不同纳秒部分)
	earlier := now.Add(-time.Nanosecond)
	s1 := compositeScore(10, earlier)
	s2 := compositeScore(10, now)
	if s1 <= s2 {
		t.Errorf("earlier score %g should > later score %g", s1, s2)
	}
	// 验证差值等于纳秒差
	diff := s2 - s1
	if diff != -1 {
		t.Errorf("score diff = %g, want -1 (1 ns difference)", diff)
	}
}

func TestXpFromScore(t *testing.T) {
	cases := []struct {
		score float64
		want  float64
	}{
		{0, 0},
		{-1, 0},
		{scoreScale * 10, 10},
		{scoreScale * 5.5, 5.5},
	}
	for _, c := range cases {
		if got := xpFromScore(c.score); got != c.want {
			t.Errorf("xpFromScore(%g) = %g, want %g", c.score, got, c.want)
		}
	}
}

// ---- rankings.go:message_idFromPayload ----

func TestMessageIDFromPayload(t *testing.T) {
	cases := []struct {
		name    string
		id      int64
		payload []byte
		want    string
	}{
		{"nil payload", 42, nil, "id:42"},
		{"empty payload", 42, []byte{}, "id:42"},
		{"message_id present", 1, []byte(`{"message_id":"msg-abc"}`), "msg-abc"},
		{"msg_id fallback", 1, []byte(`{"msg_id":"msg-xyz"}`), "msg-xyz"},
		{"message_id takes precedence", 1, []byte(`{"message_id":"mm1","msg_id":"mm2"}`), "mm1"},
		{"invalid json", 1, []byte(`{bad`), "id:1"},
		{"id fallback when no msg fields", 99, []byte(`{"other":"val"}`), "id:99"},
	}
	for _, c := range cases {
		if got := message_idFromPayload(c.id, c.payload); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// ---- rankings.go:lastLabel ----

func TestLastLabel(t *testing.T) {
	cases := []struct {
		code, want string
	}{
		{"CN/Guangdong/Shenzhen", "Shenzhen"},
		{"CN", "CN"},
		{"a/b/c/d", "d"},
	}
	for _, c := range cases {
		if got := lastLabel(c.code); got != c.want {
			t.Errorf("lastLabel(%q) = %q, want %q", c.code, got, c.want)
		}
	}
}

// ---- rankings_antifraud.go:suppressXPFromPayload ----

func TestSuppressXPFromPayload(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]any
		want    bool
	}{
		{"nil", nil, false},
		{"bool true", map[string]any{"suppress_xp": true}, true},
		{"bool false", map[string]any{"suppress_xp": false}, false},
		{"string true", map[string]any{"suppress_xp": "true"}, true},
		{"string other", map[string]any{"suppress_xp": "no"}, false},
		{"no key", map[string]any{}, false},
	}
	for _, c := range cases {
		if got := suppressXPFromPayload(c.payload); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// ---- rankings_antifraud.go:session_duration_too_short (behavioral check in antifraudDecision)
// antifraudDecision 依赖 rdb(Redis),只测无 Redis 依赖的 payload 解析分支。

// ---- rankings_cache.go:toInt64 / metaKey ----

func TestToInt64(t *testing.T) {
	cases := []struct {
		in   any
		want int64
	}{
		{int64(42), 42},
		{int(100), 100},
		{"256", 256},
		{"abc", 0},
		{nil, 0},
		{3.14, 0},
	}
	for _, c := range cases {
		if got := toInt64(c.in); got != c.want {
			t.Errorf("toInt64(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestMetaKey(t *testing.T) {
	got := metaKey("user", int64(42), int64(100))
	want := "user:42:100"
	if got != want {
		t.Errorf("metaKey = %q, want %q", got, want)
	}
	// metaKey 内部 toInt64("info")=0(string 不是数字),非数字部分变 0
	got2 := metaKey("test", int64(1))
	want2 := "test:1"
	if got2 != want2 {
		t.Errorf("metaKey simple = %q, want %q", got2, want2)
	}
}

// ---- rankings_cache.go:L1 cache 单元测试(纯进程内,不依赖 Redis)----

func TestL1Cache_SetGet(t *testing.T) {
	c := newL1(3)
	c.set("k1", "v1")
	if v, ok := c.get("k1"); !ok || v != "v1" {
		t.Errorf("get k1: got %v, %v", v, ok)
	}
}

func TestL1Cache_Eviction(t *testing.T) {
	c := newL1(2)
	c.set("a", 1)
	c.set("b", 2)
	c.set("c", 3) // should evict "a"
	if _, ok := c.get("a"); ok {
		t.Error("a should be evicted")
	}
	if v, ok := c.get("b"); !ok || v != 2 {
		t.Errorf("b should still exist, got %v", v)
	}
}

func TestL1Cache_Delete(t *testing.T) {
	c := newL1(5)
	c.set("x", 10)
	c.del("x")
	if _, ok := c.get("x"); ok {
		t.Error("x should be deleted")
	}
}

func TestL1Cache_MoveToFront_AccessRefreshes(t *testing.T) {
	c := newL1(2)
	c.set("a", 1)
	c.set("b", 2)
	c.get("a") // refresh "a" -> now most recent
	c.set("c", 3) // should evict "b" (least recent)
	if _, ok := c.get("b"); ok {
		t.Error("b should be evicted after a was accessed")
	}
	if v, ok := c.get("a"); !ok || v != 1 {
		t.Errorf("a should survive, got %v", v)
	}
}

// ---- rankings_cache.go:rankCacheGet/Set/Evict(纯进程内 L1,不依赖 Redis)----

func TestRankCacheSetGet(t *testing.T) {
	// 确保 cacheEnabled
	origEnabled := cacheEnabled
	cacheEnabled = true
	defer func() { cacheEnabled = origEnabled }()

	key := "esp:gamif:lb:xp:global:all:20260101"
	rows := []rankRow{{Rank: 1, UserID: 100, Score: 50}, {Rank: 2, UserID: 200, Score: 30}}
	rankCacheSet(key, rows)
	got, ok := rankCacheGet(nil, key)
	if !ok {
		t.Fatal("rankCacheGet: cache miss")
	}
	if len(got) != 2 || got[0].UserID != 100 {
		t.Errorf("rankCacheGet rows: %v", got)
	}
}

func TestRankCacheEvict(t *testing.T) {
	origEnabled := cacheEnabled
	cacheEnabled = true
	defer func() { cacheEnabled = origEnabled }()

	key := "esp:gamif:lb:evict:test"
	rankCacheSet(key, []rankRow{{Rank: 1, UserID: 1}})
	if _, ok := rankCacheGet(nil, key); !ok {
		t.Fatal("should be cached before evict")
	}
	rankCacheEvict(key)
	if _, ok := rankCacheGet(nil, key); ok {
		t.Error("should be evicted")
	}
}

func TestRankCacheGet_Disabled(t *testing.T) {
	origEnabled := cacheEnabled
	cacheEnabled = false
	defer func() { cacheEnabled = origEnabled }()

	_, ok := rankCacheGet(nil, "any")
	if ok {
		t.Error("cache disabled should always miss")
	}
}

// ---- rankings_cache.go:metaEvict(纯进程内 L1)----

func TestMetaEvict_L1(t *testing.T) {
	origEnabled := cacheEnabled
	cacheEnabled = true
	defer func() { cacheEnabled = origEnabled }()

	// metaKey("metaEvict", int64(42)) = "metaEvict:42"
	l1.set("metaEvict:42", "hello")
	if v, ok := l1.get("metaEvict:42"); !ok || v != "hello" {
		t.Fatal("L1 should have value")
	}
	metaEvict(nil, "metaEvict", int64(42))
	if _, ok := l1.get("metaEvict:42"); ok {
		t.Error("should be evicted from L1")
	}
}

func TestMetaEvict_Disabled(t *testing.T) {
	origEnabled := cacheEnabled
	cacheEnabled = false
	defer func() { cacheEnabled = origEnabled }()

	// 不应 panic
	metaEvict(nil, "test", int64(1))
}

// ---- rankings.go:compositeScore ts<0 分支 ----

func TestCompositeScore_TsNegative(t *testing.T) {
	// 使用 scoreEpochNS 之前的时间(2025年),ts 为负,应 clamp 到 0
	pastTime := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	score := compositeScore(10, pastTime)
	// ts<0 → ts=0 → score = 10*1e10 - 0 = 1e11
	expected := 10 * scoreScale
	if score != expected {
		t.Errorf("compositeScore with past time: got %g, want %g", score, expected)
	}
}

// ---- rankings.go:xpFromEvent 边界 ----

func TestXpFromEvent_MaxCap(t *testing.T) {
	// baseXP 超过 maxEventXP 的事件被封顶
	// session_completed = 10 < 20, 不封顶
	xp, _ := xpFromEvent("session_completed", nil)
	if xp != 10 {
		t.Errorf("session_completed xp = %g, want 10", xp)
	}
}

func TestXpFromEvent_ProgressThreshold(t *testing.T) {
	// xp < 3 → progress=0; xp >= 3 → progress=1
	xp1, p1 := xpFromEvent("answer", map[string]any{"correct": false}) // xp=2
	if p1 != 0 {
		t.Errorf("answer wrong progress = %g, want 0", p1)
	}
	if xp1 != 2 {
		t.Errorf("answer wrong xp = %g, want 2", xp1)
	}

	xp2, p2 := xpFromEvent("answer", map[string]any{"correct": true}) // xp=8
	if p2 != 1 {
		t.Errorf("answer correct progress = %g, want 1", p2)
	}
	if xp2 != 8 {
		t.Errorf("answer correct xp = %g, want 8", xp2)
	}
}

// ---- rankings.go:xpFromScore 边界 ----

func TestXpFromScore_NegativeScore(t *testing.T) {
	got := xpFromScore(-100)
	if got != 0 {
		t.Errorf("xpFromScore(-100) = %g, want 0", got)
	}
}

// ---- rankings.go:categoryOf 深度测试 ----

func TestCategoryOf_DotPrefix(t *testing.T) {
	// content.item_answered → "content"
	got := categoryOf("content.item_answered", nil)
	if got != "content" {
		t.Errorf("got %q, want content", got)
	}
}

func TestCategoryOf_NoDot(t *testing.T) {
	got := categoryOf("answer", nil)
	if got != "answer" {
		t.Errorf("got %q, want answer", got)
	}
}

// ---- rankings.go:periodBucket 所有周期 ----

func TestPeriodBucket_AllPeriods(t *testing.T) {
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := periodBucket(jan1, "daily"); got != "20260101" {
		t.Errorf("daily: %q", got)
	}
	if got := periodBucket(jan1, "monthly"); got != "202601" {
		t.Errorf("monthly: %q", got)
	}
	// 2026-01-01 is Thursday → ISO week 01 → "2026-W01"
	got := periodBucket(jan1, "weekly")
	if got != "2026-W01" {
		t.Errorf("weekly: got %q, want 2026-W01", got)
	}
}

// ---- digital_human.go:requireUID ----

func TestRequireUID_Missing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)

	uid, ok := requireUID(c)
	if ok {
		t.Error("should not be ok")
	}
	if uid != 0 {
		t.Errorf("uid should be 0, got %d", uid)
	}
	if w.Code != 401 {
		t.Errorf("status: got %d, want 401", w.Code)
	}
}

func TestRequireUID_Invalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Request.Header.Set("X-User-Id", "abc")

	uid, ok := requireUID(c)
	if ok {
		t.Error("should not be ok with invalid uid")
	}
	if uid != 0 {
		t.Errorf("uid: got %d, want 0", uid)
	}
	if w.Code != 400 {
		t.Errorf("status: got %d, want 400", w.Code)
	}
}

func TestRequireUID_Valid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Request.Header.Set("X-User-Id", "42")

	uid, ok := requireUID(c)
	if !ok {
		t.Error("should be ok")
	}
	if uid != 42 {
		t.Errorf("uid: got %d, want 42", uid)
	}
}

func TestRequireUID_Zero(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Request.Header.Set("X-User-Id", "0")

	uid, ok := requireUID(c)
	if ok {
		t.Error("uid=0 should not be ok")
	}
	if uid != 0 {
		t.Errorf("uid: got %d, want 0", uid)
	}
}

// ---- digital_human.go:personaIDRe ----

func TestPersonaIDRegex(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"cat1", true},
		{"robot-01", true},
		{"a", true},
		{"abc123def456ghi789jkl012mn", true}, // 28 chars, within 32 limit
		{"abcdefghijklmnopqrstuvwxyz012345", true}, // exactly 32 chars (26+6)
		{"", false},
		{"-starts", false},
		{"UPPER", false},
		{"has space", false},
		{"has@special", false},
		{"abcdefghijklmnopqrstuvwxyz0123456", false}, // 33 chars, exceeds 32
	}
	for _, c := range cases {
		if got := personaIDRe.MatchString(c.id); got != c.want {
			t.Errorf("personaIDRe(%q len=%d) = %v, want %v", c.id, len(c.id), got, c.want)
		}
	}
}
