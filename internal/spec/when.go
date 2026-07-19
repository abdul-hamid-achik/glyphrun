package spec

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Conditional is a Verify that also accepts a shorthand string form:
//
//	when: 'screen.contains:"optional prompt"'
//	when: screen.contains:Login
//	when: 'screen.matches:"^Ready"'
//	when: 'screen.equals:"ready"'
//	when: 'screen.notContains:"error"'
//	when: 'process.exited:true'
//
// Full Verify objects continue to work unchanged.
type Conditional Verify

// AsVerify returns a pointer to the underlying Verify, or nil.
func (c *Conditional) AsVerify() *Verify {
	if c == nil {
		return nil
	}
	v := Verify(*c)
	return &v
}

func (c *Conditional) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		return nil
	}
	if value.Kind == yaml.ScalarNode {
		v, err := ParseWhenShorthand(value.Value)
		if err != nil {
			return err
		}
		*c = Conditional(v)
		return nil
	}
	var v Verify
	if err := value.Decode(&v); err != nil {
		return err
	}
	*c = Conditional(v)
	return nil
}

func (c Conditional) MarshalYAML() (any, error) {
	return Verify(c), nil
}

func (c *Conditional) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		v, err := ParseWhenShorthand(s)
		if err != nil {
			return err
		}
		*c = Conditional(v)
		return nil
	}
	var v Verify
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*c = Conditional(v)
	return nil
}

func (c Conditional) MarshalJSON() ([]byte, error) {
	return json.Marshal(Verify(c))
}

// ParseWhenShorthand compiles a compact when expression into a Verify.
// Supported forms:
//
//	screen.contains:<text>
//	screen.notContains:<text>
//	screen.equals:<text>
//	screen.matches:<regex>
//	screen.regex:<regex>
//	process.exited:true|false
//	process.exitCode:<int>
//
// Text after the first colon may be optionally double-quoted.
func ParseWhenShorthand(expr string) (Verify, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return Verify{}, fmt.Errorf("when shorthand is empty")
	}
	key, rest, ok := strings.Cut(expr, ":")
	if !ok {
		return Verify{}, fmt.Errorf("when shorthand %q must look like screen.contains:text", expr)
	}
	key = strings.TrimSpace(key)
	value := unquoteWhenValue(strings.TrimSpace(rest))
	switch key {
	case "screen.contains":
		if value == "" {
			return Verify{}, fmt.Errorf("when screen.contains requires a non-empty value")
		}
		return Verify{Screen: &ScreenCondition{Contains: value}}, nil
	case "screen.notContains":
		if value == "" {
			return Verify{}, fmt.Errorf("when screen.notContains requires a non-empty value")
		}
		return Verify{Screen: &ScreenCondition{NotContains: value}}, nil
	case "screen.equals":
		return Verify{Screen: &ScreenCondition{Equals: value}}, nil
	case "screen.matches", "screen.regex":
		if value == "" {
			return Verify{}, fmt.Errorf("when %s requires a non-empty pattern", key)
		}
		return Verify{Screen: &ScreenCondition{Matches: value}}, nil
	case "process.exited":
		switch strings.ToLower(value) {
		case "true", "1", "yes":
			t := true
			return Verify{Process: &ProcessCondition{Exited: &t}}, nil
		case "false", "0", "no":
			f := false
			return Verify{Process: &ProcessCondition{Exited: &f}}, nil
		default:
			return Verify{}, fmt.Errorf("when process.exited must be true or false, got %q", value)
		}
	case "process.exitCode":
		var code int
		if _, err := fmt.Sscanf(value, "%d", &code); err != nil {
			return Verify{}, fmt.Errorf("when process.exitCode must be an integer: %w", err)
		}
		return Verify{Process: &ProcessCondition{ExitCode: &code}}, nil
	default:
		return Verify{}, fmt.Errorf("unsupported when shorthand %q (supported: screen.contains|notContains|equals|matches|regex, process.exited|exitCode)", key)
	}
}

func unquoteWhenValue(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}
