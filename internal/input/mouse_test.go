package input

import (
	"strings"
	"testing"
)

func TestMouseBytesSGR(t *testing.T) {
	tests := []struct {
		name           string
		x, y           int
		button, action string
		want           string
	}{
		{name: "left click", x: 10, y: 5, button: "left", action: "click", want: "\x1b[<0;11;6M\x1b[<0;11;6m"},
		{name: "default click", x: 0, y: 0, want: "\x1b[<0;1;1M\x1b[<0;1;1m"},
		{name: "press only", x: 2, y: 3, button: "right", action: "press", want: "\x1b[<2;3;4M"},
		{name: "release only", x: 2, y: 3, button: "right", action: "release", want: "\x1b[<2;3;4m"},
		{name: "move", x: 4, y: 4, button: "left", action: "move", want: "\x1b[<32;5;5M"},
		{name: "wheel up", x: 1, y: 1, button: "wheelUp", action: "click", want: "\x1b[<64;2;2M"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MouseBytes(tc.x, tc.y, tc.button, tc.action, true)
			if err != nil {
				t.Fatalf("MouseBytes: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMouseBytesLegacy(t *testing.T) {
	// Legacy X10: ESC [ M, then (button+32), (x+1+32), (y+1+32) as bytes.
	got, err := MouseBytes(0, 0, "left", "press", false)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0x1b, '[', 'M', 32, 33, 33} // button 0+32, col 1+32, row 1+32
	if string(got) != string(want) {
		t.Errorf("legacy press: got %v, want %v", got, want)
	}
	// Release is reported as button 3 in legacy encoding.
	rel, _ := MouseBytes(0, 0, "left", "release", false)
	if rel[3] != byte(3+32) {
		t.Errorf("legacy release button byte = %d, want %d", rel[3], 3+32)
	}
}

func TestMouseBytesErrors(t *testing.T) {
	if _, err := MouseBytes(-1, 0, "left", "click", true); err == nil {
		t.Error("expected error for negative coordinate")
	}
	if _, err := MouseBytes(0, 0, "bogus", "click", true); err == nil {
		t.Error("expected error for unknown button")
	}
	if _, err := MouseBytes(0, 0, "left", "bogus", true); err == nil {
		t.Error("expected error for unknown action")
	}
}

func TestMouseBytesSGRShape(t *testing.T) {
	got, _ := MouseBytes(7, 9, "middle", "click", true)
	s := string(got)
	if !strings.HasPrefix(s, "\x1b[<1;8;10M") {
		t.Errorf("middle click should start with press at 8,10: %q", s)
	}
}
