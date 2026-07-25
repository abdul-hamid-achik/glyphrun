package artifacts

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func stageCompletedRun(t *testing.T) string {
	t.Helper()
	runDir := t.TempDir()
	stageCompletedRunAt(t, runDir)
	return runDir
}

func stageCompletedRunAt(t *testing.T, runDir string) {
	t.Helper()
	files := map[string]string{
		"run.json":          `{"status":"passed","endedAt":"2026-07-24T00:00:00Z"}`,
		"run.md":            "# Completed run\n",
		"screens/final.txt": "ready\n",
		"outcomes/check.md": "# Outcome: check\n\n- status: passed\n",
	}
	for relative, content := range files {
		path := filepath.Join(runDir, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func validPublishReceipt(sha string, size int64) string {
	return fmt.Sprintf(
		`{"version":"filecheap-publish/1","artifact_ref":{"$schema":"urn:filecheap.dev:artifact-ref:v1","version":1,"provider":"fcheap-cloud","uri":"fcheap://cloud/vaults/private/artifacts/art_1","artifact_id":"art_1","kind":"glyphrun.evidence-pack","producer":{"tool":"glyphrun","native_schema":"urn:glyphrun.dev:run:v1","entrypoint":"run.json"}},"sha256":%q,"size_bytes":%d,"verification":"server-sha256","published_at":"2026-07-24T00:00:00Z"}`,
		sha,
		size,
	)
}

func TestBuildEvidencePackIsDeterministicAndComplete(t *testing.T) {
	runDir := stageCompletedRun(t)
	first, err := buildEvidencePack(runDir)
	if err != nil {
		t.Fatal(err)
	}
	defer first.cleanup()
	second, err := buildEvidencePack(runDir)
	if err != nil {
		t.Fatal(err)
	}
	defer second.cleanup()

	firstBytes, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := os.ReadFile(second.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) || first.SHA256 != second.SHA256 || first.Size != second.Size {
		t.Fatal("same completed run produced different evidence-pack bytes")
	}

	got := readEvidencePack(t, first.Path)
	for relative, want := range map[string]string{
		"run.json":          `{"status":"passed","endedAt":"2026-07-24T00:00:00Z"}`,
		"run.md":            "# Completed run\n",
		"screens/final.txt": "ready\n",
		"outcomes/check.md": "# Outcome: check\n\n- status: passed\n",
	} {
		if got[relative] != want {
			t.Fatalf("pack entry %q = %q, want %q", relative, got[relative], want)
		}
	}
}

func TestBuildEvidencePackUsesPrivateTemporaryLocation(t *testing.T) {
	runDir := stageCompletedRun(t)
	pack, err := buildEvidencePack(runDir)
	if err != nil {
		t.Fatal(err)
	}
	defer pack.cleanup()
	dirInfo, err := os.Stat(filepath.Dir(pack.Path))
	if err != nil {
		t.Fatal(err)
	}
	fileInfo, err := os.Stat(pack.Path)
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("temporary directory mode = %o, want 700", dirInfo.Mode().Perm())
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("evidence pack mode = %o, want 600", fileInfo.Mode().Perm())
	}
	if strings.HasPrefix(pack.Path, runDir+string(filepath.Separator)) {
		t.Fatal("evidence package was written inside the source run")
	}
}

func TestBuildEvidencePackRejectsSymlinkAndSpecialFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink and Unix socket coverage is Unix-only")
	}
	t.Run("symlink", func(t *testing.T) {
		runDir := stageCompletedRun(t)
		if err := os.Symlink(filepath.Join(runDir, "run.md"), filepath.Join(runDir, "linked.md")); err != nil {
			t.Fatal(err)
		}
		if _, err := buildEvidencePack(runDir); err == nil || !strings.Contains(err.Error(), "rejects symlink") {
			t.Fatalf("buildEvidencePack() error = %v, want symlink rejection", err)
		}
	})
	t.Run("special file", func(t *testing.T) {
		runDir, err := os.MkdirTemp("/tmp", "glyphrun-pack-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(runDir) })
		if err := os.WriteFile(filepath.Join(runDir, "run.json"), []byte(`{"status":"passed","endedAt":"2026-07-24T00:00:00Z"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		socketPath := filepath.Join(runDir, "diagnostics.sock")
		listener, err := net.Listen("unix", socketPath)
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		if _, err := buildEvidencePack(runDir); err == nil || !strings.Contains(err.Error(), "rejects special file") {
			t.Fatalf("buildEvidencePack() error = %v, want special-file rejection", err)
		}
	})
}

func TestBuildEvidencePackRejectsCompressedPackageOverLimit(t *testing.T) {
	runDir := stageCompletedRun(t)
	data := incompressibleBytes(MaxFcheapEvidencePackBytes + 256*1024)
	if err := os.MkdirAll(filepath.Join(runDir, "raw"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "raw", "large.bin"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := buildEvidencePack(runDir); !errors.Is(err, errEvidencePackTooLarge) {
		t.Fatalf("buildEvidencePack() error = %v, want size-limit rejection", err)
	}
}

func TestPruneRunsFcheapPublishDeletesOnlyAfterCompleteMatchingPackage(t *testing.T) {
	root := t.TempDir()
	oldRun := filepath.Join(root, "2026-07-24T00-00-00Z-old")
	newRun := filepath.Join(root, "2026-07-24T00-00-01Z-new")
	stageCompletedRunAt(t, oldRun)
	stageCompletedRunAt(t, newRun)
	oldTime := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	newTime := oldTime.Add(time.Second)
	if err := os.Chtimes(oldRun, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newRun, newTime, newTime); err != nil {
		t.Fatal(err)
	}

	expected, err := buildEvidencePack(oldRun)
	if err != nil {
		t.Fatal(err)
	}
	receipt := validPublishReceipt(expected.SHA256, expected.Size)
	expected.cleanup()
	publishedCopy := filepath.Join(t.TempDir(), "published.tar.gz")
	script := writeScript(t, fmt.Sprintf("cp \"$2\" %q\nprintf '%%s\\n' '%s'", publishedCopy, receipt))

	report, err := PruneRuns(root, 1, ArchiveConfig{
		Enabled: true,
		Mode:    fcheapPublishMode,
		Command: script,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Pruned != 1 || report.Archived != 1 || len(report.ArchiveErrors) != 0 {
		t.Fatalf("PruneRuns() report = %#v, want one validated archive and prune", report)
	}
	if _, err := os.Stat(oldRun); !os.IsNotExist(err) {
		t.Fatalf("old run still exists after complete matching publish: %v", err)
	}
	published := readEvidencePack(t, publishedCopy)
	for _, relative := range []string{"run.json", "run.md", "screens/final.txt", "outcomes/check.md"} {
		if _, ok := published[relative]; !ok {
			t.Fatalf("published complete pack is missing %q", relative)
		}
	}
}

func TestPruneRunsFcheapPublishPreservesOversizedCompleteRun(t *testing.T) {
	root := t.TempDir()
	oldRun := filepath.Join(root, "2026-07-24T00-00-00Z-old")
	newRun := filepath.Join(root, "2026-07-24T00-00-01Z-new")
	stageCompletedRunAt(t, oldRun)
	stageCompletedRunAt(t, newRun)
	if err := os.MkdirAll(filepath.Join(oldRun, "raw"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldRun, "raw", "large.bin"), incompressibleBytes(MaxFcheapEvidencePackBytes+256*1024), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(oldRun, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newRun, oldTime.Add(time.Second), oldTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	invoked := filepath.Join(t.TempDir(), "publisher-invoked")
	script := writeScript(t, fmt.Sprintf("touch %q\nexit 1", invoked))
	report, err := PruneRuns(root, 1, ArchiveConfig{
		Enabled: true,
		Mode:    fcheapPublishMode,
		Command: script,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Pruned != 0 || report.Archived != 0 || len(report.ArchiveErrors) != 1 {
		t.Fatalf("PruneRuns() report = %#v, want preserved oversized run", report)
	}
	if report.ArchiveErrors[0] != errEvidencePackTooLarge.Error() {
		t.Fatalf("archive error = %q, want explicit package limit", report.ArchiveErrors[0])
	}
	if _, err := os.Stat(oldRun); err != nil {
		t.Fatalf("oversized run was not preserved: %v", err)
	}
	if _, err := os.Stat(invoked); !os.IsNotExist(err) {
		t.Fatal("fcheap was invoked even though complete package exceeded its bound")
	}
}

func TestFcheapPublishRejectsUnboundedRemoteRetention(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "2026-07-24T00-00-00Z-run")
	stageCompletedRunAt(t, runDir)
	invoked := filepath.Join(t.TempDir(), "publisher-invoked")
	script := writeScript(t, fmt.Sprintf("touch %q", invoked))
	result, err := ArchiveRun(ArchiveConfig{
		Enabled:       true,
		Mode:          fcheapPublishMode,
		Command:       script,
		RetentionDays: 32,
	}, runDir)
	if err == nil || result.OK || !strings.Contains(result.Message, "between 1 and 31") {
		t.Fatalf("ArchiveRun() = %#v, %v; want bounded retention failure", result, err)
	}
	if _, err := os.Stat(invoked); !os.IsNotExist(err) {
		t.Fatal("fcheap was invoked for an invalid remote retention policy")
	}
}

func incompressibleBytes(size int64) []byte {
	data := make([]byte, int(size))
	var state uint64 = 0x9e3779b97f4a7c15
	for i := range data {
		state ^= state << 7
		state ^= state >> 9
		state ^= state << 8
		data[i] = byte(state)
	}
	return data
}

func TestValidateFcheapReceiptBindsPackageAndCanonicalReference(t *testing.T) {
	sha := strings.Repeat("a", 64)
	size := int64(1234)
	valid := validPublishReceipt(sha, size)
	if err := validateFcheapReceipt([]byte(valid), sha, size); err != nil {
		t.Fatalf("valid receipt rejected: %v", err)
	}
	for name, body := range map[string]string{
		"wrong package hash": strings.Replace(valid, sha, strings.Repeat("b", 64), 1),
		"wrong package size": strings.Replace(valid, `"size_bytes":1234`, `"size_bytes":1235`, 1),
		"mismatched URI":     strings.Replace(valid, "/artifacts/art_1", "/artifacts/art_2", 1),
		"noncanonical URI":   strings.Replace(valid, "fcheap://cloud/vaults/private/artifacts/art_1", "fcheap://cloud/vaults/private/artifacts/art_1?token=secret", 1),
		"bad timestamp":      strings.Replace(valid, "2026-07-24T00:00:00Z", "24 July 2026", 1),
		"signed URL field":   strings.TrimSuffix(valid, "}") + `,"upload_url":"https://storage.invalid/object?token=secret"}`,
		"trailing JSON":      valid + `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateFcheapReceipt([]byte(body), sha, size); err == nil {
				t.Fatal("invalid receipt accepted")
			}
		})
	}
}

func TestFcheapEnvIncludesOnlyScopedPublisherCredential(t *testing.T) {
	t.Setenv("FILECHEAP_ARTIFACT_SERVICE_URL", "https://file.cheap")
	t.Setenv("FILECHEAP_INGEST_TOKEN", "scoped-ingest")
	t.Setenv("TVAULT_PASSPHRASE", "must-not-cross-boundary")
	t.Setenv("UNRELATED_API_TOKEN", "must-not-cross-boundary")
	joined := "\n" + strings.Join(fcheapEnv(), "\n") + "\n"
	for _, want := range []string{
		"FILECHEAP_ARTIFACT_SERVICE_URL=https://file.cheap",
		"FILECHEAP_INGEST_TOKEN=scoped-ingest",
	} {
		if !strings.Contains(joined, "\n"+want+"\n") {
			t.Fatalf("publisher environment is missing %q", want)
		}
	}
	for _, forbidden := range []string{"TVAULT_PASSPHRASE=", "UNRELATED_API_TOKEN="} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("publisher environment contains %q", forbidden)
		}
	}
}

func readEvidencePack(t *testing.T, path string) map[string]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	files := map[string]string{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		files[header.Name] = string(data)
	}
	return files
}
