package procmon

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSummarize_ReducesPeakAndMean(t *testing.T) {
	started := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	samples := []Sample{
		{At: started, CPU: 10, RSS: 100 * 1024 * 1024, Threads: 4},
		{At: started.Add(250 * time.Millisecond), CPU: 80, RSS: 300 * 1024 * 1024, Threads: 6},
		{At: started.Add(500 * time.Millisecond), CPU: 40, RSS: 200 * 1024 * 1024, Threads: 5},
	}
	s := Summarize(4242, "myapp", started, samples)
	if s.SampleCount != 3 {
		t.Errorf("SampleCount = %d, want 3", s.SampleCount)
	}
	if s.PeakCPU != 80 || s.PeakRSS != 300*1024*1024 || s.PeakThreads != 6 {
		t.Errorf("peak = cpu %.1f rss %d threads %d, want 80 / 300MiB / 6", s.PeakCPU, s.PeakRSS, s.PeakThreads)
	}
	if !approx(s.MeanCPU, 130.0/3.0) {
		t.Errorf("MeanCPU = %.4f, want %.4f", s.MeanCPU, 130.0/3.0)
	}
	wantMeanRSS := int64((100 + 300 + 200) * 1024 * 1024 / 3)
	if s.MeanRSS != wantMeanRSS {
		t.Errorf("MeanRSS = %d, want %d", s.MeanRSS, wantMeanRSS)
	}
	if s.DurationMS != 500 {
		t.Errorf("DurationMS = %d, want 500", s.DurationMS)
	}
}

func TestSummarize_EmptySamples(t *testing.T) {
	s := Summarize(1, "x", time.Time{}, nil)
	if s.SampleCount != 0 || s.PeakCPU != 0 || s.PeakRSS != 0 {
		t.Errorf("empty summary = %+v, want zeroed", s)
	}
}

func TestRenderProcessMarkdown_IncludesPeaksAndTree(t *testing.T) {
	s := Summary{PID: 99, Name: "app", SampleCount: 4, PeakCPU: 72.5, MeanCPU: 30, PeakRSS: 524288000, MeanRSS: 400000000, PeakThreads: 8}
	md := RenderProcessMarkdown(s, "app (pid 99) cpu 72% mem 500 MiB\n  helper (pid 100)")
	for _, want := range []string{"# Glyphrun Process Telemetry", "`app` (pid 99)", "peak CPU: 72.5%", "peak RSS:", "Process Tree", "helper (pid 100)"} {
		if !contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestRenderProcessMarkdown_NoSamples(t *testing.T) {
	md := RenderProcessMarkdown(Summary{PID: 7, SampleCount: 0}, "")
	if !contains(md, "No samples captured") {
		t.Errorf("expected no-samples note, got:\n%s", md)
	}
}

// fakeMonitor writes a shell script that fakes `monitor process|tree|profile`
// --json for the given canned payloads. Unix-only (the real monitor binary is
// macOS/Linux too).
func fakeMonitor(t *testing.T, processJSON, treeJSON string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake monitor shell script is Unix-only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-monitor")
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  process) cat <<'EOF'\n" + processJSON + "\nEOF\n;;\n" +
		"  tree) cat <<'EOF'\n" + treeJSON + "\nEOF\n;;\n" +
		"  *) echo '{}'\n;;\n" +
		"esac\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestClient_ProcessAndTree(t *testing.T) {
	bin := fakeMonitor(t,
		`{"pid":123,"name":"app","cpu_percent":42.5,"memory":1048576,"memory_percent":0,"threads":3,"user":"u","parent":1,"is_system":false,"is_protected":false}`,
		`[{"pid":123,"name":"app","cpu_percent":42.5,"memory":1048576,"threads":3,"parent":1,"children":[{"pid":124,"name":"child","cpu_percent":1,"memory":2048,"threads":2,"parent":123}]}]`)
	c := &Client{Bin: bin}

	info, err := c.Process(123)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if info.Name != "app" || info.CPUPercent != 42.5 || info.Memory != 1048576 || info.Threads != 3 {
		t.Errorf("Process info = %+v", info)
	}

	tree, err := c.Tree(123)
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	if len(tree) != 1 || tree[0].Name != "app" || len(tree[0].Children) != 1 || tree[0].Children[0].PID != 124 {
		t.Errorf("Tree = %+v", tree)
	}

	// SampleOnce reduces a Process reading to a Sample.
	sm, err := SampleOnce(c, 123, func() time.Time { return time.Time{} })
	if err != nil {
		t.Fatalf("SampleOnce: %v", err)
	}
	if sm.CPU != 42.5 || sm.RSS != 1048576 || sm.Threads != 3 {
		t.Errorf("Sample = %+v", sm)
	}
}

func TestClient_MissingBinary(t *testing.T) {
	c := &Client{Bin: "/no/such/monitor-binary"}
	if _, err := c.Process(1); err == nil {
		t.Fatalf("expected error for missing monitor binary, got nil")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func approx(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-6
}

func TestAssertMetrics(t *testing.T) {
	sum := Summary{SampleCount: 3, PeakCPU: 80, MeanCPU: 40, PeakRSS: 300 * 1024 * 1024, MeanRSS: 200 * 1024 * 1024}
	peakCpu := 90.0
	peakRss := int64(400 * 1024 * 1024)
	if ok, msg := AssertMetrics(sum, &peakCpu, nil, &peakRss, nil); !ok {
		t.Errorf("within-budget: got ok=false msg=%q", msg)
	}
	tightCpu := 50.0
	if ok, msg := AssertMetrics(sum, &tightCpu, nil, nil, nil); ok {
		t.Errorf("peak cpu 80 > 50 should fail, got ok=true msg=%q", msg)
	}
	tightRss := int64(100 * 1024 * 1024)
	if ok, msg := AssertMetrics(sum, nil, nil, &tightRss, nil); ok {
		t.Errorf("peak rss over budget should fail, got ok=true msg=%q", msg)
	}
	// No telemetry → clear failure, not a silent pass.
	if ok, msg := AssertMetrics(Summary{}, &peakCpu, nil, nil, nil); ok || !contains(msg, "no process telemetry") {
		t.Errorf("empty summary: ok=%v msg=%q, want false + no-telemetry message", ok, msg)
	}
}
