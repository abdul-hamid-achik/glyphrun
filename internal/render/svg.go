// Package render turns a captured terminal screen into a deterministic SVG.
//
// The emulator owns the full cell grid (characters plus per-cell style), so a
// rendered screenshot can be a pure function of the snapshot: the same input
// produces byte-identical output, with no fonts to embed and no third-party
// dependencies. That determinism is the point — a rendered screen is evidence
// a human can read in a PR and a multimodal agent can inspect, and it survives
// being regenerated in CI without drift.
package render

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/glyphrun/internal/terminal"
)

// Theme is the color palette used to paint a screen. Empty cell colors fall
// back to Foreground/Background; named or indexed ANSI colors resolve through
// Palette.
type Theme struct {
	Background string
	Foreground string
	Cursor     string
	Palette    map[string]string
}

// Options controls the geometry of the rendered SVG. The defaults target a
// readable, compact screenshot; callers rarely need to change them.
type Options struct {
	Theme      Theme
	CellWidth  int
	CellHeight int
	FontSize   int
	Padding    int
	FontFamily string
	ShowCursor bool
}

// DefaultTheme is a dark palette close to common terminal defaults.
func DefaultTheme() Theme {
	return Theme{
		Background: "#1d1f21",
		Foreground: "#c5c8c6",
		Cursor:     "#c5c8c6",
		Palette:    defaultPalette(),
	}
}

// DefaultOptions returns the standard rendering geometry.
func DefaultOptions() Options {
	return Options{
		Theme:      DefaultTheme(),
		CellWidth:  10,
		CellHeight: 20,
		FontSize:   15,
		Padding:    10,
		FontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, 'DejaVu Sans Mono', monospace",
		ShowCursor: true,
	}
}

