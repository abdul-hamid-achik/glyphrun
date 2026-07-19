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
// Only and Prefix are optional least-privilege filters applied client-side
// after resolution. A key is kept if it matches either filter (union
// semantics, matching tvault run --only/--prefix).
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

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail != "" {
			return nil, nil, fmt.Errorf("tvault env %s: %w (%s)", source, err, detail)
		}
		return nil, nil, fmt.Errorf("tvault env %s: %w", source, err)
	}

	var resolved map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &resolved); err != nil {
		return nil, nil, fmt.Errorf("parse tvault env json: %w", err)
	}

	resolved = filterSecrets(resolved, cfg)

	values := make([]string, 0, len(resolved))
	for _, v := range resolved {
		values = append(values, v)
	}
	sort.Strings(values)

	return resolved, values, nil
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
	return nil
}

// filterSecrets applies the Only allowlist and Prefix filter client-side.
// A key is kept if it matches either selector (union semantics).
func filterSecrets(all map[string]string, cfg *config.Secrets) map[string]string {
	if len(cfg.Only) == 0 && cfg.Prefix == "" {
		return all
	}
	onlySet := make(map[string]bool, len(cfg.Only))
	for _, k := range cfg.Only {
		onlySet[k] = true
	}
	selected := make(map[string]string)
	for k, v := range all {
		if onlySet[k] || (cfg.Prefix != "" && strings.HasPrefix(k, cfg.Prefix)) {
			selected[k] = v
		}
	}
	return selected
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
