package artifacts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const (
	artifactCopyBufferSize = 64 * 1024
	redactionWindowSize    = 256 * 1024
)

// ArtifactManifestEntry describes one file in a run artifact pack.
type ArtifactManifestEntry struct {
	RelativePath string `json:"relativePath" yaml:"relativePath"`
	Kind         string `json:"kind" yaml:"kind"`
	Bytes        int64  `json:"bytes" yaml:"bytes"`
	SHA256       string `json:"sha256" yaml:"sha256"`
}

// ArtifactManager is the sole authority for paths and files inside a run
// directory. It confines relative paths, creates parent directories, applies
// redaction to structured/text writes, serializes append-only writes, and
// records checksums for the deterministic manifest.
type ArtifactManager struct {
	root       string
	redactor   Redactor
	appendMu   sync.Mutex
	manifestMu sync.RWMutex
	manifest   map[string]ArtifactManifestEntry
	appends    map[string]*appendDigest
}

type appendDigest struct {
	hash  hash.Hash
	bytes int64
}

// NewArtifactManager creates an artifact authority rooted at runDir.
func NewArtifactManager(runDir string, redactor Redactor) (*ArtifactManager, error) {
	if strings.TrimSpace(runDir) == "" {
		return nil, errors.New("artifact run directory is empty")
	}
	root, err := filepath.Abs(runDir)
	if err != nil {
		return nil, fmt.Errorf("resolve artifact run directory: %w", err)
	}
	return &ArtifactManager{
		root:     filepath.Clean(root),
		redactor: redactor,
		manifest: make(map[string]ArtifactManifestEntry),
		appends:  make(map[string]*appendDigest),
	}, nil
}

// Root returns the absolute run directory managed by m.
func (m *ArtifactManager) Root() string { return m.root }

// Resolve returns the confined absolute path for a canonical run-relative path.
func (m *ArtifactManager) Resolve(rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", errors.New("artifact path is empty")
	}
	if filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" {
		return "", fmt.Errorf("artifact path %q must be run-relative", rel)
	}
	clean := filepath.Clean(rel)
	if clean == "." || clean != rel {
		return "", fmt.Errorf("artifact path %q is not canonical", rel)
	}
	for _, part := range strings.Split(filepath.ToSlash(clean), "/") {
		if part == ".." {
			return "", fmt.Errorf("artifact path %q escapes the run directory", rel)
		}
	}
	abs := filepath.Join(m.root, clean)
	inside, err := filepath.Rel(m.root, abs)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("artifact path %q escapes the run directory", rel)
	}
	return abs, nil
}

// EnsureRoot creates the run directory.
func (m *ArtifactManager) EnsureRoot() error {
	return os.MkdirAll(m.root, 0o755)
}

// EnsureDir creates a confined run-relative directory.
func (m *ArtifactManager) EnsureDir(rel string) error {
	path, err := m.Resolve(rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	return m.checkResolvedParent(path)
}

// WriteText writes redacted text atomically.
func (m *ArtifactManager) WriteText(rel, text string) error {
	return m.writeAtomic(rel, "text", 0o644, []byte(m.redactor.Text(text)), true)
}

// WriteRedactedBytes writes an in-memory payload through the configured
// redactor. It exists for already-bounded capture buffers such as PTY logs.
func (m *ArtifactManager) WriteRedactedBytes(rel, kind string, data []byte) error {
	return m.writeAtomic(rel, kind, 0o644, m.redactor.Bytes(data), true)
}

// WriteJSON writes stable, indented, newline-terminated redacted JSON.
func (m *ArtifactManager) WriteJSON(rel string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return m.writeAtomic(rel, "json", 0o644, m.redactor.Bytes(data), true)
}

// WriteYAML writes deterministic redacted YAML for existing Glyphrun
// compatibility artifacts.
func (m *ArtifactManager) WriteYAML(rel string, value any) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	return m.writeAtomic(rel, "yaml", 0o644, m.redactor.Bytes(data), true)
}

// WriteRedactedStream atomically writes logical chunks after redacting each
// complete chunk. Callers use one chunk per NDJSON record so secrets cannot be
// split by the writer's I/O buffer.
func (m *ArtifactManager) WriteRedactedStream(rel, kind string, produce func(emit func([]byte) error) error) error {
	return m.writeGenerated(rel, kind, 0o644, true, func(dst io.Writer) error {
		return produce(func(chunk []byte) error {
			_, err := dst.Write(m.redactor.Bytes(chunk))
			return err
		})
	})
}

// CopyBinary streams an opaque payload without loading it into memory and
// records the checksum of the bytes written.
func (m *ArtifactManager) CopyBinary(rel, kind string, src io.Reader) error {
	return m.writeGenerated(rel, kind, 0o644, true, func(dst io.Writer) error {
		buf := make([]byte, artifactCopyBufferSize)
		_, err := io.CopyBuffer(dst, struct{ io.Reader }{src}, buf)
		return err
	})
}