// SnapshotSVG renders a screen snapshot to an SVG document string. It is a
// pure function of (snap, opts): identical inputs yield identical output, with
// no clock or randomness involved.
func SnapshotSVG(snap terminal.ScreenSnapshot, opts Options) string {
	opts = withDefaults(opts)
	cols, rows := snap.Cols, snap.Rows
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	cw, ch, pad := opts.CellWidth, opts.CellHeight, opts.Padding
	width := pad*2 + cols*cw
	height := pad*2 + rows*ch
	baseline := opts.FontSize // baseline offset within a cell row

	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="`)
	b.WriteString(strconv.Itoa(width))
	b.WriteString(`" height="`)
	b.WriteString(strconv.Itoa(height))
	b.WriteString(`" viewBox="0 0 `)
	b.WriteString(strconv.Itoa(width))
	b.WriteByte(' ')
	b.WriteString(strconv.Itoa(height))
	b.WriteString(`" font-family="`)
	b.WriteString(opts.FontFamily)
	b.WriteString(`" font-size="`)
	b.WriteString(strconv.Itoa(opts.FontSize))
	b.WriteString(`">`)

	// Page background.
	b.WriteString(`<rect width="`)
	b.WriteString(strconv.Itoa(width))
	b.WriteString(`" height="`)
	b.WriteString(strconv.Itoa(height))
	b.WriteString(`" fill="`)
	b.WriteString(opts.Theme.Background)
	b.WriteString(`"/>`)

	at := cellGetter(snap, cols, rows)

	// Background pass: paint runs of cells whose effective background differs
	// from the page background, so the text pass draws on top of them.
	for y := 0; y < rows; y++ {
		x := 0
		for x < cols {
			st := effectiveStyle(at(x, y), opts.Theme)
			if st.bg == "" {
				x++
				continue
			}
			start := x
			for x < cols {
				next := effectiveStyle(at(x, y), opts.Theme)
				if next.bg != st.bg {
					break
				}
				x++
			}
			b.WriteString(`<rect x="`)
			b.WriteString(strconv.Itoa(pad + start*cw))
			b.WriteString(`" y="`)
			b.WriteString(strconv.Itoa(pad + y*ch))
			b.WriteString(`" width="`)
			b.WriteString(strconv.Itoa((x - start) * cw))
			b.WriteString(`" height="`)
			b.WriteString(strconv.Itoa(ch))
			b.WriteString(`" fill="`)
			b.WriteString(st.bg)
			b.WriteString(`"/>`)
		}
	}

	// Text pass: group consecutive same-style cells into a single <text> with
	// textLength so each run occupies exactly its grid width regardless of the
	// actual font metrics on the viewer's machine.
	for y := 0; y < rows; y++ {
		x := 0
		for x < cols {
			st := effectiveStyle(at(x, y), opts.Theme)
			start := x
			var runChars strings.Builder
			for x < cols {
				cell := at(x, y)
				next := effectiveStyle(cell, opts.Theme)
				if !next.equalText(st) {
					break
				}
				runChars.WriteString(cellChar(cell))
				x++
			}
			text := runChars.String()
			if strings.TrimRight(text, " ") == "" {
				continue // blank run: nothing to draw on top of the background
			}
			n := x - start
			b.WriteString(`<text x="`)
			b.WriteString(strconv.Itoa(pad + start*cw))
			b.WriteString(`" y="`)
			b.WriteString(strconv.Itoa(pad + y*ch + baseline))
			b.WriteString(`" textLength="`)
			b.WriteString(strconv.Itoa(n * cw))
			b.WriteString(`" lengthAdjust="spacingAndGlyphs" xml:space="preserve" fill="`)
			b.WriteString(st.fg)
			b.WriteString(`"`)
			if st.bold {
				b.WriteString(` font-weight="bold"`)
			}
			if st.italic {
				b.WriteString(` font-style="italic"`)
			}
			if st.underline {
				b.WriteString(` text-decoration="underline"`)
			}
			if st.dim {
				b.WriteString(` fill-opacity="0.6"`)
			}
			b.WriteByte('>')
			b.WriteString(xmlEscape(text))
			b.WriteString(`</text>`)
		}
	}

	// Cursor: a thin outline so the underlying glyph stays readable.
	if opts.ShowCursor && snap.Cursor.Visible &&
		snap.Cursor.X >= 0 && snap.Cursor.X < cols &&
		snap.Cursor.Y >= 0 && snap.Cursor.Y < rows {
		b.WriteString(`<rect x="`)
		b.WriteString(strconv.Itoa(pad + snap.Cursor.X*cw))
		b.WriteString(`" y="`)
		b.WriteString(strconv.Itoa(pad + snap.Cursor.Y*ch))
		b.WriteString(`" width="`)
		b.WriteString(strconv.Itoa(cw))
		b.WriteString(`" height="`)
		b.WriteString(strconv.Itoa(ch))
		b.WriteString(`" fill="none" stroke="`)
		b.WriteString(opts.Theme.Cursor)
		b.WriteString(`" stroke-width="1"/>`)
	}

	b.WriteString(`</svg>`)
	return b.String()
}

func withDefaults(opts Options) Options {
	def := DefaultOptions()
	if opts.CellWidth <= 0 {
		opts.CellWidth = def.CellWidth
	}
	if opts.CellHeight <= 0 {
		opts.CellHeight = def.CellHeight
	}
	if opts.FontSize <= 0 {
		opts.FontSize = def.FontSize
	}
	if opts.Padding < 0 {
		opts.Padding = def.Padding
	}
	if opts.FontFamily == "" {
		opts.FontFamily = def.FontFamily
	}
	if opts.Theme.Background == "" {
		opts.Theme.Background = def.Theme.Background
	}
	if opts.Theme.Foreground == "" {
		opts.Theme.Foreground = def.Theme.Foreground
	}
	if opts.Theme.Cursor == "" {
		opts.Theme.Cursor = def.Theme.Cursor
	}
	if opts.Theme.Palette == nil {
		opts.Theme.Palette = def.Theme.Palette
	}
	return opts
}

// cellGetter returns a bounds-safe accessor into the snapshot's row-major cell
// slice. Missing cells read as a blank cell so a short/garbled snapshot still
// renders rather than panicking.
func cellGetter(snap terminal.ScreenSnapshot, cols, rows int) func(x, y int) terminal.Cell {
	return func(x, y int) terminal.Cell {
		if x < 0 || y < 0 || x >= cols || y >= rows {
			return terminal.Cell{X: x, Y: y, Char: " ", Width: 1}
		}
		idx := y*cols + x
		if idx < 0 || idx >= len(snap.Cells) {
			return terminal.Cell{X: x, Y: y, Char: " ", Width: 1}
		}
		return snap.Cells[idx]
	}
}

