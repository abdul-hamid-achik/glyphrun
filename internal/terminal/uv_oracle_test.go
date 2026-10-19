package terminal

import (
	"bytes"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

// TestSimpleEmulatorMatchesUltravioletOnWideText renders frames containing wide
// characters, emoji, ZWJ sequences and combining marks with ultraviolet — the
// renderer behind Bubble Tea v2 — and feeds the bytes it writes to the
// emulator. Every cell must match the renderer's own buffer, character and
// width. The second frame is drawn as a diff, so the renderer reaches for
// cursor motion (HPA, CHA, CUF), ECH, ICH or IRM and REP around the wide cells;
// the TERM values select different capability sets in ultraviolet. In its
// default WcWidth mode ultraviolet drops combining marks from its cells, so
// joining is covered by TestSimpleEmulatorJoinsGraphemeClusters instead.
func TestSimpleEmulatorMatchesUltravioletOnWideText(t *testing.T) {
	const cols, rows = 32, 8
	frames := [][]string{
		{
			"雪 snow 🔩 bolt",
			"éclair ● ▲ × ─",
			"👨‍👩‍👧 family 👍\U0001F3FD",
			"日本語のテキスト end",
			"\x1b[44m 雪 \x1b[0m\x1b[31m🇲🇽 ❤️\x1b[0m tail",
			"ascii only line",
			// The next two lines are exactly 32 columns wide.
			"中文中文中文中文中文中文中文中文",
			"x雪yyyyyyyyyyyyyyyyyyyyyyyyyyy雨",
		},
		{
			"雪 SNOW 🔩 bolt!",
			"éclair ▲ ● × ─ more",
			"👨‍👩‍👧 fam 👍\U0001F3FD",
			"日本語 end",
			"\x1b[44m 雨 \x1b[0m\x1b[31m🇲🇽 ❤️\x1b[0m tail",
			"aaaaaaaaaaaaaaaaaaaaaaaa 雪",
			"中文中文中文 中文中文中文中文中a",
			"雪yyyyyyyyyyyyyyyyyyyyyyyyyyyy雨",
		},
	}
	for _, term := range []string{"xterm-256color", "ghostty", "screen-256color", "linux", "vt100"} {
		t.Run(term, func(t *testing.T) {
			var out bytes.Buffer
			r := uv.NewTerminalRenderer(&out, []string{"TERM=" + term})
			r.Resize(cols, rows)
			r.EnterAltScreen()
			em := NewEmulator(cols, rows)
			for i, frame := range frames {
				buf := uv.NewScreenBuffer(cols, rows) // WcWidth, as Bubble Tea v2 uses by default
				for y, line := range frame {
					uv.NewStyledString(line).Draw(buf, uv.Rect(0, y, cols, 1))
				}
				r.Render(buf.RenderBuffer)
				if err := r.Flush(); err != nil {
					t.Fatal(err)
				}
				if _, err := em.Feed(out.Bytes()); err != nil {
					t.Fatal(err)
				}
				out.Reset()
				scr := em.Screen()
				for y := range rows {
					for x := range cols {
						want := buf.CellAt(x, y)
						got := scr.Cell(x, y)
						wantChar := want.Content
						if want.Width > 0 && wantChar == "" {
							wantChar = " "
						}
						if got.Char != wantChar || got.Width != want.Width {
							t.Fatalf("frame %d cell (%d,%d) = {%q, %d}, renderer has {%q, %d}\nemulator row: %q",
								i, x, y, got.Char, got.Width, want.Content, want.Width, rowText(scr, y, cols))
						}
					}
				}
			}
		})
	}
}
