package cli

import (
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"

	"github.com/abdul-hamid-achik/glyphrun/internal/config"
	"github.com/abdul-hamid-achik/glyphrun/internal/terminal/adapters/gote"
	"github.com/creack/pty"
	"github.com/spf13/cobra"
)

func newDoctorCommand(opts *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check local Glyphrun prerequisites",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			checks := []map[string]any{}
			add := func(name string, ok bool, detail string) {
				checks = append(checks, map[string]any{"name": name, "ok": ok, "detail": detail})
			}
			add("go version", true, goruntime.Version())
			add("platform", goruntime.GOOS == "darwin" || goruntime.GOOS == "linux", goruntime.GOOS+"/"+goruntime.GOARCH)
			ptmx, tty, ptyErr := pty.Open()
			if ptyErr == nil {
				_ = ptmx.Close()
				_ = tty.Close()
			}
			add("PTY availability", ptyErr == nil, detailOrOK(ptyErr))
			cfgPath, _ := config.FindConfig(".")
			add("config", cfgPath != "", cfgPath)
			rt, err := config.LoadRuntime(".", opts.configPath, opts.environment)
			if err != nil {
				add("config valid", false, err.Error())
			} else {
				add("config valid", true, rt.ConfigPath)
				root := opts.artifactRoot
				if root == "" {
					root = rt.Config.ArtifactRoot
				}
				if !filepath.IsAbs(root) {
					root = filepath.Join(rt.ProjectRoot, root)
				}
				err := os.MkdirAll(root, 0o755)
				add("artifact root writable", err == nil, root)
			}
			_, taskErr := os.Stat("Taskfile.yml")
			add("Taskfile", taskErr == nil, "Taskfile.yml")
			_, schemaErr := os.Stat("schemas/glyphrun.spec.v1.schema.json")
			add("schema files", schemaErr == nil, "schemas/glyphrun.spec.v1.schema.json")
			emulator := gote.New(80, 24)
			add("terminal emulator", emulator != nil, "internal terminal adapter")
			ok := true
			for _, check := range checks {
				if check["ok"] != true {
					ok = false
				}
			}
			value := map[string]any{"schemaVersion": 1, "ok": ok, "checks": checks}
			output, err := emit(format, value, func() string {
				md := "# Glyphrun Doctor\n\n"
				for _, check := range checks {
					mark := "PASS"
					if check["ok"] != true {
						mark = "FAIL"
					}
					md += fmt.Sprintf("- %s %s: %s\n", mark, check["name"], check["detail"])
				}
				return md
			})
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			if !ok {
				return exitError{code: 2}
			}
			return nil
		},
	}
}

func detailOrOK(err error) string {
	if err == nil {
		return "ok"
	}
	return err.Error()
}
