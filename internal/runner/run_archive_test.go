package runner

import (
	"reflect"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/config"
)

// TestArchiveConfigFromConfig guards the config→artifacts archive
// translation that the runner owns (the artifacts package must not
// import internal/config). It copies Enabled/Command/Args verbatim and
// parses Timeout via artifacts.ParseArchiveTimeout; an unparseable
// timeout is dropped (left 0) so the downstream default applies, and an
// empty timeout is likewise 0.
func TestArchiveConfigFromConfig(t *testing.T) {
	cases := []struct {
		name string
		in   config.ArchiveConfig
		want artifacts.ArchiveConfig
	}{
		{
			name: "full block parses timeout",
			in:   config.ArchiveConfig{Enabled: true, Command: "fcheap", Args: []string{"store"}, Timeout: "90s"},
			want: artifacts.ArchiveConfig{Enabled: true, Command: "fcheap", Args: []string{"store"}, Timeout: 90 * time.Second},
		},
		{
			name: "bad timeout dropped to zero without panic",
			in:   config.ArchiveConfig{Enabled: true, Command: "fcheap", Args: []string{"store"}, Timeout: "bad"},
			want: artifacts.ArchiveConfig{Enabled: true, Command: "fcheap", Args: []string{"store"}},
		},
		{
			name: "empty timeout is zero",
			in:   config.ArchiveConfig{Enabled: false, Command: "fcheap", Timeout: ""},
			want: artifacts.ArchiveConfig{Command: "fcheap"},
		},
		{
			name: "zero value stays zero",
			in:   config.ArchiveConfig{},
			want: artifacts.ArchiveConfig{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := archiveConfigFromConfig(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
