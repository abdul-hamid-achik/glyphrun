package cli

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

type outputFormat string

const (
	formatJSON outputFormat = "json"
	formatYAML outputFormat = "yaml"
	formatMD   outputFormat = "md"
)

func resolveFormat(value string) (outputFormat, error) {
	switch outputFormat(value) {
	case formatJSON, formatYAML, formatMD:
		return outputFormat(value), nil
	default:
		return "", fmt.Errorf("unsupported --format %q", value)
	}
}

func emit(format outputFormat, value any, markdown func() string) (string, error) {
	switch format {
	case formatJSON:
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data) + "\n", nil
	case formatYAML:
		data, err := yaml.Marshal(value)
		if err != nil {
			return "", err
		}
		return string(data), nil
	case formatMD:
		return markdown(), nil
	default:
		return "", fmt.Errorf("unsupported format %s", format)
	}
}
