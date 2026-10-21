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
		{"escaped artifact key is literal", `echo $${artifacts.report.path}`, `echo ${artifacts.report.path}`, ""},
		{"escaped artifact key beside a real one", `$${artifacts.report.path} ${artifacts.report.path}`, `${artifacts.report.path} /run/artifacts/report/r.txt`, ""},
		{"escaped non-artifact text was unescaped by the parser and stays", `${vars.x}`, `${vars.x}`, ""},
		{"escaped artifact key needs no artifact", `$${artifacts.nope.path}`, `${artifacts.nope.path}`, ""},
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
