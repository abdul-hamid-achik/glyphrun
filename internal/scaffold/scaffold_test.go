package scaffold

import (
	"testing"

	"github.com/abdul-hamid-achik/glyphrun/internal/ptyrunner"
	"github.com/abdul-hamid-achik/glyphrun/internal/spec"
)

func TestPickReadyLine(t *testing.T) {
	tests := []struct {
		name   string
		screen string
		want   string
	}{
		{name: "first letter line", screen: "\n   \nHello there\nmore", want: "Hello there"},
		{name: "skips separators", screen: "-----\n=====\nWelcome\n", want: "Welcome"},
		{name: "skips short", screen: "ok\nok\nReady to go", want: "Ready to go"},
		{name: "empty", screen: "\n   \n----", want: ""},
		{name: "truncates long", screen: "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzMORE", want: "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwx"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := pickReadyLine(tc.screen); got != tc.want {
				t.Errorf("pickReadyLine(%q) = %q, want %q", tc.screen, got, tc.want)
			}
		})
	}
}

func TestDeriveSpecName(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{name: "binary path", argv: []string{"./bin/hello", "--flag"}, want: "hello_smoke"},
		{name: "with extension", argv: []string{"/usr/bin/my-app.sh"}, want: "my_app_smoke"},
		{name: "empty", argv: nil, want: "recorded_smoke"},
		{name: "leading digit", argv: []string{"7zip"}, want: "spec_7zip_smoke"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveSpecName(tc.argv); got != tc.want {
				t.Errorf("deriveSpecName(%v) = %q, want %q", tc.argv, got, tc.want)
			}
		})
	}
}

func TestBuildSpec(t *testing.T) {
	term := spec.Terminal{Cols: 80, Rows: 24, Profile: "xterm-256color"}

	t.Run("ready and clean exit", func(t *testing.T) {
		s, ready, needsEdit := buildSpec("app_smoke", []string{"./app"}, ".", term, "Welcome to app", ptyrunner.ExitState{Exited: true, ExitCode: 0})
		if needsEdit {
			t.Errorf("did not expect needsEdit")
		}
		if ready != "Welcome to app" {
			t.Errorf("ready = %q", ready)
		}
		if len(s.Outcomes) != 2 {
			t.Fatalf("expected 2 outcomes, got %d", len(s.Outcomes))
		}
		if s.Outcomes[1].ID != "clean_exit" || s.Outcomes[1].Verify.Process == nil || *s.Outcomes[1].Verify.Process.ExitCode != 0 {
			t.Errorf("expected clean_exit with exitCode 0, got %+v", s.Outcomes[1])
		}
	})

	t.Run("killed process skips clean_exit", func(t *testing.T) {
		s, _, _ := buildSpec("app_smoke", []string{"./app"}, ".", term, "Welcome to app", ptyrunner.ExitState{Exited: true, ExitCode: -1})
		for _, o := range s.Outcomes {
			if o.ID == "clean_exit" {
				t.Errorf("killed process should not produce clean_exit outcome")
			}
		}
	})

	t.Run("no signal needs edit", func(t *testing.T) {
		s, ready, needsEdit := buildSpec("app_smoke", []string{"./app"}, ".", term, "   \n----", ptyrunner.ExitState{Exited: true, ExitCode: -1})
		if !needsEdit {
			t.Errorf("expected needsEdit when nothing is observable")
		}
		if ready != "" {
			t.Errorf("expected empty ready, got %q", ready)
		}
		if len(s.Outcomes) != 1 {
			t.Fatalf("expected 1 placeholder outcome, got %d", len(s.Outcomes))
		}
	})
}