func cellChar(c terminal.Cell) string {
	if c.Char == "" {
		return " "
	}
	return c.Char
}

// resolvedStyle is the rendering-ready form of a cell's style: colors are
// concrete (reverse already applied), and attributes are plain booleans.
type resolvedStyle struct {
	fg        string
	bg        string // "" means "page background; no rect needed"
	bold      bool
	italic    bool
	underline bool
	dim       bool
}

// equalText reports whether two styles render text identically. Background is
// excluded because the background pass groups runs separately.
func (s resolvedStyle) equalText(o resolvedStyle) bool {
	return s.fg == o.fg && s.bold == o.bold && s.italic == o.italic &&
		s.underline == o.underline && s.dim == o.dim
}

func effectiveStyle(cell terminal.Cell, theme Theme) resolvedStyle {
	st := cell.Style
	fg := resolveColor(st.Fg, theme, theme.Foreground)
	bg := resolveColor(st.Bg, theme, "")
	if st.Reverse {
		// Swap, materializing the page colors so the inversion is visible even
		// when the cell used the defaults.
		swappedFg := bg
		if swappedFg == "" {
			swappedFg = theme.Background
		}
		swappedBg := fg
		if swappedBg == "" {
			swappedBg = theme.Foreground
		}
		fg, bg = swappedFg, swappedBg
	}
	return resolvedStyle{
		fg:        fg,
		bg:        bg,
		bold:      st.Bold,
		italic:    st.Italic,
		underline: st.Underline,
		dim:       st.Dim,
	}
}

// resolveColor maps a cell color string to a concrete SVG color. Hex values
// pass through; named or indexed ANSI colors resolve via the theme palette;
// anything unknown (including empty) returns the fallback.
func resolveColor(value string, theme Theme, fallback string) string {
	if value == "" {
		return fallback
	}
	if strings.HasPrefix(value, "#") {
		return value
	}
	if hex, ok := theme.Palette[strings.ToLower(value)]; ok {
		return hex
	}
	return fallback
}

func xmlEscape(s string) string {
	if !strings.ContainsAny(s, "&<>\"") {
		return s
	}
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
	)
	return r.Replace(s)
}

// defaultPalette maps the standard ANSI color names, their 0-15 index aliases,
// and the full 16-255 xterm palette to hex. The emulator stores named colors
// for 0-15 and decimal indices for 16-255, so this resolves whatever it emits.
func defaultPalette() map[string]string {
	names := []string{
		"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
		"brightblack", "brightred", "brightgreen", "brightyellow",
		"brightblue", "brightmagenta", "brightcyan", "brightwhite",
	}
	hexes := []string{
		"#1d1f21", "#cc342b", "#198844", "#fba922", "#3971ed", "#a36ac7", "#3971ed", "#c5c8c6",
		"#969896", "#cc342b", "#198844", "#fba922", "#3971ed", "#a36ac7", "#3971ed", "#ffffff",
	}
	out := make(map[string]string, 256+len(names))
	for i, name := range names {
		out[name] = hexes[i]
		out[strconv.Itoa(i)] = hexes[i]
	}
	// The rest of the xterm 256-color space: the 6×6×6 color cube (16-231)
	// and the grayscale ramp (232-255).
	for i := 16; i <= 255; i++ {
		out[strconv.Itoa(i)] = xterm256Hex(i)
	}
	return out
}

// xterm256Hex returns the hex value for an xterm 256-color index in the
// 16-255 range using the standard cube + grayscale formula.
func xterm256Hex(i int) string {
	if i >= 16 && i <= 231 {
		n := i - 16
		conv := func(v int) int {
			if v == 0 {
				return 0
			}
			return 55 + v*40
		}
		return fmt.Sprintf("#%02x%02x%02x", conv(n/36), conv((n/6)%6), conv(n%6))
	}
	if i >= 232 && i <= 255 {
		level := 8 + (i-232)*10
		return fmt.Sprintf("#%02x%02x%02x", level, level, level)
	}
	return ""
}

// PaletteNames returns the sorted palette keys. Exposed for tests and tooling
// that needs a stable, deterministic ordering of the palette.
func PaletteNames(theme Theme) []string {
	names := make([]string, 0, len(theme.Palette))
	for name := range theme.Palette {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
