package artifacts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// DefaultArchiveTimeout is used when ArchiveConfig.Timeout is empty.
const DefaultArchiveTimeout = 5 * time.Minute

// DefaultFcheapRetentionDays bounds remote demo evidence without requiring
// every existing config to repeat the policy.
const DefaultFcheapRetentionDays = 7

// MaxArchiveOutputBytes bounds diagnostic capture from external archive tools.
const MaxArchiveOutputBytes = 64 * 1024

// ArchiveConfig is the artifacts-package view of the config
// retention.archive block. It mirrors config.ArchiveConfig without
// importing internal/config (the artifacts package owns no runner or
// config state). The runner translates config.ArchiveConfig into this
// type before calling PruneRuns.
type ArchiveConfig struct {
	// Enabled gates archival. When false, pruned directories are
	// deleted locally as usual.
	Enabled bool
	// Mode "fcheap-publish" packages and publishes the complete bounded run
	// directory through `fcheap publish`, then validates its receipt before
	// deletion.
	Mode string
	// Command is the external binary invoked to archive a run dir.
	// The run directory path is appended as the final positional arg.
	// Required when Enabled is true.
	Command string
	// Args are fixed arguments passed to Command before the run dir.
	Args []string
	// Timeout is the max wall time for the archival command. Empty
	// means DefaultArchiveTimeout. A timeout is treated as archive
	// failure (the local dir is preserved).
	Timeout time.Duration
	// RetentionDays is the remote file.cheap lifetime. Zero selects the
	// seven-day default; explicit values must be between 1 and 31.
	RetentionDays int
}

const fcheapPublishMode = "fcheap-publish"

var (
	sha256Pattern   = regexp.MustCompile(`^[a-f0-9]{64}$`)
	remoteIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,159}$`)
)

// ArchiveResult captures the outcome of a single archival invocation.
type ArchiveResult struct {
	Path    string `json:"path" yaml:"path"`
	OK      bool   `json:"ok" yaml:"ok"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

type boundedArchiveOutput struct {
	buf       bytes.Buffer
	truncated bool
}

func (w *boundedArchiveOutput) Write(p []byte) (int, error) {
	remaining := MaxArchiveOutputBytes - w.buf.Len()
	if remaining > 0 {
		n := len(p)
		if n > remaining {
			n = remaining
		}
		_, _ = w.buf.Write(p[:n])
	}
	if len(p) > remaining {
		w.truncated = true
	}
	return len(p), nil
}

func (w *boundedArchiveOutput) String() string {
	out := w.buf.String()
	if w.truncated {
		out += "\n[glyphrun: archive command output truncated]"
	}
	return out
}

// ArchiveRun invokes the configured archival command for a single run
// directory. The command is run as:
//
//	<Command> <Args...> <runDir>
//
// with the run directory path appended as the final positional arg.
// Combined stdout+stderr is captured for diagnostics. A non-zero exit
// code, a timeout, or a missing binary is an error; the caller is
// expected to keep the local directory in all those cases (move
// semantics: delete only on success).
//
// ArchiveRun is pure with respect to run state — it does not touch the
// run dir itself, only shells out to the external command. The caller
// owns the delete decision.
func ArchiveRun(cfg ArchiveConfig, runDir string) (ArchiveResult, error) {
	if cfg.Command == "" {
		return ArchiveResult{Path: runDir, OK: false, Message: "archive command not configured"}, nil
	}
	if cfg.Mode == fcheapPublishMode {
		return archiveFcheapEvidencePack(cfg, runDir)
	}
	args := append(append([]string{}, cfg.Args...), runDir)
	cmd := exec.Command(cfg.Command, args...)
	cmd.Dir = filepath.Dir(runDir)
	var out boundedArchiveOutput
	cmd.Stdout = &out
	cmd.Stderr = &out

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultArchiveTimeout
	}
	if err := cmd.Start(); err != nil {
		return ArchiveResult{Path: runDir, OK: false, Message: fmt.Sprintf("start %s: %v", cfg.Command, err)}, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		msg := strings.TrimSpace(out.String())
		if err != nil {
			if msg == "" {
				msg = strings.TrimSpace(err.Error())
			}
			return ArchiveResult{Path: runDir, OK: false, Message: fmt.Sprintf("exit: %s", msg)}, err
		}
		return ArchiveResult{Path: runDir, OK: true, Message: msg}, nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done // reap the killed process
		return ArchiveResult{Path: runDir, OK: false, Message: fmt.Sprintf("timeout after %s", timeout)}, fmt.Errorf("archive %s: timeout after %s", runDir, timeout)
	}
}

