package schemas

import _ "embed"

//go:embed glyphrun.spec.v1.schema.json
var SpecV1 []byte

//go:embed glyphrun.config.v1.schema.json
var ConfigV1 []byte

//go:embed glyphrun.run.v1.schema.json
var RunV1 []byte

func Get(name string) ([]byte, bool) {
	switch name {
	case "glyphrun.spec.v1.schema.json":
		return SpecV1, true
	case "glyphrun.config.v1.schema.json":
		return ConfigV1, true
	case "glyphrun.run.v1.schema.json":
		return RunV1, true
	default:
		return nil, false
	}
}
