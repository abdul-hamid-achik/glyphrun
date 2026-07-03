package artifacts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeScript writes a /bin/sh script to a temp file, chmods it
// executable, and returns its path. Archive tests use this to stage
// exit-0, exit-1, sleep, and side-effect scripts deterministically on
// macOS. The body is appended after a `#!/bin/sh` shebang.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "arch-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintf(f, "#!/bin/sh\n%s\n", body); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(f.Name(), 0o755); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

// TestArchiveRun_NotConfigured covers the no-command guard: with an
// empty Command the runner must not shell out at all. The result is a
// benign "not configured" message with a nil error so the caller can
// treat it as a no-op rather than a failure.
func TestArchiveRun_NotConfigured(t *testing.T) {
	res, err := ArchiveRun(ArchiveConfig{}, "/tmp/whatever")
	if err != nil {
		t.Fatalf("expected nil error for unconfigured archive, got %v", err)
	}
	if res.OK {
		t.Errorf("expected OK=false for unconfigured archive")
	}
	if res.Message != "archive command not configured" {
		t.Errorf("message = %q, want %q", res.Message, "archive command not configured")
	}
}

// TestArchiveRun_SuccessExit0 verifies the happy path: the command
// runs, writes combined stdout/stderr, exits 0, and the trimmed
// output is returned as Message with OK=true and a nil error. The run
// directory path is appended as the final positional arg.
func TestArchiveRun_SuccessExit0(t *testing.T) {
	runDir := t.TempDir()
	// Echo a marker plus the first positional arg ($1 = runDir) to
	// confirm both the positional append and combined-output capture.
	script := writeScript(t, `printf 'archived %s' "$1"`)
	res, err := ArchiveRun(ArchiveConfig{
		Enabled: true,
		Command: script,
	}, runDir)
	if err != nil {
		t.Fatalf("expected nil error on exit 0, got %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got false (msg=%q)", res.Message)
	}
	want := "archived " + runDir
	if res.Message != want {
		t.Errorf("message = %q, want %q", res.Message, want)
	}
	if res.Path != runDir {
		t.Errorf("path = %q, want %q", res.Path, runDir)
	}
}

// TestArchiveRun_WithArgs confirms Args are placed between the
// command and the appended runDir positional.
func TestArchiveRun_WithArgs(t *testing.T) {
	runDir := t.TempDir()
	// Print all args joined by spaces; runDir must be the last one.
	script := writeScript(t, `printf '%s' "$*"`)
	res, err := ArchiveRun(ArchiveConfig{
		Enabled: true,
		Command: script,
		Args:    []string{"store", "fast"},
	}, runDir)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got false (msg=%q)", res.Message)
	}
	wantSuffix := "store fast " + runDir
	if !strings.HasSuffix(res.Message, wantSuffix) {
		t.Errorf("message = %q, want suffix %q", res.Message, wantSuffix)
	}
}

// TestArchiveRun_NonZeroExit covers the failure path: a non-zero
// exit returns OK=false, an "exit: ..." Message folding in the
// combined output, and a non-nil error.
func TestArchiveRun_NonZeroExit(t *testing.T) {
	runDir := t.TempDir()
	script := writeScript(t, `echo 'boom'; exit 1`)
	res, err := ArchiveRun(ArchiveConfig{
		Enabled: true,
		Command: script,
		Timeout: 5 * time.Second,
	}, runDir)
	if err == nil {
		t.Fatal("expected non-nil error on non-zero exit")
	}
	if res.OK {
		t.Errorf("expected OK=false on non-zero exit")
	}
	if !strings.HasPrefix(res.Message, "exit:") {
		t.Errorf("message = %q, want prefix %q", res.Message, "exit:")
	}
	if !strings.Contains(res.Message, "boom") {
		t.Errorf("message = %q, want to contain combined output", res.Message)
	}
}

