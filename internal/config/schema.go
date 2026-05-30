package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	embeddedschemas "github.com/abdul-hamid-achik/glyphrun/schemas"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

func ValidateConfigSchema(data []byte, configPath string) error {
	projectRoot := filepath.Dir(configPath)
	schemaName := "glyphrun.config.v1.schema.json"
	schemaPath := filepath.Join(projectRoot, DefaultSchemaRoot, schemaName)
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		embedded, ok := embeddedschemas.Get(schemaName)
		if !ok {
			return nil
		}
		schemaBytes = embedded
		schemaPath = "embedded://" + schemaName
	}
	var document any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return err
	}
	document = toJSONValue(document)
	var schemaDoc any
	if err := json.Unmarshal(schemaBytes, &schemaDoc); err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(schemaPath, schemaDoc); err != nil {
		return err
	}
	compiled, err := compiler.Compile(schemaPath)
	if err != nil {
		return err
	}
	return compiled.Validate(document)
}

func toJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, v := range typed {
			out[k] = toJSONValue(v)
		}
		return out
	case map[any]any:
		out := map[string]any{}
		for k, v := range typed {
			if key, ok := k.(string); ok {
				out[key] = toJSONValue(v)
			}
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, v := range typed {
			out[i] = toJSONValue(v)
		}
		return out
	default:
		return value
	}
}
