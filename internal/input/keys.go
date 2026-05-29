package input

import (
	"fmt"
	"strings"
)

func KeyBytes(key string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "enter", "return":
		return []byte("\r"), nil
	case "tab":
		return []byte("\t"), nil
	case "esc", "escape":
		return []byte("\x1b"), nil
	case "backspace":
		return []byte("\x7f"), nil
	case "space":
		return []byte(" "), nil
	case "up":
		return []byte("\x1b[A"), nil
	case "down":
		return []byte("\x1b[B"), nil
	case "right":
		return []byte("\x1b[C"), nil
	case "left":
		return []byte("\x1b[D"), nil
	case "ctrl+c", "c-c":
		return []byte("\x03"), nil
	case "ctrl+d", "c-d":
		return []byte("\x04"), nil
	default:
		if len([]rune(key)) == 1 {
			return []byte(key), nil
		}
		return nil, fmt.Errorf("unsupported key %q", key)
	}
}
