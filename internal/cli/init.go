package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

type initResult struct {
	SchemaVersion int      `json:"schemaVersion" yaml:"schemaVersion"`
	ProjectDir    string   `json:"projectDir" yaml:"projectDir"`
	ConfigPath    string   `json:"configPath" yaml:"configPath"`
	SpecPath      string   `json:"specPath" yaml:"specPath"`
	Created       []string `json:"created,omitempty" yaml:"created,omitempty"`
	Updated       []string `json:"updated,omitempty" yaml:"updated,omitempty"`
	Skipped       []string `json:"skipped,omitempty" yaml:"skipped,omitempty"`
	Next          []string `json:"next" yaml:"next"`
}

func newInitCommand(opts *globalOptions) *cobra.Command {
	var targetCmd string
	var buildCmd string
	var readyText string
	var quitKey string
	var name string
	var force bool
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Initialize Glyphrun files in a project",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := resolveFormat(opts.format)
			if err != nil {
				return exitError{code: 2, err: err}
			}
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			result, err := initProject(dir, initProjectOptions{
				TargetCmd: targetCmd,
				BuildCmd:  buildCmd,
				ReadyText: readyText,
				QuitKey:   quitKey,
				Name:      name,
				Force:     force,
			})
			if err != nil {
				return exitError{code: 2, err: err}
			}
			output, err := emitForCLI(cmd, opts, format, result, func() string {
				return renderInitMarkdown(result)
			})
			if err != nil {
				return exitError{code: 2, err: err}
			}
			cmd.Print(output)
			return nil
		},
	}
	cmd.Flags().StringVar(&targetCmd, "cmd", "./bin/app", "target command for the starter spec")
	cmd.Flags().StringVar(&buildCmd, "build", "", "optional setup command to build the target")
	cmd.Flags().StringVar(&readyText, "ready", "ready", "screen text the starter spec waits for")
	cmd.Flags().StringVar(&quitKey, "quit-key", "ctrl+c", "key used to quit the starter spec target")
	cmd.Flags().StringVar(&name, "name", "glyphrun_smoke", "starter spec name")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing Glyphrun config and starter spec")
	return cmd
}

type initProjectOptions struct {
	TargetCmd string
	BuildCmd  string
	ReadyText string
	QuitKey   string
	Name      string
	Force     bool
}

func initProject(dir string, opts initProjectOptions) (initResult, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return initResult{}, err
	}
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return initResult{}, err
	}
	if opts.TargetCmd == "" {
		opts.TargetCmd = "./bin/app"
	}
	if opts.ReadyText == "" {
		opts.ReadyText = "ready"
	}
	if opts.QuitKey == "" {
		opts.QuitKey = "ctrl+c"
	}
	if opts.Name == "" {
		opts.Name = "glyphrun_smoke"
	}
	result := initResult{
		SchemaVersion: 1,
		ProjectDir:    absDir,
		ConfigPath:    filepath.Join(absDir, "glyphrun.config.yml"),
		SpecPath:      filepath.Join(absDir, "specs", "glyphrun", "smoke.yml"),
	}
	if status, err := writeProjectFile(result.ConfigPath, defaultInitConfig(), opts.Force); err != nil {
		return initResult{}, err
	} else {
		addInitStatus(&result, status, relTo(absDir, result.ConfigPath))
	}
	if status, err := writeProjectFile(result.SpecPath, starterSpec(opts), opts.Force); err != nil {
		return initResult{}, err
	} else {
		addInitStatus(&result, status, relTo(absDir, result.SpecPath))
	}
	if status, err := updateGitignore(filepath.Join(absDir, ".gitignore")); err != nil {
		return initResult{}, err
	} else {
		addInitStatus(&result, status, ".gitignore")
	}
	result.Next = []string{
		"glyph spec verify " + initShellQuote(relTo(absDir, result.SpecPath)) + " --format json",
		"glyph run " + initShellQuote(relTo(absDir, result.SpecPath)) + " --format md --progress auto",
		"glyph context latest --format md",
	}
	return result, nil
}

func writeProjectFile(path string, content string, force bool) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil && !force {
		return "skipped", nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	status := "created"
	if _, err := os.Stat(path); err == nil {
		status = "updated"
	}
	return status, os.WriteFile(path, []byte(content), 0o644)
}

func updateGitignore(path string) (string, error) {
	const block = ".glyphrun/runs/\n.glyphrun/tmp/\n"
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "created", os.WriteFile(path, []byte(block), 0o644)
	}
	if err != nil {
		return "", err
	}
	text := string(data)
	var missing []string
	for _, line := range strings.Split(strings.TrimSpace(block), "\n") {
		if !containsGitignoreLine(text, line) {
			missing = append(missing, line)
		}
	}
	if len(missing) == 0 {
		return "skipped", nil
	}
	var b strings.Builder
	b.WriteString(text)
	if !strings.HasSuffix(text, "\n") {
		b.WriteByte('\n')
	}
	for _, line := range missing {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return "updated", os.WriteFile(path, []byte(b.String()), 0o644)
}

