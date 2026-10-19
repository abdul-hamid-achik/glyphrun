package terminal

import "github.com/clipperhouse/uax29/v2/graphemes"

// TextColumns splits one line of screen text into the columns the emulator
// would lay it out in: each grapheme cluster in its first column and, for a
// wide (two-column) cluster, an empty string standing for its continuation
// cell. It serves code that has only text — text goldens, plain-text stories —
// and must address it by column. Joining the result gives back the line.
func TextColumns(line string) []string {
	var cols []string
	g := graphemes.FromString(line)
	for g.Next() {
		cluster := g.Value()
		switch width := clusterWidth(cluster); {
		case width == 0:
			// A cluster with no width of its own (a stray mark) belongs to the
			// character before it, if there is one.
			if i := lastLead(cols); i >= 0 {
				cols[i] += cluster
			}
		case width >= 2:
			cols = append(cols, cluster, "")
		default:
			cols = append(cols, cluster)
		}
	}
	return cols
}

// clusterWidth is the width the emulator gives a cluster: that of its first
// character with any width, capped at two columns.
func clusterWidth(cluster string) int {
	for _, r := range cluster {
		if w := cellWidth.RuneWidth(r); w > 0 {
			return min(w, 2)
		}
	}
	return 0
}

func lastLead(cols []string) int {
	for i := len(cols) - 1; i >= 0; i-- {
		if cols[i] != "" {
			return i
		}
	}
	return -1
}
