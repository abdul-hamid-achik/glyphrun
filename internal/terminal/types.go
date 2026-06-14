package terminal

type Size struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

type Cursor struct {
	X       int  `json:"x"`
	Y       int  `json:"y"`
	Visible bool `json:"visible"`
}

type Style struct {
	Fg        string `json:"fg,omitempty"`
	Bg        string `json:"bg,omitempty"`
	Bold      bool   `json:"bold,omitempty"`
	Dim       bool   `json:"dim,omitempty"`
	Italic    bool   `json:"italic,omitempty"`
	Underline bool   `json:"underline,omitempty"`
	Reverse   bool   `json:"reverse,omitempty"`
}

type Cell struct {
	X     int    `json:"x"`
	Y     int    `json:"y"`
	Char  string `json:"char"`
	Width int    `json:"width"`
	Style Style  `json:"style"`
	// Link is the OSC 8 hyperlink URI attached to this cell, if any.
	Link string `json:"link,omitempty"`
}

type Region interface {
	Text() string
	Cells() []Cell
}

type Screen interface {
	Size() Size
	Cursor() Cursor
	Text() string
	Region(x, y, width, height int) Region
	Cell(x, y int) Cell
	Snapshot() ScreenSnapshot
}

type ScreenSnapshot struct {
	Cols   int    `json:"cols"`
	Rows   int    `json:"rows"`
	Cursor Cursor `json:"cursor"`
	Cells  []Cell `json:"cells"`
	Text   string `json:"text"`
}

type Frame struct {
	Seq            int64           `json:"seq"`
	Time           string          `json:"time"`
	Kind           string          `json:"kind"`
	Screen         *ScreenSnapshot `json:"screen,omitempty"`
	RawBytesBase64 string          `json:"rawBytesBase64,omitempty"`
}

type Emulator interface {
	Resize(cols, rows int) error
	Feed(data []byte) ([]Frame, error)
	Screen() Screen
}