func containsGitignoreLine(text string, want string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

func defaultInitConfig() string {
	return `version: 1

artifactRoot: .glyphrun/runs
snapshotRoot: .glyphrun/snapshots

terminal:
  cols: 100
  rows: 30
  profile: xterm-256color
  normalize:
    trimRight: true
    normalizeLineEndings: true

artifacts:
  rawLog: true
  frames: true
  finalScreen: true
  snapshots: true
  agentContext: true
`
}

func starterSpec(opts initProjectOptions) string {
	var b strings.Builder
	fmt.Fprintf(&b, "version: 1\n")
	fmt.Fprintf(&b, "name: %s\n\n", opts.Name)
	fmt.Fprintf(&b, "intent: |\n")
	fmt.Fprintf(&b, "  a user can launch the terminal app, see its ready state, and quit cleanly.\n\n")
	fmt.Fprintf(&b, "target:\n")
	fmt.Fprintf(&b, "  cmd: [%s, %s, %s]\n", yamlString("/bin/sh"), yamlString("-lc"), yamlString(opts.TargetCmd))
	fmt.Fprintf(&b, "  cwd: %s\n\n", yamlString("."))
	fmt.Fprintf(&b, "terminal:\n")
	fmt.Fprintf(&b, "  cols: 100\n")
	fmt.Fprintf(&b, "  rows: 30\n")
	fmt.Fprintf(&b, "  profile: xterm-256color\n\n")
	if opts.BuildCmd != "" {
		fmt.Fprintf(&b, "preconditions:\n")
		fmt.Fprintf(&b, "  commands:\n")
		fmt.Fprintf(&b, "    - run: %s\n", yamlString(opts.BuildCmd))
		fmt.Fprintf(&b, "      timeoutMs: 30000\n\n")
	}
	fmt.Fprintf(&b, "steps:\n")
	fmt.Fprintf(&b, "  - wait:\n")
	fmt.Fprintf(&b, "      screen:\n")
	fmt.Fprintf(&b, "        contains: %s\n", yamlString(opts.ReadyText))
	fmt.Fprintf(&b, "      timeoutMs: 5000\n")
	fmt.Fprintf(&b, "  - snapshot: launch\n")
	fmt.Fprintf(&b, "  - press: %s\n", yamlString(opts.QuitKey))
	fmt.Fprintf(&b, "  - wait:\n")
	fmt.Fprintf(&b, "      process:\n")
	fmt.Fprintf(&b, "        exitCode: 0\n")
	fmt.Fprintf(&b, "      timeoutMs: 3000\n\n")
	fmt.Fprintf(&b, "outcomes:\n")
	fmt.Fprintf(&b, "  - id: ready_visible\n")
	fmt.Fprintf(&b, "    description: the app renders the expected ready state\n")
	fmt.Fprintf(&b, "    verify:\n")
	fmt.Fprintf(&b, "      screen:\n")
	fmt.Fprintf(&b, "        contains: %s\n", yamlString(opts.ReadyText))
	fmt.Fprintf(&b, "  - id: clean_exit\n")
	fmt.Fprintf(&b, "    description: the app exits cleanly\n")
	fmt.Fprintf(&b, "    verify:\n")
	fmt.Fprintf(&b, "      process:\n")
	fmt.Fprintf(&b, "        exitCode: 0\n")
	return b.String()
}

func yamlString(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(data)
}

func initShellQuote(arg string) string {
	if arg == "" {
		return "''"
	}
	for _, r := range arg {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			continue
		case strings.ContainsRune("@%_+=:,./-", r):
			continue
		default:
			return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
		}
	}
	return arg
}

func addInitStatus(result *initResult, status string, path string) {
	switch status {
	case "created":
		result.Created = append(result.Created, path)
	case "updated":
		result.Updated = append(result.Updated, path)
	case "skipped":
		result.Skipped = append(result.Skipped, path)
	}
}

func relTo(base string, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}

func renderInitMarkdown(result initResult) string {
	var b strings.Builder
	b.WriteString("# Glyphrun Init\n\n")
	fmt.Fprintf(&b, "- project: `%s`\n", result.ProjectDir)
	fmt.Fprintf(&b, "- config: `%s`\n", relTo(result.ProjectDir, result.ConfigPath))
	fmt.Fprintf(&b, "- spec: `%s`\n", relTo(result.ProjectDir, result.SpecPath))
	writeInitList(&b, "Created", result.Created)
	writeInitList(&b, "Updated", result.Updated)
	writeInitList(&b, "Skipped", result.Skipped)
	b.WriteString("\n## Next Commands\n\n")
	for _, command := range result.Next {
		fmt.Fprintf(&b, "- `%s`\n", command)
	}
	return b.String()
}

func writeInitList(b *strings.Builder, title string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## %s\n\n", title)
	for _, value := range values {
		fmt.Fprintf(b, "- `%s`\n", value)
	}
}
