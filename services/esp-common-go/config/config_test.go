package config

import (
	"os"
	"testing"
)

func TestGetenv_DefaultWhenEmpty(t *testing.T) {
	os.Unsetenv("ESP_TEST_GETENV_KEY")
	if got := Getenv("ESP_TEST_GETENV_KEY", "fallback"); got != "fallback" {
		t.Errorf("Getenv unset: got %q, want fallback", got)
	}
	os.Setenv("ESP_TEST_GETENV_KEY", "")
	t.Cleanup(func() { os.Unsetenv("ESP_TEST_GETENV_KEY") })
	if got := Getenv("ESP_TEST_GETENV_KEY", "fallback"); got != "fallback" {
		t.Errorf("Getenv empty: got %q, want fallback", got)
	}
}

func TestGetenv_ReturnsValueWhenSet(t *testing.T) {
	os.Setenv("ESP_TEST_GETENV_KEY", "hello")
	t.Cleanup(func() { os.Unsetenv("ESP_TEST_GETENV_KEY") })
	if got := Getenv("ESP_TEST_GETENV_KEY", "fallback"); got != "hello" {
		t.Errorf("Getenv set: got %q, want hello", got)
	}
}

func TestGetenvInt_DefaultWhenEmptyOrInvalid(t *testing.T) {
	cases := []struct {
		name string
		val  string
	}{
		{"empty", ""},
		{"non-numeric", "abc"},
		{"mixed", "12a3"},
		{"negative", "-5"},
		{"float", "1.5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.val == "" {
				os.Unsetenv("ESP_TEST_GETENVINT_KEY")
			} else {
				os.Setenv("ESP_TEST_GETENVINT_KEY", c.val)
			}
			t.Cleanup(func() { os.Unsetenv("ESP_TEST_GETENVINT_KEY") })
			if got := GetenvInt("ESP_TEST_GETENVINT_KEY", 42); got != 42 {
				t.Errorf("GetenvInt(%q): got %d, want 42", c.val, got)
			}
		})
	}
}

func TestGetenvInt_ParsesValid(t *testing.T) {
	cases := []struct {
		val  string
		want int
	}{
		{"0", 0},
		{"1", 1},
		{"12345", 12345},
		{"8080", 8080},
	}
	for _, c := range cases {
		os.Setenv("ESP_TEST_GETENVINT_KEY", c.val)
		t.Cleanup(func() { os.Unsetenv("ESP_TEST_GETENVINT_KEY") })
		if got := GetenvInt("ESP_TEST_GETENVINT_KEY", -1); got != c.want {
			t.Errorf("GetenvInt(%q): got %d, want %d", c.val, got, c.want)
		}
	}
}