// archiveFcheapEvidencePack packages the complete run directory before
// delegating the upload. A successful process exit alone is never evidence
// that the whole run is safely stored.
func archiveFcheapEvidencePack(cfg ArchiveConfig, runDir string) (ArchiveResult, error) {
	retentionDays := cfg.RetentionDays
	if retentionDays == 0 {
		retentionDays = DefaultFcheapRetentionDays
	}
	if retentionDays < 1 || retentionDays > 31 {
		err := fmt.Errorf("fcheap retention days must be between 1 and 31")
		return ArchiveResult{Path: runDir, Message: err.Error()}, err
	}
	pack, err := buildEvidencePack(runDir)
	if err != nil {
		if errors.Is(err, errEvidencePackTooLarge) {
			return ArchiveResult{Path: runDir, Message: errEvidencePackTooLarge.Error()}, err
		}
		return ArchiveResult{Path: runDir, Message: "build complete evidence pack failed"}, err
	}
	defer pack.cleanup()
	args := []string{"publish", pack.Path, "--json", "--content-type", "application/gzip", "--expires-in", fmt.Sprintf("%dh", retentionDays*24), "--kind", "glyphrun.evidence-pack", "--producer-tool", "glyphrun", "--native-schema", "urn:glyphrun.dev:run:v1", "--entrypoint", "run.json"}
	return runFcheapPublish(cfg, runDir, args, pack.SHA256, pack.Size)
}

func runFcheapPublish(cfg ArchiveConfig, runDir string, args []string, expectedSHA256 string, expectedSize int64) (ArchiveResult, error) {
	cmd := exec.Command(cfg.Command, args...)
	cmd.Dir = filepath.Dir(runDir)
	cmd.Env = fcheapEnv()
	var out boundedArchiveOutput
	cmd.Stdout = &out
	cmd.Stderr = &out
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultArchiveTimeout
	}
	if err := cmd.Start(); err != nil {
		return archiveFailure(runDir, "start fcheap publish", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			// The command can include remote error text. Keep diagnostics
			// credential-free and never surface its raw combined output.
			return ArchiveResult{Path: runDir, Message: "fcheap publish failed"}, err
		}
		if err := validateFcheapReceipt([]byte(out.String()), expectedSHA256, expectedSize); err != nil {
			return ArchiveResult{Path: runDir, Message: "fcheap publish returned an invalid receipt"}, err
		}
		return ArchiveResult{Path: runDir, OK: true, Message: "fcheap evidence pack published"}, nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return ArchiveResult{Path: runDir, Message: fmt.Sprintf("timeout after %s", timeout)}, fmt.Errorf("fcheap publish %s: timeout after %s", runDir, timeout)
	}
}

func archiveFailure(runDir, action string, err error) (ArchiveResult, error) {
	return ArchiveResult{Path: runDir, Message: action + " failed"}, fmt.Errorf("%s: %w", action, err)
}

// fcheapEnv grants the publisher only its scoped ingest contract. In
// particular it excludes TinyVault material and every unrelated parent secret.
func fcheapEnv() []string {
	allowed := map[string]bool{"PATH": true, "HOME": true, "TMPDIR": true, "TMP": true, "TEMP": true, "SystemRoot": true, "WINDIR": true, "ComSpec": true, "PATHEXT": true, "FILECHEAP_ARTIFACT_SERVICE_URL": true, "FILECHEAP_INGEST_TOKEN": true}
	var env []string
	for _, pair := range os.Environ() {
		key, _, ok := strings.Cut(pair, "=")
		if ok && allowed[key] {
			env = append(env, pair)
		}
	}
	return env
}

