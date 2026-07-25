package ptyrunner

import (
	"strings"
	"testing"
)

func TestSanitizedEnvExcludesAmbientAndTinyVaultSecrets(t *testing.T) {
	t.Setenv("TVAULT_PASSPHRASE", "never-pass-this-to-a-target")
	t.Setenv("FILECHEAP_INGEST_TOKEN", "never-pass-this-to-a-target")
	t.Setenv("UNRELATED_API_TOKEN", "never-pass-this-to-a-target")
	env := SanitizedEnv(map[string]string{
		"TVAULT_IDENTITY_KEY":    "also-private",
		"FILECHEAP_INGEST_TOKEN": "publisher-only",
		"RUN_SCOPED_KEY":         "explicit-value",
	})
	joined := "\n" + strings.Join(env, "\n") + "\n"
	for _, forbidden := range []string{"TVAULT_PASSPHRASE=", "TVAULT_IDENTITY_KEY=", "FILECHEAP_INGEST_TOKEN=", "UNRELATED_API_TOKEN="} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("sanitized target environment contains %q: %s", forbidden, joined)
		}
	}
	if !strings.Contains(joined, "RUN_SCOPED_KEY=explicit-value") {
		t.Fatalf("sanitized target environment lost explicit run value: %s", joined)
	}
}
