package runner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
)

// fakeMonitorBinary writes a shell script that fakes the `monitor` CLI's
// process/tree/profile subcommands for the procmon integration tests. The
// canned payloads are static — the tests assert wiring, not real metrics.
// Unix-only: the real monitor binary is macOS/Linux, and the fake is /bin/sh.
func fakeMonitorBinary(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("procmon integration tests are Unix-only (fake /bin/sh monitor)")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-monitor")
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  process) cat <<'EOF'\n" +
		`{"pid":1,"name":"faketarget","cpu_percent":12.5,"memory":1048576,"memory_percent":0,"threads":2,"user":"u","parent":0,"is_system":false,"is_protected":false}` + "\nEOF\n;;\n" +
		"  tree) cat <<'EOF'\nfaketarget (pid 1) cpu 12.5% mem 1.0 MiB\nEOF\n;;\n" +
		"  profile) cat <<'EOF'\n{\"pid\":1,\"type\":\"heap\",\"taken\":\"now\",\"text\":\"\",\"symbols\":[]}\nEOF\n;;\n" +
		"  *) echo \"{}\"\n;;\n" +
		"esac\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestRunSpecMonitorSampling proves `glyph run --monitor` end-to-end: with
// Procmon enabled and pointed at a fake monitor, the runner samples the
// target, writes diagnostics/process.{md,json}, and surfaces them on the
// result. Zero-cost when Procmon is nil is covered by every other runner
// test (none set Procmon).
func TestRunSpecMonitorSampling(t *testing.T) {
	fake := fakeMonitorBinary(t)
	dir := t.TempDir()
	specPath := filepath.Join(dir, "slow.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: monitor_sampling
intent: a target stays alive long enough to sample.
target:
  cmd: ["/bin/sh", "-lc", "printf 'ready\n'; sleep 0.3; exit 0"]
  cwd: "."
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait: { screen: { contains: "ready" }, timeoutMs: 2000 }
  - wait: { process: { exitCode: 0 }, timeoutMs: 2000 }
outcomes:
  - id: ok
    description: target exits cleanly
    verify: { process: { exitCode: 0 } }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := RunSpec(context.Background(), Options{
		SpecPath:     specPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
		Procmon:      &ProcmonConfig{Bin: fake, Interval: 50 * time.Millisecond},
	})
	if err != nil {
		t.Fatalf("RunSpec: %v", err)
	}
	if result.Artifacts["processTelemetry"] != "diagnostics/process.md" {
		t.Fatalf("processTelemetry artifact missing: %#v", result.Artifacts)
	}
	md, err := os.ReadFile(filepath.Join(result.RunDir, "diagnostics/process.md"))
	if err != nil {
		t.Fatalf("process.md not written: %v", err)
	}
	if !contains(string(md), "peak CPU") {
		t.Fatalf("process.md missing peak CPU:\n%s", md)
	}
	if _, err := os.Stat(filepath.Join(result.RunDir, "diagnostics/process.json")); err != nil {
		t.Fatalf("process.json not written: %v", err)
	}
}

// TestRunSpecMonitorStep proves a `monitor:` step captures a named artifact
// via the fake monitor binary, and that a `metrics:` outcome passes when the
// sampled summary is within budget.
func TestRunSpecMonitorStep(t *testing.T) {
	fake := fakeMonitorBinary(t)
	dir := t.TempDir()
	specPath := filepath.Join(dir, "monstep.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: monitor_step
intent: a monitor step captures the target and a metrics outcome asserts a budget.
target:
  cmd: ["/bin/sh", "-lc", "printf 'ready\n'; sleep 0.3; exit 0"]
  cwd: "."
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait: { screen: { contains: "ready" }, timeoutMs: 2000 }
  - monitor:
      saveAs: snap
      tree: true
  - wait: { process: { exitCode: 0 }, timeoutMs: 2000 }
outcomes:
  - id: rss_budget
    description: peak RSS stays under 1 GiB
    verify: { metrics: { peakRss: 1073741824 } }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := RunSpec(context.Background(), Options{
		SpecPath:     specPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
		Procmon:      &ProcmonConfig{Bin: fake, Interval: 50 * time.Millisecond},
	})
	if err != nil {
		t.Fatalf("RunSpec: %v", err)
	}
	if result.Status != artifacts.StatusPassed {
		t.Fatalf("status = %s, outcomes = %#v", result.Status, result.Outcomes)
	}
	art, ok := result.NamedArtifacts["snap"]
	if !ok || art.RelativePath != "monitors/snap.md" {
		t.Fatalf("snap named artifact missing: %#v", result.NamedArtifacts)
	}
	if _, err := os.Stat(filepath.Join(result.RunDir, "monitors/snap.md")); err != nil {
		t.Fatalf("monitors/snap.md not written: %v", err)
	}
}

// TestRunSpecMetricsVerifierFailsNoTelemetry confirms a `metrics:` outcome
// fails loudly (not silently) when process telemetry was not enabled — the
// actionable "run with --monitor" message — so a contributor doesn't ship a
// perf budget that can never be checked.
func TestRunSpecMetricsVerifierFailsNoTelemetry(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "no telemetry.yml")
	if err := os.WriteFile(specPath, []byte(`version: 1
name: metrics_no_telemetry
intent: a metrics outcome without --monitor fails with a clear message.
target:
  cmd: ["/bin/sh", "-lc", "printf 'ready\n'; sleep 0.1; exit 0"]
  cwd: "."
terminal:
  cols: 80
  rows: 24
  profile: xterm-256color
steps:
  - wait: { screen: { contains: "ready" }, timeoutMs: 2000 }
  - wait: { process: { exitCode: 0 }, timeoutMs: 2000 }
outcomes:
  - id: rss_budget
    description: peak RSS under 1 GiB
    verify: { metrics: { peakRss: 1073741824 } }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := RunSpec(context.Background(), Options{
		SpecPath:     specPath,
		ArtifactRoot: filepath.Join(dir, "runs"),
	})
	if err != nil {
		t.Fatalf("RunSpec: %v", err)
	}
	if result.Status != artifacts.StatusFailed {
		t.Fatalf("status = %s, want failed (no telemetry)", result.Status)
	}
	if !contains(result.Outcomes[0].Message, "no process telemetry") {
		t.Fatalf("expected no-telemetry message, got %q", result.Outcomes[0].Message)
	}
}
