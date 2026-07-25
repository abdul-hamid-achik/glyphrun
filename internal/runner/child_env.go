package runner

import "github.com/abdul-hamid-achik/glyphrun/internal/ptyrunner"

// childEnv is shared by every target-adjacent subprocess. It begins with the
// explicit run environment and uses the PTY package's allowlisted parent
// environment, so command/script verifiers cannot inherit TinyVault unlock
// material or ambient credentials from the process running glyph.
func (s *runState) childEnv(extra map[string]string) []string {
	env := make(map[string]string, len(s.runtime.Env)+len(extra)+2)
	for key, value := range s.runtime.Env {
		env[key] = value
	}
	for key, value := range extra {
		env[key] = value
	}
	env["GLYPHRUN_RUN_DIR"] = s.writer.RunDir
	env["GLYPHRUN"] = "1"
	return ptyrunner.SanitizedEnv(env)
}
