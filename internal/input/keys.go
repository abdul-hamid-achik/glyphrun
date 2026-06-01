package input

import (
	"fmt"
	"strings"
)

func KeyBytes(key string) ([]byte, error) {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "enter", "return":
		return []byte("\r"), nil
	case "tab":
		return []byte("\t"), nil
	case "shift+tab", "backtab":
		return []byte("\x1b[Z"), nil
	case "esc", "escape":
		return []byte("\x1b"), nil
	case "backspace":
		return []byte("\x7f"), nil
	case "delete", "del":
		return []byte("\x1b[3~"), nil
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
	case "pageup", "page-up", "pgup":
		return []byte("\x1b[5~"), nil
	case "pagedown", "page-down", "pgdown":
		return []byte("\x1b[6~"), nil
	case "home":
		return []byte("\x1b[H"), nil
	case "end":
		return []byte("\x1b[F"), nil
	default:
		if ctrl, ok := controlKeyBytes(normalized); ok {
			return ctrl, nil
		}
		if len([]rune(key)) == 1 {
			return []byte(key), nil
		}
		return nil, fmt.Errorf("unsupported key %q", key)
	}
}

func controlKeyBytes(key string) ([]byte, bool) {
	name, ok := strings.CutPrefix(key, "ctrl+")
	if !ok {
		name, ok = strings.CutPrefix(key, "c-")
	}
	if !ok {
		return nil, false
	}
	runes := []rune(name)
	if len(runes) != 1 {
		return nil, false
	}
	r := runes[0]
	if r >= 'a' && r <= 'z' {
		return []byte{byte(r-'a') + 1}, true
	}
	return nil, false
}

func BracketedPasteBytes(text string) []byte {
	return []byte("\x1b[200~" + text + "\x1b[201~")
}