func validateFcheapReceipt(data []byte, expectedSHA256 string, expectedSize int64) error {
	var receipt struct {
		Version      string `json:"version"`
		SHA256       string `json:"sha256"`
		SizeBytes    int64  `json:"size_bytes"`
		Verification string `json:"verification"`
		PublishedAt  string `json:"published_at"`
		ArtifactRef  struct {
			Schema   string `json:"$schema"`
			Version  int    `json:"version"`
			Provider string `json:"provider"`
			URI      string `json:"uri"`
			Artifact string `json:"artifact_id"`
			Kind     string `json:"kind"`
			Producer *struct {
				Tool         string `json:"tool"`
				Version      string `json:"version,omitempty"`
				NativeSchema string `json:"native_schema,omitempty"`
				NativeID     string `json:"native_id,omitempty"`
				Entrypoint   string `json:"entrypoint,omitempty"`
			} `json:"producer,omitempty"`
		} `json:"artifact_ref"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return fmt.Errorf("decode fcheap receipt: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("decode fcheap receipt: trailing JSON")
	}
	if receipt.Version != "filecheap-publish/1" ||
		receipt.Verification != "server-sha256" ||
		receipt.SHA256 != expectedSHA256 ||
		receipt.SizeBytes != expectedSize ||
		expectedSize < 1 ||
		expectedSize > MaxFcheapEvidencePackBytes ||
		!sha256Pattern.MatchString(receipt.SHA256) ||
		receipt.ArtifactRef.Schema != "urn:filecheap.dev:artifact-ref:v1" ||
		receipt.ArtifactRef.Version != 1 ||
		receipt.ArtifactRef.Provider != "fcheap-cloud" ||
		receipt.ArtifactRef.Kind != "glyphrun.evidence-pack" ||
		receipt.ArtifactRef.Producer == nil ||
		receipt.ArtifactRef.Producer.Tool != "glyphrun" ||
		receipt.ArtifactRef.Producer.Version != "" ||
		receipt.ArtifactRef.Producer.NativeSchema != "urn:glyphrun.dev:run:v1" ||
		receipt.ArtifactRef.Producer.NativeID != "" ||
		receipt.ArtifactRef.Producer.Entrypoint != "run.json" ||
		!validFcheapCloudURI(receipt.ArtifactRef.URI, receipt.ArtifactRef.Artifact) ||
		!validCanonicalPublishedAt(receipt.PublishedAt) {
		return fmt.Errorf("invalid fcheap receipt")
	}
	return nil
}

func validFcheapCloudURI(rawURI, artifactID string) bool {
	if !remoteIDPattern.MatchString(artifactID) {
		return false
	}
	parsed, err := url.Parse(rawURI)
	if err != nil || parsed.Scheme != "fcheap" || parsed.Host != "cloud" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		parsed.RawPath != "" {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
	return len(parts) == 4 &&
		parts[0] == "vaults" &&
		remoteIDPattern.MatchString(parts[1]) &&
		parts[2] == "artifacts" &&
		parts[3] == artifactID
}

func validCanonicalPublishedAt(value string) bool {
	publishedAt, err := time.Parse(time.RFC3339, value)
	return err == nil && publishedAt.UTC().Format(time.RFC3339) == value
}

// archiveEnabled reports whether archival should run for a prune. It
// is a small helper so callers don't repeat the Enabled/Command guard.
func (c ArchiveConfig) archiveEnabled() bool {
	if !c.Enabled {
		return false
	}
	// A malformed fcheap-publish block must fail closed: ArchiveRun will
	// report the missing command and PruneRuns will preserve local evidence.
	if c.Mode == fcheapPublishMode {
		return true
	}
	return c.Command != ""
}

// ParseArchiveTimeout parses a duration string (e.g. "5m", "30s")
// into a time.Duration. Empty returns 0, which the caller maps to the
// default. An invalid string returns an error. This lives here so the
// config-to-artifacts translation stays in the artifacts package.
func ParseArchiveTimeout(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	return time.ParseDuration(s)
}
