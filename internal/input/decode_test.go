package input

import "testing"

func TestTokens(t *testing.T) {
	got := Tokens([]byte("hello\r\x1b[A"))
	if len(got) != 3 {
		t.Fatalf("got %#v", got)
	}
	if got[0] != (Token{Kind: "type", Value: "hello"}) {
		t.Errorf("type = %#v", got[0])
	}
	if got[1] != (Token{Kind: "press", Value: "enter"}) {
		t.Errorf("enter = %#v", got[1])
	}
	if got[2] != (Token{Kind: "press", Value: "up"}) {
		t.Errorf("up = %#v", got[2])
	}
}
