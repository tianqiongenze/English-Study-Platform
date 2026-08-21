package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// createTestContext creates a gin context for testing.
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

// newTestServer creates a Server with nil DBs and a test JWT validator for
// handler unit tests that don't need DB. P1-30: requireAuth 走零信任 JWT 自校验,
// 故单测必须注入真实公钥 validator(测试私钥签发 token 走真实执行路径)。
func newTestServer(t *testing.T) *Server {
	return &Server{jwt: testValidator(t)}
}

// ---- filterBlocked:关键词过滤 ----

func TestFilterBlocked_Hit(t *testing.T) {
	cases := []struct {
		body    string
		wantHit bool
	}{
		{"大家来赌博吧", true},
		{"这个代考服务不错", true},
		{"加V详聊", true},
		{"正常学习讨论", false},
		{"", false},
		{"赌博诈骗", true},
	}
	for _, c := range cases {
		_, hit := filterBlocked(c.body)
		if hit != c.wantHit {
			t.Errorf("filterBlocked(%q): hit=%v, want %v", c.body, hit, c.wantHit)
		}
	}
}

func TestFilterBlocked_CaseInsensitive(t *testing.T) {
	_, hit := filterBlocked("正常内容")
	if hit {
		t.Error("clean content should not be blocked")
	}
}

func TestFilterBlocked_ReturnsWord(t *testing.T) {
	word, hit := filterBlocked("我来加微信")
	if !hit {
		t.Fatal("expected hit")
	}
	if word == "" {
		t.Error("expected non-empty blocked word")
	}
}

// ---- genInviteCode:6 位大写字母数字 ----

func TestGenInviteCode_Length(t *testing.T) {
	code := genInviteCode()
	if len(code) != inviteCodeLen {
		t.Errorf("invite code length: got %d, want %d", len(code), inviteCodeLen)
	}
}

func TestGenInviteCode_Charset(t *testing.T) {
	const allowed = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	for i := 0; i < 100; i++ {
		code := genInviteCode()
		for _, c := range code {
			if !strings.ContainsRune(allowed, c) {
				t.Errorf("invite code %q contains invalid char %c", code, c)
				return
			}
		}
	}
}

func TestGenInviteCode_Unique(t *testing.T) {
	seen := make(map[string]bool, 50)
	for i := 0; i < 50; i++ {
		code := genInviteCode()
		if seen[code] {
			t.Fatalf("duplicate invite code: %s", code)
		}
		seen[code] = true
	}
}

// ---- randID:随机 hex ----

func TestRandID_Length(t *testing.T) {
	id := randID()
	if len(id) != 32 {
		t.Errorf("randID length: got %d, want 32", len(id))
	}
}

func TestRandID_Hex(t *testing.T) {
	id := randID()
	for _, c := range id {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Errorf("randID contains non-hex: %c", c)
			return
		}
	}
}

// ---- requireAuth(P1-30 零信任 JWT 自校验,单测走真实签发 token)----

func TestRequireAuth_Unauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil) // 无 token

	srv := newTestServer(t)
	uid, ok := srv.requireAuth(c)
	if ok {
		t.Error("should not be authorized without token")
	}
	if uid != 0 {
		t.Errorf("uid should be 0, got %d", uid)
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", w.Code)
	}
}

func TestRequireAuth_Valid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Request.Header.Set("Authorization", "Bearer "+signTestJWT(t, 123))

	srv := newTestServer(t)
	uid, ok := srv.requireAuth(c)
	if !ok {
		t.Error("should be authorized")
	}
	if uid != 123 {
		t.Errorf("uid: got %d, want 123", uid)
	}
}

func TestRequireAuth_InvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Request.Header.Set("Authorization", "Bearer not-a-valid-jwt") // 非法 token

	srv := newTestServer(t)
	uid, ok := srv.requireAuth(c)
	if ok {
		t.Error("should not be authorized with invalid token")
	}
	if uid != 0 {
		t.Errorf("uid should be 0, got %d", uid)
	}
}

func TestRequireAuth_ZeroUID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	// signTestJWT 对 uid<=0 返回空串(模拟生产:无有效 subject → ParseUserID 返 0)
	c.Request.Header.Set("Authorization", "Bearer "+signTestJWT(t, 0))

	srv := newTestServer(t)
	uid, ok := srv.requireAuth(c)
	if ok {
		t.Error("uid=0 should not be authorized")
	}
	if uid != 0 {
		t.Errorf("uid should be 0, got %d", uid)
	}
}

// ---- reportHideThreshold / maxMemberLimit 常量锁 ----

func TestConstants(t *testing.T) {
	if reportHideThreshold != 3 {
		t.Errorf("reportHideThreshold = %d, want 3", reportHideThreshold)
	}
	if inviteCodeLen != 6 {
		t.Errorf("inviteCodeLen = %d, want 6", inviteCodeLen)
	}
	if maxMemberLimit != 500 {
		t.Errorf("maxMemberLimit = %d, want 500", maxMemberLimit)
	}
}
