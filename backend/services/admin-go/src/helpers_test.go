package main

// 纯函数单测(表驱动),覆盖 cache.go / main.go / ops.go / runtime.go / pulsar.go / debug_proxy.go / middleware.go。
// CLC 铁律:追踪真实执行路径非语义;Go nil-safe;err 显式;禁 panic 控制流。
// TCG 铁律:admin-go 此前零单测,本文件补齐核心纯逻辑路径覆盖。

import (
	"testing"
)

// ---------- cache.go ----------

func TestTruncate(t *testing.T) {
	cases := []struct {
		name, in string
		n        int
		want     string
	}{
		{"shorter_than_n", "abc", 10, "abc"},
		{"exact_len", "abc", 3, "abc"},
		{"longer_truncated", "abcdef", 3, "abc..."},
		{"empty", "", 5, ""},
		{"n_zero", "abc", 0, "..."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := truncate(c.in, c.n); got != c.want {
				t.Errorf("truncate(%q,%d): got %q, want %q", c.in, c.n, got, c.want)
			}
		})
	}
}

func TestParseFloatSafe(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"42", 42},
		{"3.14", 3.14},
		{"0", 0},
		{"", 0},
		{"not-a-number", 0},
		{"  7  ", 7}, // Sscanf %f 跳过前导空白解析出 7
	}
	for _, c := range cases {
		if got := parseFloatSafe(c.in); got != c.want {
			t.Errorf("parseFloatSafe(%q): got %v, want %v", c.in, got, c.want)
		}
	}
}

func TestComputeHitRate(t *testing.T) {
	cases := []struct {
		name, hits, misses, want string
	}{
		{"both_zero", "0", "0", ""},                  // total=0 → 空串
		{"empty_strings", "", "", ""},                // 同 0+0
		{"all_hits", "100", "0", "1.0000"},
		{"no_hits", "0", "100", "0.0000"},
		{"half", "50", "50", "0.5000"},
		{"third", "1", "2", "0.3333"}, // 1/3 → 0.3333(4位)
		{"invalid_inputs_zero", "abc", "xyz", ""},   // 解析失败都 0
		{"one_invalid", "10", "xyz", "1.0000"},       // misses=0 → 10/10
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := computeHitRate(c.hits, c.misses); got != c.want {
				t.Errorf("computeHitRate(%q,%q): got %q, want %q", c.hits, c.misses, got, c.want)
			}
		})
	}
}

