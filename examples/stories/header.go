package main

// Header is the top chrome: optional brand, session title, and a status word
// aligned to the right. Selected/failed state is also a word, not color alone.
type Header struct {
	Brand  string
	Title  string
	Status string
}

func (h Header) Render(width int) string {
	left := titleStyle.Render(h.Title)
	if h.Brand != "" {
		left = brandStyle.Render(h.Brand) + "  " + left
	}
	right := statusStyle(h.Status).Render(h.Status)
	return splitBar(left, right, width)
}
