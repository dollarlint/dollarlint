package main

import (
	"sort"
	"strings"
)

type armRef struct {
	API        string `json:"apiVersion"`
	Type       string `json:"type"`
	Provider   string `json:"provider"`
	Definition string `json:"definition"`
}

func collectARMRefs(template any) []armRef {
	seen := map[string]bool{}
	refs := collectARMRefsFromTemplate(template, seen, nil)
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].Type < refs[j].Type
	})
	return refs
}

func collectARMRefsFromTemplate(template any, seen map[string]bool, refs []armRef) []armRef {
	object, ok := template.(map[string]any)
	if !ok {
		return refs
	}
	resources, ok := object["resources"].([]any)
	if !ok {
		return refs
	}
	for _, raw := range resources {
		resource, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		ref, ok := armRefFromResource(resource)
		if ok && !seen[ref.API+"/"+ref.Type] {
			seen[ref.API+"/"+ref.Type] = true
			refs = append(refs, ref)
		}
		if properties, ok := resource["properties"].(map[string]any); ok {
			refs = collectARMRefsFromTemplate(properties["template"], seen, refs)
		}
	}
	return refs
}

func armRefFromResource(resource map[string]any) (armRef, bool) {
	rawType, ok := resource["type"].(string)
	if !ok || strings.TrimSpace(rawType) == "" || strings.HasPrefix(strings.TrimSpace(rawType), "[") {
		return armRef{}, false
	}
	apiVersion, ok := resource["apiVersion"].(string)
	if !ok || strings.TrimSpace(apiVersion) == "" || strings.HasPrefix(strings.TrimSpace(apiVersion), "[") {
		return armRef{}, false
	}
	parts := strings.Split(strings.TrimSpace(rawType), "/")
	if len(parts) < 2 || parts[0] == "" {
		return armRef{}, false
	}
	return armRef{
		API:        strings.TrimSpace(apiVersion),
		Type:       strings.TrimSpace(rawType),
		Provider:   parts[0],
		Definition: strings.Join(parts[1:], "_"),
	}, true
}
