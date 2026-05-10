package engine

import "path"

type schemaAssociationMatch struct {
	Association SchemaAssociation
	Source      string
}

func applySchemaAssociation(document *Document, associations []SchemaAssociation, source string) {
	if document.Schema != "" {
		return
	}
	match, ok := matchSchemaAssociation(document.RelativePath, associations, source)
	if !ok {
		return
	}
	document.Schema = match.Association.Schema
	document.SchemaSource = match.Source
}

func matchSchemaAssociation(rel string, associations []SchemaAssociation, source string) (schemaAssociationMatch, bool) {
	for i := len(associations) - 1; i >= 0; i-- {
		association := associations[i]
		if association.File == "" || association.Schema == "" {
			continue
		}
		if matchPattern(association.File, rel) {
			return schemaAssociationMatch{Association: association, Source: source}, true
		}
	}
	return schemaAssociationMatch{}, false
}

func applyBuiltinSchemaAssociation(document *Document) {
	if document.Schema != "" || path.Base(document.RelativePath) != ".dollarlint.toml" {
		return
	}
	document.Schema = builtinDollarlintConfigSchemaURI
	document.SchemaSource = builtinDollarlintConfigSchemaSource
}

func matchBuiltinSchemaAssociation(rel string) (schemaAssociationMatch, bool) {
	if path.Base(rel) != ".dollarlint.toml" {
		return schemaAssociationMatch{}, false
	}
	return schemaAssociationMatch{
		Association: SchemaAssociation{
			File:   ".dollarlint.toml",
			Schema: builtinDollarlintConfigSchemaURI,
		},
		Source: builtinDollarlintConfigSchemaSource,
	}, true
}
