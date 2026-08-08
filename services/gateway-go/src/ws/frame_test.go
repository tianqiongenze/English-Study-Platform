package ws

import (
	"reflect"
	"testing"
)

func TestFrame_EncodeDecodeRoundTrip(t *testing.T) {
	cases := []Frame{
		{Type: TypeTextInput, Text: "I'd like to check in."},
		{Type: TypeAIReply, Text: "Sure! May I see your passport?", Rephrasing: "May I have your passport?"},
		{Type: TypeCorrection, Original: "I wants", Corrected: "I want", Phoneme: "/wɒnt/"},
		{Type: TypeHint, Options: []string{"Show passport", "Ask for window seat"}, Hints: []string{"passport", "window seat"}},
		{Type: TypeEnd, Summary: "You practiced airport check-in."},
	}
	for i, f := range cases {
		b, err := f.Encode()
		if err != nil {
			t.Fatalf("case %d encode: %v", i, err)
		}
		got, err := DecodeFrame(b)
		if err != nil {
			t.Fatalf("case %d decode: %v", i, err)
		}
		if !reflect.DeepEqual(got, f) {
			t.Fatalf("case %d round-trip mismatch:\n got=%+v\nwant=%+v", i, got, f)
		}
	}
}

func TestDecodeFrame_MissingType(t *testing.T) {
	_, err := DecodeFrame([]byte(`{"text":"hi"}`))
	if err == nil {
		t.Fatal("expected error for missing type")
	}
}

func TestDecodeFrame_InvalidJSON(t *testing.T) {
	_, err := DecodeFrame([]byte(`{not json`))
	if err == nil {
		t.Fatal("expected error for invalid json")
	}
}
