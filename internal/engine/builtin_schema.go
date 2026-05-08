package engine

import (
	_ "embed"
	"fmt"
)

const (
	builtinDollarlintConfigSchemaURI    = "dollarlint://schemas/dollarlint.schema.json"
	builtinDollarlintConfigSchemaSource = "builtin:dollarlint"
)

//go:embed builtin_schemas/dollarlint.schema.json
var builtinDollarlintConfigSchema []byte

func loadBuiltinSchema(raw string) (any, error) {
	switch raw {
	case builtinDollarlintConfigSchemaURI:
		return decodeSchemaDocument(builtinDollarlintConfigSchema, "dollarlint.schema.json")
	default:
		return nil, fmt.Errorf("unknown built-in schema %s", raw)
	}
}
