package artifacts

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func newTestManager(t *testing.T) *ArtifactManager {
	t.Helper()
	manager, err := NewArtifactManager(t.TempDir(), Redactor{})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.EnsureRoot(); err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestArtifactManagerRejectsTraversal(t *testing.T) {
	manager := newTestManager(t)
	for _, path := range []string{"../escape", "nested/../../escape", "/tmp/escape", "nested/../escape"} {
		if _, err := manager.Resolve(path); err == nil {
			t.Errorf("Resolve(%q) accepted an unconfined path", path)
		}
		if err := manager.WriteText(path, "unsafe"); err == nil {
			t.Errorf("WriteText(%q) accepted an unconfined path", path)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(manager.Root()), "escape")); !os.IsNotExist(err) {
		t.Fatalf("traversal created an outside file: %v", err)
	}
}

func TestArtifactManagerConcurrentAppendIntegrity(t *testing.T) {
	manager := newTestManager(t)
	const count = 200
	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if err := manager.AppendJSONLine("events.ndjson", map[string]int{"id": id}); err != nil {
				t.Errorf("append %d: %v", id, err)
			}
		}(i)
	}
	wg.Wait()

	path, err := manager.Resolve("events.ndjson")
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	seen := make(map[int]bool, count)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event map[string]int
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("corrupt event line %q: %v", scanner.Text(), err)
		}
		seen[event["id"]] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(seen) != count {
		t.Fatalf("unique events = %d, want %d", len(seen), count)
	}
}

func TestArtifactManagerManifestDeterministic(t *testing.T) {
	manager := newTestManager(t)
	if err := manager.WriteText("z.txt", "z\n"); err != nil {
		t.Fatal(err)
	}
	if err := manager.WriteJSON("a.json", map[string]int{"b": 2, "a": 1}); err != nil {
		t.Fatal(err)
	}
	first := manager.Manifest()
	second := manager.Manifest()
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("manifest changed between reads: %#v != %#v", first, second)
	}
	if len(first) != 2 || first[0].RelativePath != "a.json" || first[1].RelativePath != "z.txt" {
		t.Fatalf("manifest order = %#v", first)
	}
	for _, entry := range first {
		path, err := manager.Resolve(entry.RelativePath)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if entry.Bytes != int64(len(data)) || entry.SHA256 != hex.EncodeToString(sum[:]) {
			t.Fatalf("bad manifest entry for %s: %#v", entry.RelativePath, entry)
		}
	}
}

type boundedPatternReader struct {
	remaining int64
	maxRead   int
}

func (r *boundedPatternReader) Read(p []byte) (int, error) {
	if len(p) > artifactCopyBufferSize {
		return 0, io.ErrShortBuffer
	}
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := len(p)
	if int64(n) > r.remaining {
		n = int(r.remaining)
	}
	for i := range n {
		p[i] = byte('a' + i%26)
	}
	r.remaining -= int64(n)
	if n > r.maxRead {
		r.maxRead = n
	}
	return n, nil
}

func TestArtifactManagerCopyRedactedUsesBoundedIO(t *testing.T) {
	manager := newTestManager(t)
	const size = int64(8 * 1024 * 1024)
	source := &boundedPatternReader{remaining: size}
	if err := manager.CopyRedacted("artifacts/large.bin", "binary", source); err != nil {
		t.Fatal(err)
	}
	if source.maxRead > artifactCopyBufferSize {
		t.Fatalf("largest source read = %d, want <= %d", source.maxRead, artifactCopyBufferSize)
	}
	entries := manager.Manifest()
	if len(entries) != 1 || entries[0].Bytes != size {
		t.Fatalf("large copy manifest = %#v", entries)
	}
}
