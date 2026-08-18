package dao

import (
	"testing"
)

func TestNullStr_Empty(t *testing.T) {
	v := nullStr("")
	if v != nil {
		t.Errorf("nullStr('') = %v, want nil", v)
	}
}

func TestNullStr_NonEmpty(t *testing.T) {
	v := nullStr("hello")
	s, ok := v.(string)
	if !ok {
		t.Fatalf("nullStr('hello') type = %T, want string", v)
	}
	if s != "hello" {
		t.Errorf("nullStr('hello') = %q, want %q", s, "hello")
	}
}

func TestNullStr_Whitespace(t *testing.T) {
	v := nullStr(" ")
	s, ok := v.(string)
	if !ok || s != " " {
		t.Errorf("nullStr(' ') = %v, want string(' ')", v)
	}
}
