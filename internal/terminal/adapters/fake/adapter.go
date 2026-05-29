package fake

import "github.com/abdul-hamid-achik/glyphrun/internal/terminal"

func New(cols, rows int) terminal.Emulator {
	return terminal.NewEmulator(cols, rows)
}
