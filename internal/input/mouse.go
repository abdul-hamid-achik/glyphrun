package input

import "fmt"

// MouseButton names accepted by a `mouse:` step.
const (
	MouseLeft      = "left"
	MouseMiddle    = "middle"
	MouseRight     = "right"
	MouseWheelUp   = "wheelUp"
	MouseWheelDown = "wheelDown"
)

// mouseButtonCode maps a button name to its base SGR/X10 button number.
var mouseButtonCode = map[string]int{
	MouseLeft:      0,
	MouseMiddle:    1,
	MouseRight:     2,
	MouseWheelUp:   64,
	MouseWheelDown: 65,
	"":             0, // default to left
}

// MouseBytes encodes a mouse event at the 0-based cell (x, y) for the given
// button and action. When sgr is true it emits the SGR (1006) encoding
// (ESC [ < b ; col ; row M/m, 1-based); otherwise the legacy X10 encoding
// (ESC [ M Cb Cx Cy with a +32 offset). action is one of:
//
//	click   press then release (default)
//	press   button down only
//	release button up only
//	move    motion with the button held
//
// Wheel buttons ignore action and emit a single press event.
func MouseBytes(x, y int, button, action string, sgr bool) ([]byte, error) {
	if x < 0 || y < 0 {
		return nil, fmt.Errorf("mouse coordinates must be non-negative")
	}
	base, ok := mouseButtonCode[button]
	if !ok {
		return nil, fmt.Errorf("unknown mouse button %q", button)
	}
	if action == "" {
		action = "click"
	}

	// Wheel events are momentary; a single press is the convention.
	if base == 64 || base == 65 {
		return mouseEvent(base, x, y, true, sgr), nil
	}

	switch action {
	case "press":
		return mouseEvent(base, x, y, true, sgr), nil
	case "release":
		return mouseEvent(base, x, y, false, sgr), nil
	case "move":
		return mouseEvent(base+32, x, y, true, sgr), nil // +32 = motion flag
	case "click":
		press := mouseEvent(base, x, y, true, sgr)
		release := mouseEvent(base, x, y, false, sgr)
		return append(press, release...), nil
	default:
		return nil, fmt.Errorf("unknown mouse action %q", action)
	}
}

// mouseEvent builds one mouse report. `press` distinguishes button-down from
// button-up (only meaningful in SGR; the legacy encoding signals release with
// button code 3).
func mouseEvent(code, x, y int, press, sgr bool) []byte {
	if sgr {
		final := byte('M')
		if !press {
			final = 'm'
		}
		return []byte(fmt.Sprintf("\x1b[<%d;%d;%d%c", code, x+1, y+1, final))
	}
	// Legacy X10: release is reported as button 3; coordinates and button are
	// offset by 32 (and capped to the historical 223 limit).
	cb := code
	if !press {
		cb = 3
	}
	return []byte{
		0x1b, '[', 'M',
		byte(clampLegacy(cb + 32)),
		byte(clampLegacy(x + 1 + 32)),
		byte(clampLegacy(y + 1 + 32)),
	}
}

func clampLegacy(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}
