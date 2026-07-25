package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/glyphrun/internal/config"
)

func TestValidateSecrets(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Secrets
		wantErr string
	}{
		{
			name:    "nil config is valid",
			cfg:     nil,
			wantErr: "",
		},
		{
			name:    "empty config errors",
			cfg:     &config.Secrets{},
			wantErr: "must set either group+env or project",
		},
		{
			name:    "group without env errors",
			cfg:     &config.Secrets{Group: "liftclub"},
			wantErr: "group requires env",
		},
		{
			name:    "env without group errors",
			cfg:     &config.Secrets{Env: "preview"},
			wantErr: "env requires group",
		},
		{
			name:    "group+env and project mutually exclusive",
			cfg:     &config.Secrets{Group: "liftclub", Env: "preview", Project: "liftclub-preview"},
			wantErr: "mutually exclusive",
		},
		{
			name:    "unsupported provider errors",
			cfg:     &config.Secrets{Provider: "doppler", Project: "app"},
			wantErr: "unsupported provider",
		},
		{
			name:    "selector is required",
			cfg:     &config.Secrets{Project: "app"},
			wantErr: "only or prefix",
		},
		{
			name:    "group+env with only is valid",
			cfg:     &config.Secrets{Group: "liftclub", Env: "preview", Only: []string{"DATABASE_URL"}},
			wantErr: "",
		},
		{
			name:    "project with prefix is valid",
			cfg:     &config.Secrets{Project: "liftclub-preview", Prefix: "APP_"},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSecrets(tt.cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestResolveSecretsNilIsNoop(t *testing.T) {
	resolved, values, err := resolveSecrets(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != nil {
		t.Fatalf("expected nil resolved, got %v", resolved)
	}
	if values != nil {
		t.Fatalf("expected nil values, got %v", values)
	}
}

func TestResolveSecretsInvalidConfig(t *testing.T) {
	_, _, err := resolveSecrets(context.Background(), &config.Secrets{}, nil)
	if err == nil {
		t.Fatal("expected error for empty secrets config")
	}
}

func TestResolveSecretsGroupEnv(t *testing.T) {
	// Create a fake tvault binary that outputs JSON.
	dir := t.TempDir()
	binPath := filepath.Join(dir, "tvault")
	script := `#!/bin/sh
# Emit a deterministic JSON payload for the env command.
echo '{"DATABASE_URL":"postgres://prod","STRIPE_SECRET_KEY":"fixture"}'
`
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Secrets{
		Group:  "liftclub",
		Env:    "preview",
		Binary: binPath,
		Only:   []string{"DATABASE_URL", "STRIPE_SECRET_KEY"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resolved, values, err := resolveSecrets(ctx, cfg, os.Environ())
	if err != nil {
		t.Fatalf("resolveSecrets failed: %v", err)
	}
	if len(resolved) != 2 {
		t.Fatalf("expected 2 resolved secrets, got %d: %v", len(resolved), resolved)
	}
	if resolved["DATABASE_URL"] != "postgres://prod" {
		t.Fatalf("DATABASE_URL = %q, want %q", resolved["DATABASE_URL"], "postgres://prod")
	}
	if resolved["STRIPE_SECRET_KEY"] != "fixture" {
		t.Fatalf("STRIPE_SECRET_KEY = %q, want %q", resolved["STRIPE_SECRET_KEY"], "fixture")
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 redaction values, got %d", len(values))
	}
}

func TestResolveSecretsWithOnlyFilter(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "tvault")
	// A fake TinyVault emits exactly the requested value. A separate test
	// confirms that unexpected values fail closed instead of being filtered.
	output := map[string]string{
		"DATABASE_URL": "postgres://prod",
	}
	data, _ := json.Marshal(output)
	script := "#!/bin/sh\necho '" + string(data) + "'\n"
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Secrets{
		Group:  "liftclub",
		Env:    "preview",
		Binary: binPath,
		Only:   []string{"DATABASE_URL"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resolved, values, err := resolveSecrets(ctx, cfg, os.Environ())
	if err != nil {
		t.Fatalf("resolveSecrets failed: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved secret (only filter), got %d: %v", len(resolved), resolved)
	}
	if _, ok := resolved["DATABASE_URL"]; !ok {
		t.Fatal("DATABASE_URL should be present")
	}
	if _, ok := resolved["OTHER_KEY"]; ok {
		t.Fatal("OTHER_KEY should be filtered out")
	}
	if len(values) != 1 {
		t.Fatalf("expected 1 redaction value, got %d", len(values))
	}
}

func TestResolveSecretsRejectsProviderValuesOutsideSelector(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "tvault")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\necho '{\"DATABASE_URL\":\"selected\",\"OTHER_KEY\":\"must-not-enter-run\"}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, err := resolveSecrets(context.Background(), &config.Secrets{Project: "app", Binary: binPath, Only: []string{"DATABASE_URL"}}, os.Environ())
	if err == nil || !contains(err.Error(), "outside declared selector") {
		t.Fatalf("resolveSecrets error = %v, want selector failure", err)
	}
}

func TestResolveSecretsPassesSelectorsToTinyVault(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	binPath := filepath.Join(dir, "tvault")
	script := "#!/bin/sh\nprintf '%s' \"$*\" > " + argsPath + "\necho '{\"DATABASE_URL\":\"selected-value\"}'\n"
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, err := resolveSecrets(context.Background(), &config.Secrets{
		Project: "myapp", Binary: binPath, Only: []string{"DATABASE_URL"}, Prefix: "APP_",
	}, os.Environ())
	if err != nil {
		t.Fatalf("resolveSecrets failed: %v", err)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"env", "--format json", "-p myapp", "--only DATABASE_URL", "--prefix APP_"} {
		if !contains(string(args), want) {
			t.Fatalf("TinyVault args %q missing %q", args, want)
		}
	}
}

func TestResolveSecretsProjectMode(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "tvault")
	script := `#!/bin/sh
echo '{"API_KEY":"key123"}'
`
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Secrets{
		Project: "myapp",
		Binary:  binPath,
		Only:    []string{"API_KEY"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resolved, _, err := resolveSecrets(ctx, cfg, os.Environ())
	if err != nil {
		t.Fatalf("resolveSecrets failed: %v", err)
	}
	if resolved["API_KEY"] != "key123" {
		t.Fatalf("API_KEY = %q, want %q", resolved["API_KEY"], "key123")
	}
}

func TestResolveSecretsBinaryFailure(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "tvault")
	script := `#!/bin/sh
echo "error: vault locked" >&2
exit 1
`
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Secrets{
		Group:  "liftclub",
		Env:    "preview",
		Binary: binPath,
		Only:   []string{"DATABASE_URL"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _, err := resolveSecrets(ctx, cfg, os.Environ())
	if err == nil {
		t.Fatal("expected error for failing tvault binary")
	}
	if !contains(err.Error(), "tvault env") {
		t.Fatalf("error should mention tvault env, got: %v", err)
	}
}

func TestResolveSecretsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "tvault")
	script := `#!/bin/sh
echo "not json at all"
`
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Secrets{
		Project: "myapp",
		Binary:  binPath,
		Only:    []string{"API_KEY"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _, err := resolveSecrets(ctx, cfg, os.Environ())
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !contains(err.Error(), "parse tvault env json") {
		t.Fatalf("error should mention json parse, got: %v", err)
	}
}

func TestEnvSlice(t *testing.T) {
	env := map[string]string{
		"FOO": "bar",
		"BAZ": "qux",
	}
	slice := envSlice(env)
	found := map[string]bool{}
	for _, pair := range slice {
		if pair == "FOO=bar" {
			found["FOO"] = true
		}
		if pair == "BAZ=qux" {
			found["BAZ"] = true
		}
	}
	if !found["FOO"] || !found["BAZ"] {
		t.Fatalf("envSlice missing expected entries, got: %v", slice)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
