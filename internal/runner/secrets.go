package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/artifacts"
	"github.com/abdul-hamid-achik/glyphrun/internal/config"
)

// resolveSecrets fetches secret values from a tvault env-group (or direct
// project) and returns them as a map suitable for merging into the run
// environment. The secret values are also collected into a sorted list
// for the per-run redactor so they are scrubbed from every artifact.
//
// The function shells out to `tvault env --format json` with the
// appropriate --group/--env or --project flags. The tvault binary must be
// on PATH (or specified via Secrets.Binary). TVAULT_DIR and
// TVAULT_PASSPHRASE (or TVAULT_IDENTITY_KEY) are expected to be in the
// environment, typically set by the config's env block — they are never
// read from the config file itself.
//
// Only and Prefix are passed to TinyVault. Glyphrun deliberately never asks
// TinyVault for every value and filters locally: values that are outside the
// run's scope must not enter this process in the first place.
func resolveSecrets(ctx context.Context, cfg *config.Secrets, env []string) (map[string]string, []string, error) {
	if cfg == nil {
		return nil, nil, nil
	}
	if err := validateSecrets(cfg); err != nil {
		return nil, nil, err
	}

	binary := cfg.Binary
	if binary == "" {
		binary = "tvault"
	}

	args := []string{"env", "--format", "json"}
	source := ""
	if cfg.Group != "" && cfg.Env != "" {
		args = append(args, "--group", cfg.Group, "--env", cfg.Env)
		source = cfg.Group + "/" + cfg.Env
	} else if cfg.Project != "" {
		args = append(args, "-p", cfg.Project)
		source = cfg.Project
	}
	for _, key := range cfg.Only {
		args = append(args, "--only", key)
	}
	if cfg.Prefix != "" {
		args = append(args, "--prefix", cfg.Prefix)
	}

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// TinyVault's stdout is secret-bearing JSON and stderr is controlled by
		// an external program. Neither may cross a Glyphrun error surface.
		return nil, nil, fmt.Errorf("tvault env %s: %w", source, err)
	}

	var resolved map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &resolved); err != nil {
		return nil, nil, fmt.Errorf("parse tvault env json: %w", err)
	}
	if err := validateResolvedSecretKeys(resolved, cfg); err != nil {
		return nil, nil, err
	}

	values := make([]string, 0, len(resolved))
	for _, v := range resolved {
		values = append(values, v)
	}
	sort.Strings(values)

	return resolved, values, nil
}

// validateResolvedSecretKeys fails closed if a provider ignores the selector.
// This is not a local filtering fallback: no unrequested value is returned to
// the caller or placed into a target environment.
func validateResolvedSecretKeys(resolved map[string]string, cfg *config.Secrets) error {
	only := make(map[string]bool, len(cfg.Only))
	for _, key := range cfg.Only {
		only[key] = true
	}
	for key := range resolved {
		if only[key] || (cfg.Prefix != "" && strings.HasPrefix(key, cfg.Prefix)) {
			continue
		}
		return fmt.Errorf("tvault env returned key outside declared selector")
	}
	return nil
}

// validateSecrets checks that the config block is well-formed: either
// group+env or project is set (not both), and the provider (if set) is
// tvault.
func validateSecrets(cfg *config.Secrets) error {
	if cfg == nil {
		return nil
	}
	provider := cfg.Provider
	if provider == "" {
		provider = "tvault"
	}
	if provider != "tvault" {
		return fmt.Errorf("secrets: unsupported provider %q (only \"tvault\" is supported)", provider)
	}
	hasGroup := cfg.Group != ""
	hasEnv := cfg.Env != ""
	hasProject := cfg.Project != ""
	if hasGroup && hasEnv && hasProject {
		return fmt.Errorf("secrets: group+env and project are mutually exclusive")
	}
	if !hasGroup && !hasEnv && !hasProject {
		return fmt.Errorf("secrets: must set either group+env or project")
	}
	if hasGroup && !hasEnv {
		return fmt.Errorf("secrets: group requires env")
	}
	if hasEnv && !hasGroup {
		return fmt.Errorf("secrets: env requires group")
	}
	if len(cfg.Only) == 0 && cfg.Prefix == "" {
		return fmt.Errorf("secrets: set only or prefix so TinyVault resolves a bounded secret set")
	}
	return nil
}

// envSlice converts a map to "KEY=VALUE" slices suitable for exec.Cmd.Env.
func envSlice(env map[string]string) []string {
	out := os.Environ()
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

// earlyError builds a RunResult for a failure that occurred before the run
// state (writer, PTY) was initialised — e.g. secret resolution failed. The
// result carries the error diagnostic and errorKind so the CLI surface can
// report it consistently as a structured envelope on stdout.
func earlyError(runDir string, started time.Time, specName, diagnostic string, errorKind artifacts.ErrorKind, exitCode int) artifacts.RunResult {
	ended := time.Now().UTC()
	return artifacts.RunResult{
		Schema:        artifacts.RunSchemaURI,
		SchemaVersion: 1,
		RunID:         makeRunID(started, specName),
		SpecName:      specName,
		Status:        artifacts.StatusErrored,
		ErrorKind:     errorKind,
		Diagnostic:    diagnostic,
		StartedAt:     started.Format(time.RFC3339Nano),
		EndedAt:       ended.Format(time.RFC3339Nano),
		DurationMS:    ended.Sub(started).Milliseconds(),
		RunDir:        runDir,
		ExitCode:      exitCode,
		Outcomes:      []artifacts.OutcomeResult{},
		Artifacts:     map[string]string{"failureDiagnostic": diagnostic},
		NextActions:   artifacts.NextActionsFor(errorKind, specName, "", ""),
	}
}