// CopyRedacted streams an arbitrary captured file through a bounded redaction
// window. Small files retain byte-for-byte legacy redaction behavior, while
// large files never require memory proportional to their size.
func (m *ArtifactManager) CopyRedacted(rel, kind string, src io.Reader) error {
	return m.writeGenerated(rel, kind, 0o644, true, func(dst io.Writer) error {
		readBuf := make([]byte, artifactCopyBufferSize)
		pending := make([]byte, 0, 2*redactionWindowSize)
		for {
			n, readErr := src.Read(readBuf)
			if n > 0 {
				pending = append(pending, readBuf[:n]...)
				for len(pending) > 2*redactionWindowSize {
					cut := redactionWindowSize
					if newline := bytes.LastIndexByte(pending[:cut], '\n'); newline >= 0 {
						cut = newline + 1
					}
					if _, err := dst.Write(m.redactor.Bytes(pending[:cut])); err != nil {
						return err
					}
					copy(pending, pending[cut:])
					pending = pending[:len(pending)-cut]
				}
			}
			if readErr != nil {
				if !errors.Is(readErr, io.EOF) {
					return readErr
				}
				if len(pending) > 0 {
					_, err := dst.Write(m.redactor.Bytes(pending))
					return err
				}
				return nil
			}
		}
	})
}

// RegisterFile adds a file produced by an external transform to the manifest
// using bounded checksum I/O.
func (m *ArtifactManager) RegisterFile(rel, kind string) error {
	path, err := m.Resolve(rel)
	if err != nil {
		return err
	}
	if err := m.checkResolvedParent(path); err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("artifact path %q is not a regular file", rel)
	}
	digest := sha256.New()
	buf := make([]byte, artifactCopyBufferSize)
	n, err := io.CopyBuffer(digest, struct{ io.Reader }{f}, buf)
	if err != nil {
		return err
	}
	m.record(rel, kind, n, digest.Sum(nil))
	return nil
}

// AppendJSONLine serializes append-only, redacted NDJSON writes.
func (m *ArtifactManager) AppendJSONLine(rel string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	data = m.redactor.Bytes(data)

	m.appendMu.Lock()
	defer m.appendMu.Unlock()
	path, err := m.prepareDestination(rel)
	if err != nil {
		return err
	}
	state, err := m.appendState(rel, path)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	n, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	_, _ = state.hash.Write(data)
	state.bytes += int64(len(data))
	m.record(rel, "ndjson", state.bytes, state.hash.Sum(nil))
	return nil
}

// Manifest returns entries sorted by relative path and detached from manager
// state so repeated calls are byte-stable.
func (m *ArtifactManager) Manifest() []ArtifactManifestEntry {
	m.manifestMu.RLock()
	entries := make([]ArtifactManifestEntry, 0, len(m.manifest))
	for _, entry := range m.manifest {
		entries = append(entries, entry)
	}
	m.manifestMu.RUnlock()
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].RelativePath == entries[j].RelativePath {
			return entries[i].Kind < entries[j].Kind
		}
		return entries[i].RelativePath < entries[j].RelativePath
	})
	return entries
}

// WriteManifest writes the supplied snapshot without recursively including the
// manifest file in itself.
func (m *ArtifactManager) WriteManifest(rel string, entries []ArtifactManifestEntry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return m.writeAtomic(rel, "json", 0o644, data, false)
}

func (m *ArtifactManager) writeGenerated(rel, kind string, mode os.FileMode, track bool, generate func(io.Writer) error) error {
	path, tmp, err := m.createTemp(rel, mode)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()
	digest := sha256.New()
	counter := &countingWriter{writer: io.MultiWriter(tmp, digest)}
	if err := generate(counter); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	committed = true
	if track {
		m.record(rel, kind, counter.bytes, digest.Sum(nil))
	}
	return nil
}

func (m *ArtifactManager) writeAtomic(rel, kind string, mode os.FileMode, data []byte, track bool) error {
	return m.writeGenerated(rel, kind, mode, track, func(dst io.Writer) error {
		_, err := dst.Write(data)
		return err
	})
}

func (m *ArtifactManager) createTemp(rel string, mode os.FileMode) (string, *os.File, error) {
	path, err := m.prepareDestination(rel)
	if err != nil {
		return "", nil, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".glyphrun-artifact-*")
	if err != nil {
		return "", nil, err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", nil, err
	}
	return path, tmp, nil
}

func (m *ArtifactManager) prepareDestination(rel string) (string, error) {
	path, err := m.Resolve(rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := m.checkResolvedParent(path); err != nil {
		return "", err
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("artifact path %q is a symbolic link", rel)
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return path, nil
}

func (m *ArtifactManager) checkResolvedParent(path string) error {
	root, err := filepath.EvalSymlinks(m.root)
	if err != nil {
		return err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, parent)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("artifact path %q escapes the run directory through a symbolic link", path)
	}
	return nil
}

func (m *ArtifactManager) appendState(rel, path string) (*appendDigest, error) {
	if state := m.appends[rel]; state != nil {
		return state, nil
	}
	state := &appendDigest{hash: sha256.New()}
	f, err := os.Open(path)
	if err == nil {
		buf := make([]byte, artifactCopyBufferSize)
		n, copyErr := io.CopyBuffer(state.hash, struct{ io.Reader }{f}, buf)
		closeErr := f.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		state.bytes = n
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	m.appends[rel] = state
	return state, nil
}

func (m *ArtifactManager) record(rel, kind string, bytes int64, sum []byte) {
	m.manifestMu.Lock()
	m.manifest[filepath.ToSlash(rel)] = ArtifactManifestEntry{
		RelativePath: filepath.ToSlash(rel),
		Kind:         kind,
		Bytes:        bytes,
		SHA256:       hex.EncodeToString(sum),
	}
	m.manifestMu.Unlock()
}

type countingWriter struct {
	writer io.Writer
	bytes  int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	w.bytes += int64(n)
	return n, err
}
