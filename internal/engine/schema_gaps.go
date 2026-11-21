package engine

import "strings"

type knownSchemaGapEntry struct {
	Name      string
	FileMatch []string
	DocsURL   string
	Reason    string
}

var knownSchemaGaps = []knownSchemaGapEntry{
	{
		Name:      "Cargo configuration",
		FileMatch: []string{".cargo/config.toml", "**/.cargo/config.toml"},
		DocsURL:   "https://doc.rust-lang.org/cargo/reference/config.html",
		Reason:    "DollarLint recognizes this Cargo configuration file, but has no built-in or catalog schema for it yet.",
	},
	{
		Name:      "Netlify deploy config",
		FileMatch: []string{"netlify.toml"},
		DocsURL:   "https://docs.netlify.com/build/configure-builds/file-based-configuration/",
		Reason:    "DollarLint recognizes this Netlify deploy config, but has no built-in or catalog schema for it yet; SchemaStore's Netlify schema covers Netlify/Decap CMS admin/config*.yml, not netlify.toml.",
	},
	{
		Name:      "Apache .asf.yaml configuration",
		FileMatch: []string{".asf.yaml"},
		DocsURL:   "https://infra.apache.org/asf-yaml.html",
		Reason:    "DollarLint recognizes this Apache Infra .asf.yaml configuration file, but has no built-in or catalog schema for it yet.",
	},
	{
		Name:      "terraform-docs configuration",
		FileMatch: []string{".terraform-docs.yml", ".terraform-docs.yaml"},
		DocsURL:   "https://terraform-docs.io/user-guide/configuration/",
		Reason:    "DollarLint recognizes this terraform-docs configuration file, but has no built-in or catalog schema for it yet.",
	},
}

func applyKnownSchemaGap(document *Document) {
	if document == nil || document.Schema != "" || document.SchemaGap != nil {
		return
	}
	if gap, ok := matchKnownSchemaGap(document.RelativePath); ok {
		document.SchemaGap = gap
	}
}

func matchKnownSchemaGap(rel string) (*SchemaGap, bool) {
	rel = strings.ToLower(cleanGlob(rel))
	if rel == "" {
		return nil, false
	}
	for _, entry := range knownSchemaGaps {
		for _, pattern := range entry.FileMatch {
			if matchPattern(strings.ToLower(pattern), rel) {
				return entry.schemaGap(pattern), true
			}
		}
	}
	return nil, false
}

func (entry knownSchemaGapEntry) schemaGap(fileMatch string) *SchemaGap {
	return &SchemaGap{
		Name:      entry.Name,
		Reason:    entry.Reason,
		DocsURL:   entry.DocsURL,
		FileMatch: fileMatch,
	}
}
