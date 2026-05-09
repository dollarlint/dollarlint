package engine

import "path"

func applySchemaAssociation(document *Document, associations []SchemaAssociation, source string) {
	if document.Schema != "" {
		return
	}
	for i := len(associations) - 1; i >= 0; i-- {
		association := associations[i]
		if association.File == "" || association.Schema == "" {
			continue
		}
		if matchPattern(association.File, document.RelativePath) {
			document.Schema = association.Schema
			document.SchemaSource = source
			return
		}
	}
}

func applyBuiltinSchemaAssociation(document *Document) {
	if document.Schema != "" || path.Base(document.RelativePath) != ".dollarlint.toml" {
		return
	}
	document.Schema = builtinDollarlintConfigSchemaURI
	document.SchemaSource = builtinDollarlintConfigSchemaSource
}
