// Package doctor implements prerequisite checks shared by `glyph doctor`
// and the MCP `glyph_doctor` tool so both surfaces stay in lockstep.
package doctor

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal/adapters/gote"
	"github.com/creack/pty"
)

// Options configures doctor against a project.
type Options struct {
	ConfigPath   string
	ArtifactRoot string
	Environment  string
	// WorkDir is the project root to probe (default: ".").
	WorkDir string
}

// Result is the machine-readable doctor payload.
type Result struct {
	SchemaVersion int              `json:"schemaVersion"`
	OK            bool             `json:"ok"`
	Checks        []map[string]any `json:"checks"`
}

// Run executes the prerequisite matrix.
func Run(opts Options) Result {
	wd := opts.WorkDir
	if wd == "" {
		wd = "."
	}
	checks := []map[string]any{}
	add := func(name string, ok bool, detail string) {
		checks = append(checks, map[string]any{"name": name, "ok": ok, "detail": detail})
	}
	add("go version", true, runtime.Version())
	platformOK := runtime.GOOS == "darwin" || runtime.GOOS == "linux" || runtime.GOOS == "windows"
	platformDetail := runtime.GOOS + "/" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		platformDetail += " (ConPTY)"
	}
	add("platform", platformOK, platformDetail)

	switch runtime.GOOS {
	case "windows":
		add("PTY availability", true, "ConPTY backend (Windows 10 1809+)")
	default:
		ptmx, tty, ptyErr := pty.Open()
		if ptyErr == nil {
			_ = ptmx.Close()
			_ = tty.Close()
		}
		detail := "ok"
		if ptyErr != nil {
			detail = ptyErr.Error()
		}
		add("PTY availability", ptyErr == nil, detail)
	}

	cfgPath, _ := config.FindConfig(wd)
	if cfgPath == "" {
		add("config", true, "not found; using defaults")
	} else {
		add("config", true, cfgPath)
	}
	rt, err := config.LoadRuntime(wd, opts.ConfigPath, opts.Environment)
	if err != nil {
		add("config valid", false, err.Error())
	} else {
		configDetail := rt.ConfigPath
		if configDetail == "" {
			configDetail = "defaults"
		}
		add("config valid", true, configDetail)
		root := opts.ArtifactRoot
		if root == "" {
			root = rt.Config.ArtifactRoot
		}
		if !filepath.IsAbs(root) {
			root = filepath.Join(rt.ProjectRoot, root)
		}
		err := os.MkdirAll(root, 0o755)
		add("artifact root writable", err == nil, root)
	}
	_, taskErr := os.Stat(filepath.Join(wd, "Taskfile.yml"))
	add("Taskfile", taskErr == nil, "Taskfile.yml")
	_, schemaErr := os.Stat(filepath.Join(wd, "schemas/glyphrun.spec.v1.schema.json"))
	if schemaErr == nil {
		add("schema files", true, "schemas/glyphrun.spec.v1.schema.json")
	} else {
		add("schema files", true, "embedded")
	}
	emulator := gote.New(80, 24)
	add("terminal emulator", emulator != nil, "internal terminal adapter")
	ok := true
	for _, check := range checks {
		if check["ok"] != true {
			ok = false
		}
	}
	return Result{SchemaVersion: 1, OK: ok, Checks: checks}
}