// TestArchiveRun_MissingBinary guards the missing-binary case. The
// command path does not exist, so exec.LookPath fails at Start. The
// result is OK=false, a "start <cmd>: ..." message, and a non-nil
// error.
func TestArchiveRun_MissingBinary(t *testing.T) {
	runDir := t.TempDir()
	missing := filepath.Join(t.TempDir(), "no-such-binary")
	res, err := ArchiveRun(ArchiveConfig{
		Enabled: true,
		Command: missing,
		Timeout: 5 * time.Second,
	}, runDir)
	if err == nil {
		t.Fatal("expected non-nil error for missing binary")
	}
	if res.OK {
		t.Errorf("expected OK=false for missing binary")
	}
	if !strings.HasPrefix(res.Message, "start ") {
		t.Errorf("message = %q, want prefix %q", res.Message, "start ")
	}
	if !strings.Contains(res.Message, missing) {
		t.Errorf("message = %q, want to contain command path %q", res.Message, missing)
	}
}

// TestArchiveRun_Timeout covers the timeout path: a command that
// sleeps past the configured Timeout is killed, returns OK=false, a
// "timeout after <d>" message, and a non-nil error.
func TestArchiveRun_Timeout(t *testing.T) {
	runDir := t.TempDir()
	script := writeScript(t, `exec sleep 2`)
	start := time.Now()
	res, err := ArchiveRun(ArchiveConfig{
		Enabled: true,
		Command: script,
		Timeout: 200 * time.Millisecond,
	}, runDir)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected non-nil error on timeout")
	}
	if res.OK {
		t.Errorf("expected OK=false on timeout")
	}
	if !strings.HasPrefix(res.Message, "timeout after ") {
		t.Errorf("message = %q, want prefix %q", res.Message, "timeout after ")
	}
	// Sanity: the timeout should bound wall time well below the sleep.
	// Allow generous slack for slow CI runners.
	if elapsed > 5*time.Second {
		t.Errorf("timeout did not bound wall time: elapsed=%s", elapsed)
	}
}

// TestArchiveRun_DefaultTimeoutWhenZero confirms that a non-positive
// Timeout falls back to DefaultArchiveTimeout rather than hanging.
// We use a fast exit-0 script so the default is never actually
// reached; the point is that Timeout<=0 does not produce an immediate
// timeout error and the command runs to completion.
func TestArchiveRun_DefaultTimeoutWhenZero(t *testing.T) {
	runDir := t.TempDir()
	script := writeScript(t, `exit 0`)
	res, err := ArchiveRun(ArchiveConfig{
		Enabled: true,
		Command: script,
		// Timeout left at zero → must fall back to DefaultArchiveTimeout.
	}, runDir)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got false (msg=%q)", res.Message)
	}
}

// TestParseArchiveTimeout table-tests the duration parser: empty → 0
// (no error), a valid duration parses, an invalid string errors.
func TestParseArchiveTimeout(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"", 0, false},
		{"5m", 5 * time.Minute, false},
		{"30s", 30 * time.Second, false},
		{"1h30m", 90 * time.Minute, false},
		{"bad", 0, true},
		{"1xy", 0, true},
	}
	for _, tc := range tests {
		got, err := ParseArchiveTimeout(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseArchiveTimeout(%q): expected error, got %v", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseArchiveTimeout(%q): unexpected error %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseArchiveTimeout(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestArchiveConfig_ArchiveEnabled confirms the Enabled+Command
// guard: archive runs only when both Enabled is true and Command is
// non-empty.
func TestArchiveConfig_ArchiveEnabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  ArchiveConfig
		want bool
	}{
		{"disabled", ArchiveConfig{}, false},
		{"enabled no command", ArchiveConfig{Enabled: true}, false},
		{"command no enabled", ArchiveConfig{Command: "fcheap"}, false},
		{"both set", ArchiveConfig{Enabled: true, Command: "fcheap"}, true},
	}
	for _, tc := range tests {
		if got := tc.cfg.archiveEnabled(); got != tc.want {
			t.Errorf("%s: archiveEnabled() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
