package artifacts

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

// MaxFcheapEvidencePackBytes matches fcheap publish's bounded-file contract.
const MaxFcheapEvidencePackBytes int64 = 2 * 1024 * 1024

// MaxEvidencePackEntries prevents a highly fragmented tree from consuming
// unbounded packaging time even when compression keeps its byte size small.
const MaxEvidencePackEntries = 10_000

var errEvidencePackTooLarge = errors.New("complete evidence pack exceeds the 2097152-byte publish limit")

type evidencePack struct {
	Path   string
	SHA256 string
	Size   int64
	dir    string
}

func (p evidencePack) cleanup() {
	if p.dir != "" {
		_ = os.RemoveAll(p.dir)
	}
}

type evidenceEntry struct {
	absolute string
	relative string
	isDir    bool
	size     int64
	mode     os.FileMode
	modTime  time.Time
}

type boundedPackWriter struct {
	dst     io.Writer
	written int64
	limit   int64
}

func (w *boundedPackWriter) Write(data []byte) (int, error) {
	remaining := w.limit - w.written
	if remaining <= 0 {
		return 0, errEvidencePackTooLarge
	}
	if int64(len(data)) > remaining {
		n, err := w.dst.Write(data[:remaining])
		w.written += int64(n)
		if err != nil {
			return n, err
		}
		return n, errEvidencePackTooLarge
	}
	n, err := w.dst.Write(data)
	w.written += int64(n)
	return n, err
}

// buildEvidencePack snapshots one completed run directory into a deterministic
// tar.gz. The temporary directory is private (0700), the package is 0600, and
// the writer aborts before it can exceed file.cheap's 2 MiB input bound.
func buildEvidencePack(runDir string) (pack evidencePack, err error) {
	if err := validateCompletedRun(runDir); err != nil {
		return evidencePack{}, err
	}
	entries, err := scanEvidenceEntries(runDir)
	if err != nil {
		return evidencePack{}, err
	}

	tempDir, err := os.MkdirTemp("", "glyphrun-evidence-*")
	if err != nil {
		return evidencePack{}, fmt.Errorf("create private evidence-pack directory: %w", err)
	}
	pack = evidencePack{
		Path: filepath.Join(tempDir, "glyphrun-evidence.tar.gz"),
		dir:  tempDir,
	}
	defer func() {
		if err != nil {
			pack.cleanup()
			pack = evidencePack{}
		}
	}()

	file, err := os.OpenFile(pack.Path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return pack, fmt.Errorf("create evidence pack: %w", err)
	}
	digest := sha256.New()
	bounded := &boundedPackWriter{
		dst:   io.MultiWriter(file, digest),
		limit: MaxFcheapEvidencePackBytes,
	}
	gzipWriter, err := gzip.NewWriterLevel(bounded, gzip.DefaultCompression)
	if err != nil {
		_ = file.Close()
		return pack, fmt.Errorf("create evidence gzip writer: %w", err)
	}
	gzipWriter.Header.ModTime = time.Unix(0, 0).UTC()
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)

	writeErr := writeEvidenceTar(tarWriter, entries)
	if closeErr := tarWriter.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if closeErr := gzipWriter.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if closeErr := file.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if bounded.written >= MaxFcheapEvidencePackBytes && errors.Is(writeErr, io.ErrShortWrite) {
		writeErr = errEvidencePackTooLarge
	}
	if writeErr != nil {
		if bounded.written >= MaxFcheapEvidencePackBytes {
			return pack, errEvidencePackTooLarge
		}
		return pack, fmt.Errorf("write complete evidence pack: %w", writeErr)
	}

	after, err := scanEvidenceEntries(runDir)
	if err != nil {
		return pack, err
	}
	if !sameEvidenceSnapshot(entries, after) {
		return pack, errors.New("run directory changed while building the complete evidence pack")
	}

	info, err := os.Stat(pack.Path)
	if err != nil {
		return pack, fmt.Errorf("inspect evidence pack: %w", err)
	}
	if info.Size() < 1 || info.Size() > MaxFcheapEvidencePackBytes {
		return pack, errEvidencePackTooLarge
	}
	pack.Size = info.Size()
	pack.SHA256 = hex.EncodeToString(digest.Sum(nil))
	return pack, nil
}

