package input

import "testing"

func TestKeyBytes(t *testing.T) {
	tests := map[string]string{
		"enter":     "\r",
		"q":         "q",
		"shift+tab": "\x1b[Z",
		"pgup":      "\x1b[5~",
		"pgdown":    "\x1b[6~",
		"home":      "\x1b[H",
		"end":       "\x1b[F",
		"delete":    "\x1b[3~",
		"ctrl+c":    "\x03",
		"ctrl+u":    "\x15",
		"c-m":       "\r",
	}

	for key, want := range tests {
		t.Run(key, func(t *testing.T) {
			got, err := KeyBytes(key)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != want {
				t.Fatalf("%s = %q, want %q", key, string(got), want)
			}
		})
	}
}
