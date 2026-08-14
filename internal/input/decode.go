package input

import (
	"bytes"
	"unicode/utf8"
)

// Token is one decoded keystroke or a run of printable text from a raw PTY input log.
type Token struct {
	Kind  string // "press" or "type"
	Value string
}

var knownPress = []struct {
	name string
	seq  []byte
}{
	{name: "shift+tab", seq: []byte("\x1b[Z")},
	{name: "delete", seq: []byte("\x1b[3~")},
	{name: "pageup", seq: []byte("\x1b[5~")},
	{name: "pagedown", seq: []byte("\x1b[6~")},
	{name: "up", seq: []byte("\x1b[A")},
	{name: "down", seq: []byte("\x1b[B")},
	{name: "right", seq: []byte("\x1b[C")},
	{name: "left", seq: []byte("\x1b[D")},
	{name: "home", seq: []byte("\x1b[H")},
	{name: "end", seq: []byte("\x1b[F")},
	{name: "enter", seq: []byte("\r")},
	{name: "tab", seq: []byte("\t")},
	{name: "esc", seq: []byte("\x1b")},
	{name: "backspace", seq: []byte("\x7f")},
}

// Tokens turns a captured stdin/PTY input stream into press/type tokens so a
// recorded session can be scaffolded into steps.
func Tokens(data []byte) []Token {
	var out []Token
	i := 0
	for i < len(data) {
		if data[i] == '\n' {
			// PTY enter is usually CR; ignore lone LF so we do not double-press.
			i++
			continue
		}
		if name, n := matchPress(data[i:]); n > 0 {
			out = append(out, Token{Kind: "press", Value: name})
			i += n
			continue
		}
		if data[i] < 0x20 && data[i] != 0 {
			// Ctrl-A .. Ctrl-Z
			out = append(out, Token{Kind: "press", Value: "ctrl+" + string(rune('a'+data[i]-1))})
			i++
			continue
		}
		r, size := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError && size == 1 {
			i++
			continue
		}
		if r == ' ' {
			out = append(out, Token{Kind: "press", Value: "space"})
			i += size
			continue
		}
		start := i
		i += size
		for i < len(data) {
			if _, n := matchPress(data[i:]); n > 0 {
				break
			}
			if data[i] < 0x20 || data[i] == 0x7f {
				break
			}
			nr, ns := utf8.DecodeRune(data[i:])
			if nr == utf8.RuneError && ns == 1 {
				break
			}
			if nr == ' ' {
				break
			}
			i += ns
		}
		out = append(out, Token{Kind: "type", Value: string(data[start:i])})
	}
	return out
}

func matchPress(data []byte) (string, int) {
	bestName := ""
	bestN := 0
	for _, k := range knownPress {
		if len(k.seq) > bestN && bytes.HasPrefix(data, k.seq) {
			bestName = k.name
			bestN = len(k.seq)
		}
	}
	return bestName, bestN
}
