package main

import "testing"

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