func validateCompletedRun(runDir string) error {
	info, err := os.Lstat(runDir)
	if err != nil {
		return fmt.Errorf("inspect run directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("evidence source must be a run directory")
	}
	runPath := filepath.Join(runDir, "run.json")
	runInfo, err := os.Lstat(runPath)
	if err != nil {
		return fmt.Errorf("inspect completed run.json: %w", err)
	}
	if !runInfo.Mode().IsRegular() {
		return errors.New("run.json must be a regular file")
	}
	data, err := os.ReadFile(runPath)
	if err != nil {
		return fmt.Errorf("read completed run.json: %w", err)
	}
	var completed struct {
		Status  string `json:"status"`
		EndedAt string `json:"endedAt"`
	}
	if err := json.Unmarshal(data, &completed); err != nil {
		return fmt.Errorf("parse completed run.json: %w", err)
	}
	switch completed.Status {
	case string(StatusPassed), string(StatusFailed), string(StatusErrored):
	default:
		return errors.New("run.json is not a completed evidence pack")
	}
	if _, err := time.Parse(time.RFC3339Nano, completed.EndedAt); err != nil {
		return errors.New("run.json is not a completed evidence pack")
	}
	return nil
}

func scanEvidenceEntries(runDir string) ([]evidenceEntry, error) {
	var entries []evidenceEntry
	err := filepath.WalkDir(runDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == runDir {
			return nil
		}
		if len(entries) >= MaxEvidencePackEntries {
			return fmt.Errorf("complete evidence pack exceeds the %d-entry limit", MaxEvidencePackEntries)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("complete evidence pack rejects symlink %q", safeEvidencePath(runDir, path))
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("complete evidence pack rejects special file %q", safeEvidencePath(runDir, path))
		}
		relative, err := filepath.Rel(runDir, path)
		if err != nil || relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("evidence path escapes the run directory")
		}
		entries = append(entries, evidenceEntry{
			absolute: path,
			relative: filepath.ToSlash(relative),
			isDir:    info.IsDir(),
			size:     info.Size(),
			mode:     info.Mode(),
			modTime:  info.ModTime(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].relative < entries[j].relative
	})
	return entries, nil
}

func writeEvidenceTar(writer *tar.Writer, entries []evidenceEntry) error {
	epoch := time.Unix(0, 0).UTC()
	for _, entry := range entries {
		header := &tar.Header{
			Name:       entry.relative,
			Mode:       0o644,
			Size:       entry.size,
			ModTime:    epoch,
			AccessTime: epoch,
			ChangeTime: epoch,
			Typeflag:   tar.TypeReg,
			Format:     tar.FormatPAX,
		}
		if entry.isDir {
			header.Name += "/"
			header.Mode = 0o755
			header.Size = 0
			header.Typeflag = tar.TypeDir
		}
		if err := writer.WriteHeader(header); err != nil {
			return err
		}
		if entry.isDir {
			continue
		}
		file, err := os.Open(entry.absolute)
		if err != nil {
			return err
		}
		info, statErr := file.Stat()
		current, lstatErr := os.Lstat(entry.absolute)
		if statErr != nil || lstatErr != nil || !info.Mode().IsRegular() ||
			!os.SameFile(info, current) || info.Size() != entry.size ||
			info.Mode() != entry.mode || !info.ModTime().Equal(entry.modTime) {
			_ = file.Close()
			return fmt.Errorf("evidence file changed before packaging: %q", entry.relative)
		}
		_, copyErr := io.CopyN(writer, file, entry.size)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func sameEvidenceSnapshot(before, after []evidenceEntry) bool {
	if len(before) != len(after) {
		return false
	}
	type comparableEntry struct {
		Relative string
		IsDir    bool
		Size     int64
		Mode     os.FileMode
		ModTime  time.Time
	}
	normalized := func(entries []evidenceEntry) []comparableEntry {
		out := make([]comparableEntry, len(entries))
		for i, entry := range entries {
			out[i] = comparableEntry{entry.relative, entry.isDir, entry.size, entry.mode, entry.modTime}
		}
		return out
	}
	return reflect.DeepEqual(normalized(before), normalized(after))
}

func safeEvidencePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return "<invalid>"
	}
	return filepath.ToSlash(relative)
}
