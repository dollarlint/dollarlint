package engine

import (
	"strconv"

	"gopkg.in/yaml.v3"
)

func buildYAMLSourceMap(raw []byte) (SourceMap, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	sourceMap := SourceMap{}
	node := &root
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		node = root.Content[0]
	}
	walkYAMLNode(node, sourceMap, "/")
	return sourceMap, nil
}

func walkYAMLNode(node *yaml.Node, sourceMap SourceMap, pointer string) {
	if node == nil {
		return
	}
	setSourcePosition(sourceMap, pointer, SourcePosition{Line: node.Line, Column: node.Column})
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			walkYAMLNode(value, sourceMap, joinPointer(pointer, key.Value))
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			walkYAMLNode(child, sourceMap, joinPointer(pointer, strconv.Itoa(i)))
		}
	}
}
