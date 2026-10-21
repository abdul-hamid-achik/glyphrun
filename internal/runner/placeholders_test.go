package runner

import (
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
)

func TestResolveRuntimePlaceholders(t *testing.T) {
	s := &runState{namedArtifacts: map[string]artifacts.NamedArtifact{
		"report": {Path: "/run/artifacts/report/r.txt", RelativePath: "artifacts/report/r.txt"},
	}}
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{"path", "cat ${artifacts.report.path}", "cat /run/artifacts/report/r.txt", ""},
		{"relative path", "${artifacts.report.relativePath}", "artifacts/report/r.txt", ""},
		{"shell expansions pass through", `echo ${var#*:} ${HOME} ${x:-y} ${#arr}`, `echo ${var#*:} ${HOME} ${x:-y} ${#arr}`, ""},
		{"artifact key inside a shell default", `${v:-${artifacts.report.path}}`, `${v:-/run/artifacts/report/r.txt}`, ""},
		{"a span after a double dollar is left alone", `$${artifacts.report.path}`, `$${artifacts.report.path}`, ""},
		{"non-key text is left alone", `${vars.x} ${env.Y}`, `${vars.x} ${env.Y}`, ""},
		{"unknown artifact is an error", `${artifacts.nope.path}`, "", `artifact "nope"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.resolveRuntimePlaceholders(tc.in)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
