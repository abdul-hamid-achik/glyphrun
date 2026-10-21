package fake

import "github.com/abdul-hamid-achik/glyphrun/internal/terminal"

func New(cols, rows int) terminal.Emulator {
	return terminal.NewEmulator(cols, rows)
}

// NewWithOptions is New with an emulator configuration (the scrollback
// capacity). Both adapters build the same emulator, so they behave alike.
func NewWithOptions(cols, rows int, opts terminal.Options) terminal.Emulator {
	return terminal.NewEmulatorWithOptions(cols, rows, opts)
}
