package spec

type Spec struct {
	Version       int           `yaml:"version" json:"version"`
	Name          string        `yaml:"name" json:"name"`
	ContractHash  string        `yaml:"contractHash,omitempty" json:"contractHash,omitempty"`
	Intent        string        `yaml:"intent" json:"intent"`
	Imports       []string      `yaml:"imports,omitempty" json:"imports,omitempty"`
	Target        Target        `yaml:"target" json:"target"`
	Terminal      Terminal      `yaml:"terminal" json:"terminal"`
	Preconditions Preconditions `yaml:"preconditions,omitempty" json:"preconditions,omitempty"`
	Steps         []Step        `yaml:"steps" json:"steps"`
	Outcomes      []Outcome     `yaml:"outcomes" json:"outcomes"`
	Normalize     *Normalize    `yaml:"normalize,omitempty" json:"normalize,omitempty"`
}

type ReusableAction struct {
	Version int    `yaml:"version,omitempty" json:"version,omitempty"`
	Name    string `yaml:"name" json:"name"`
	Steps   []Step `yaml:"steps" json:"steps"`
}

type Target struct {
	Cmd       []string          `yaml:"cmd" json:"cmd"`
	Cwd       string            `yaml:"cwd,omitempty" json:"cwd,omitempty"`
	Env       map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	TimeoutMS int               `yaml:"timeoutMs,omitempty" json:"timeoutMs,omitempty"`
}

type Terminal struct {
	Cols            int    `yaml:"cols,omitempty" json:"cols,omitempty"`
	Rows            int    `yaml:"rows,omitempty" json:"rows,omitempty"`
	Profile         string `yaml:"profile,omitempty" json:"profile,omitempty"`
	Color           string `yaml:"color,omitempty" json:"color,omitempty"`
	AlternateScreen string `yaml:"alternateScreen,omitempty" json:"alternateScreen,omitempty"`
}

type Preconditions struct {
	Commands []Command `yaml:"commands,omitempty" json:"commands,omitempty"`
}

type Command struct {
	Run       string `yaml:"run" json:"run"`
	Cwd       string `yaml:"cwd,omitempty" json:"cwd,omitempty"`
	TimeoutMS int    `yaml:"timeoutMs,omitempty" json:"timeoutMs,omitempty"`
}

type Step struct {
	When     *Verify     `yaml:"when,omitempty" json:"when,omitempty"`
	Press    string      `yaml:"press,omitempty" json:"press,omitempty"`
	Type     string      `yaml:"type,omitempty" json:"type,omitempty"`
	Paste    string      `yaml:"paste,omitempty" json:"paste,omitempty"`
	Send     *SendStep   `yaml:"send,omitempty" json:"send,omitempty"`
	Wait     *WaitStep   `yaml:"wait,omitempty" json:"wait,omitempty"`
	Resize   *ResizeStep `yaml:"resize,omitempty" json:"resize,omitempty"`
	Snapshot string      `yaml:"snapshot,omitempty" json:"snapshot,omitempty"`
	Use      string      `yaml:"use,omitempty" json:"use,omitempty"`
}

type SendStep struct {
	Bytes string `yaml:"bytes" json:"bytes"`
}

type WaitStep struct {
	Screen    *ScreenCondition  `yaml:"screen,omitempty" json:"screen,omitempty"`
	Process   *ProcessCondition `yaml:"process,omitempty" json:"process,omitempty"`
	Idle      *IdleCondition    `yaml:"idle,omitempty" json:"idle,omitempty"`
	TimeoutMS int               `yaml:"timeoutMs,omitempty" json:"timeoutMs,omitempty"`
}

type ResizeStep struct {
	Cols int `yaml:"cols" json:"cols"`
	Rows int `yaml:"rows" json:"rows"`
}

type Outcome struct {
	ID          string `yaml:"id" json:"id"`
	Description string `yaml:"description" json:"description"`
	Verify      Verify `yaml:"verify" json:"verify"`
}

