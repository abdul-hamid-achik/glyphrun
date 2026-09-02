package spec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	embeddedschemas "github.com/abdul-hamid-achik/glyphrun/schemas"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// ValidateSourceSchema checks a spec document against the spec JSON schema
// after placeholder substitution.
func ValidateSourceSchema(source []byte, path string, opts ParseOptions) error {
	substituted, err := SubstitutePlaceholders(string(source), path, opts)
	if err != nil {
		return err
	}
	var document any
	if strings.HasSuffix(path, ".json") {
		if err := json.Unmarshal([]byte(substituted), &document); err != nil {
			return err
		}
	} else {
		if err := yaml.Unmarshal([]byte(substituted), &document); err != nil {
			return err
		}
		document = ToJSONValue(document)
	}
	return ValidateDocumentSchema(document, path, "glyphrun.spec.v1.schema.json", opts)
}

// ValidateDocumentSchema validates an already-decoded document (JSON-shaped:
// string-keyed maps) against the named schema. The schema is read from the
// project schemaRoot when present so a project can override any bundled
// schema (spec, config, stories alike); otherwise the embedded copy is used.
func ValidateDocumentSchema(document any, path, schemaName string, opts ParseOptions) error {
	schemaRoot := opts.ConfigValues["schemaRoot"]
	if schemaRoot == "" {
		schemaRoot = "schemas"
	}
	if !filepath.IsAbs(schemaRoot) {
		projectRoot := opts.ProjectRoot
		if projectRoot == "" {
			projectRoot = filepath.Dir(path)
		}
		schemaRoot = filepath.Join(projectRoot, schemaRoot)
	}
	schemaPath := filepath.Join(schemaRoot, schemaName)
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

// ToJSONValue converts yaml.v3 output (which may contain map[any]any) into
// the string-keyed tree a JSON-schema validator expects.
func ToJSONValue(value any) any { return toJSONValue(value) }

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