func TestExtractLCOMetric(t *testing.T) {
	body := []byte(`# HELP lco_cache_hits_total cache hits
lco_cache_hits_total{layer="L1"} 42
lco_cache_misses_total{layer="L1"} 8
lco_l2_hits_total 7
lco_l2_misses_total 3
# EOF`)
	cases := []struct {
		name, pattern, want string
	}{
		{"hits_L1", `lco_cache_hits_total\{layer="L1"\}\s+([0-9.]+)`, "42"},
		{"misses_L1", `lco_cache_misses_total\{layer="L1"\}\s+([0-9.]+)`, "8"},
		{"l2_hits", `lco_l2_hits_total\s+([0-9.]+)`, "7"},
		{"l2_misses", `lco_l2_misses_total\s+([0-9.]+)`, "3"},
		{"no_match", `nonexistent_metric_{}.*\s+([0-9.]+)`, ""},
		{"empty_body", `lco_cache_hits_total.*\s+([0-9.]+)`, ""}, // applied to empty
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := body
			if c.name == "empty_body" {
				b = []byte("")
			}
			if got := extractLCOMetric(b, c.pattern); got != c.want {
				t.Errorf("extractLCOMetric[%s]: got %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// ---------- main.go ----------

func TestEnv(t *testing.T) {
	t.Setenv("ADMIN_GO_TEST_ENV_KEY", "from-env")
	if got := env("ADMIN_GO_TEST_ENV_KEY", "fallback"); got != "from-env" {
		t.Errorf("env(set): got %q, want from-env", got)
	}
	if got := env("ADMIN_GO_NONEXISTENT_KEY_XYZ", "fallback"); got != "fallback" {
		t.Errorf("env(default): got %q, want fallback", got)
	}
	// 空值环境变量应回落到 default(env() 只判非空)
	t.Setenv("ADMIN_GO_EMPTY_KEY", "")
	if got := env("ADMIN_GO_EMPTY_KEY", "fallback"); got != "fallback" {
		t.Errorf("env(empty-val): got %q, want fallback", got)
	}
}

func TestValidPromPath(t *testing.T) {
	valid := []string{"query", "query_range", "alerts", "rules"}
	for _, p := range valid {
		if !validPromPath(p) {
			t.Errorf("validPromPath(%q): want true, got false", p)
		}
	}
	invalid := []string{"", "delete", "api/v1/query", "QUERY", "query/", "rules?x=1"}
	for _, p := range invalid {
		if validPromPath(p) {
			t.Errorf("validPromPath(%q): want false, got true", p)
		}
	}
}

func TestSummary(t *testing.T) {
	results := []ProbeResult{
		{Status: "UP"},
		{Status: "UP"},
		{Status: "DOWN"},
		{Status: "DOWN"},
		{Status: "UP"},
	}
	m := summary(results)
	if m["total"] != 5 {
		t.Errorf("total: got %d, want 5", m["total"])
	}
	if m["up"] != 3 {
		t.Errorf("up: got %d, want 3", m["up"])
	}
	if m["down"] != 2 {
		t.Errorf("down: got %d, want 2", m["down"])
	}
	// 空切片
	m2 := summary(nil)
	if m2["total"] != 0 || m2["up"] != 0 || m2["down"] != 0 {
		t.Errorf("empty summary: got %v, want all 0", m2)
	}
}

// ---------- ops.go ----------

func TestURLEncode(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"abc123", "abc123"},                              // 不编码
		{"abc-_.~", "abc-_.~"},                            // 保留字符原样
		{"a b", "a%20b"},                                  // 空格(rune=0x20)→ %20
		{"up{job=\"x\"}", "up%7Bjob%3D%22x%22%7D"},       // PromQL 特殊字符全编码(rune 码点十六进制)
		{"", ""},
		{"中文", "%4E2D%6587"}, // 实测:urlEncode 按 rune 码点编码(非 UTF-8 字节),中=U+4E2D 文=U+6587
	}
	for _, c := range cases {
		if got := urlEncode(c.in); got != c.want {
			t.Errorf("urlEncode(%q): got %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExtractPromValue(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"normal", `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1234.5,"0.42"]}]}}`, "0.42"},
		{"int_value", `{"value":[100,"7"]}`, "7"},
		{"no_value_key", `{"status":"success","data":{}}`, ""},
		{"value_no_closing_quote", `{"value":[123, "abc]}`, ""},
		{"empty", "", ""},
		{"missing_comma", `{"value":[1234"0.42"]}`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractPromValue([]byte(c.body)); got != c.want {
				t.Errorf("extractPromValue[%s]: got %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// ---------- runtime.go ----------

func TestExtractMetric(t *testing.T) {
	body := `# HELP foo_total counter
foo_total{job="x"} 99
bar_count 1`
	cases := []struct {
		name, pattern, want string
	}{
		{"match", `foo_total\{job="x"\}\s+([0-9.]+)`, "99"},
		{"no_capture_group", `foo_total`, ""}, // 无捕获组 → m[1] 越界返空
		{"no_match", `nonexistent\s+([0-9.]+)`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractMetric(body, c.pattern); got != c.want {
				t.Errorf("extractMetric[%s]: got %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// ---------- pulsar.go ----------

func TestParseIntSafe(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"42", 42},
		{"0", 0},
		{"", 0},
		{"not-a-number", 0},
		{"7abc", 7}, // Sscanf %d 读前导数字
		{"-3", -3},
	}
	for _, c := range cases {
		if got := parseIntSafe(c.in); got != c.want {
			t.Errorf("parseIntSafe(%q): got %d, want %d", c.in, got, c.want)
		}
	}
}

// ---------- debug_proxy.go ----------

func TestDebugSupportedEndpoints(t *testing.T) {
	cases := []struct {
		lang string
		want []string
	}{
		{"Java", []string{"env", "loggers", "threaddump", "heapdump", "beans", "mappings"}},
		{"Go", []string{"env", "loggers", "threaddump", "heapdump", "beans", "mappings"}},
		{"Python", []string{"env", "loggers", "beans", "mappings"}}, // 无 threaddump/heapdump
		{"Rust", []string{"env", "loggers", "beans", "mappings"}},
		{"UnknownLang", nil}, // 未知语言返 nil
		{"", nil},
	}
	for _, c := range cases {
		t.Run(c.lang, func(t *testing.T) {
			got := debugSupportedEndpoints(c.lang)
			if len(got) != len(c.want) {
				t.Fatalf("debugSupportedEndpoints(%q): got %v (len %d), want %v (len %d)",
					c.lang, got, len(got), c.want, len(c.want))
			}
			for i, e := range got {
				if e != c.want[i] {
					t.Errorf("debugSupportedEndpoints(%q)[%d]: got %q, want %q", c.lang, i, e, c.want[i])
				}
			}
		})
	}
}

// ---------- middleware.go ----------

func TestMiddlewareByName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"grafana", true},
		{"prometheus", true},
		{"nacos", true},
		{"sentinel", true},
		{"otel-collector", true}, // 含连字符
		{"grafana2", false},
		{"", false},
		{"Grafana", false}, // 大小写敏感
		{"evil-internal-host", false}, // SSRF 白名单外必拒
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ok := middlewareByName(c.name)
			if ok != c.want {
				t.Errorf("middlewareByName(%q): found=%v, want %v", c.name, ok, c.want)
			}
		})
	}
}