type Verify struct {
	Screen   *ScreenCondition   `yaml:"screen,omitempty" json:"screen,omitempty"`
	Region   *RegionCondition   `yaml:"region,omitempty" json:"region,omitempty"`
	Cell     *CellCondition     `yaml:"cell,omitempty" json:"cell,omitempty"`
	Cursor   *CursorCondition   `yaml:"cursor,omitempty" json:"cursor,omitempty"`
	Process  *ProcessCondition  `yaml:"process,omitempty" json:"process,omitempty"`
	Snapshot *SnapshotCondition `yaml:"snapshot,omitempty" json:"snapshot,omitempty"`
	Command  *CommandCondition  `yaml:"command,omitempty" json:"command,omitempty"`
}

type ScreenCondition struct {
	Contains    string `yaml:"contains,omitempty" json:"contains,omitempty"`
	NotContains string `yaml:"notContains,omitempty" json:"notContains,omitempty"`
	Regex       string `yaml:"regex,omitempty" json:"regex,omitempty"`
}

type RegionCondition struct {
	X           int    `yaml:"x" json:"x"`
	Y           int    `yaml:"y" json:"y"`
	Width       int    `yaml:"width" json:"width"`
	Height      int    `yaml:"height" json:"height"`
	Contains    string `yaml:"contains,omitempty" json:"contains,omitempty"`
	NotContains string `yaml:"notContains,omitempty" json:"notContains,omitempty"`
	Regex       string `yaml:"regex,omitempty" json:"regex,omitempty"`
}

type CellCondition struct {
	X     int    `yaml:"x" json:"x"`
	Y     int    `yaml:"y" json:"y"`
	Char  string `yaml:"char,omitempty" json:"char,omitempty"`
	Style *Style `yaml:"style,omitempty" json:"style,omitempty"`
}

type Style struct {
	Fg        string `yaml:"fg,omitempty" json:"fg,omitempty"`
	Bg        string `yaml:"bg,omitempty" json:"bg,omitempty"`
	Bold      *bool  `yaml:"bold,omitempty" json:"bold,omitempty"`
	Dim       *bool  `yaml:"dim,omitempty" json:"dim,omitempty"`
	Italic    *bool  `yaml:"italic,omitempty" json:"italic,omitempty"`
	Underline *bool  `yaml:"underline,omitempty" json:"underline,omitempty"`
	Reverse   *bool  `yaml:"reverse,omitempty" json:"reverse,omitempty"`
}

type CursorCondition struct {
	X       int   `yaml:"x,omitempty" json:"x,omitempty"`
	Y       int   `yaml:"y,omitempty" json:"y,omitempty"`
	Visible *bool `yaml:"visible,omitempty" json:"visible,omitempty"`
}

type ProcessCondition struct {
	ExitCode *int  `yaml:"exitCode,omitempty" json:"exitCode,omitempty"`
	Exited   *bool `yaml:"exited,omitempty" json:"exited,omitempty"`
}

type IdleCondition struct {
	QuietForMS int `yaml:"quietForMs" json:"quietForMs"`
}

type SnapshotCondition struct {
	Name string `yaml:"name" json:"name"`
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty"`
}

type CommandCondition struct {
	Run       string `yaml:"run" json:"run"`
	Cwd       string `yaml:"cwd,omitempty" json:"cwd,omitempty"`
	TimeoutMS int    `yaml:"timeoutMs,omitempty" json:"timeoutMs,omitempty"`
}

type Normalize struct {
	TrimRight            *bool                 `yaml:"trimRight,omitempty" json:"trimRight,omitempty"`
	NormalizeLineEndings *bool                 `yaml:"normalizeLineEndings,omitempty" json:"normalizeLineEndings,omitempty"`
	StripAnsiTitle       *bool                 `yaml:"stripAnsiTitle,omitempty" json:"stripAnsiTitle,omitempty"`
	Replace              []NormalizeReplace    `yaml:"replace,omitempty" json:"replace,omitempty"`
	IgnoreRegions        []NormalizeIgnoreArea `yaml:"ignoreRegions,omitempty" json:"ignoreRegions,omitempty"`
}

type NormalizeReplace struct {
	Regex string `yaml:"regex" json:"regex"`
	With  string `yaml:"with" json:"with"`
}

type NormalizeIgnoreArea struct {
	X      int    `yaml:"x" json:"x"`
	Y      int    `yaml:"y" json:"y"`
	Width  int    `yaml:"width" json:"width"`
	Height int    `yaml:"height" json:"height"`
	Reason string `yaml:"reason,omitempty" json:"reason,omitempty"`
}
