package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Dotted paths: `sessions.epcp.env.KUBE_CONTEXT`. The first segment is
// looked up from the document root mapping; subsequent segments descend
// into nested mappings.

// PathError is returned when a path cannot be resolved.
type PathError struct {
	Path    string
	Segment string
	Reason  string
}

func (e *PathError) Error() string {
	return fmt.Sprintf("config: path %q: segment %q: %s", e.Path, e.Segment, e.Reason)
}

// documentRoot returns the inner mapping node of a document. Accepts either
// a bare *yaml.Node directly or a Config with Node populated.
func documentRoot(n *yaml.Node) (*yaml.Node, error) {
	if n == nil {
		return nil, fmt.Errorf("config: nil node")
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			// empty document — create an empty map inside
			m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			n.Content = append(n.Content, m)
			return m, nil
		}
		return n.Content[0], nil
	case yaml.MappingNode:
		return n, nil
	default:
		return nil, fmt.Errorf("config: expected document or mapping, got kind %d", n.Kind)
	}
}

// findPair locates the (keyNode, valNode) pair inside a MappingNode for a
// given key. Returns (-1, nil, nil) if not found.
func findPair(m *yaml.Node, key string) (index int, keyNode, valNode *yaml.Node) {
	if m == nil || m.Kind != yaml.MappingNode {
		return -1, nil, nil
	}
	for i := 0; i < len(m.Content); i += 2 {
		k := m.Content[i]
		if k.Value == key {
			return i, k, m.Content[i+1]
		}
	}
	return -1, nil, nil
}

// PathGet returns the yaml.Node at the given dotted path, or nil + error.
func PathGet(root *yaml.Node, path string) (*yaml.Node, error) {
	segments := splitPath(path)
	cursor, err := documentRoot(root)
	if err != nil {
		return nil, err
	}
	for _, seg := range segments {
		if cursor.Kind != yaml.MappingNode {
			return nil, &PathError{Path: path, Segment: seg, Reason: "parent is not a mapping"}
		}
		_, _, val := findPair(cursor, seg)
		if val == nil {
			return nil, &PathError{Path: path, Segment: seg, Reason: "not found"}
		}
		cursor = val
	}
	return cursor, nil
}

// pathSetScalar is the shared implementation for PathSet and its typed variants.
// tag must be a valid YAML scalar tag such as "!!str", "!!bool", or "!!int".
func pathSetScalar(root *yaml.Node, path, value, tag string) error {
	segments := splitPath(path)
	if len(segments) == 0 {
		return fmt.Errorf("config: empty path")
	}
	cursor, err := documentRoot(root)
	if err != nil {
		return err
	}
	for _, seg := range segments[:len(segments)-1] {
		if cursor.Kind != yaml.MappingNode {
			return &PathError{Path: path, Segment: seg, Reason: "parent is not a mapping"}
		}
		_, _, val := findPair(cursor, seg)
		if val == nil {
			kn := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: seg}
			vn := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			cursor.Content = append(cursor.Content, kn, vn)
			cursor = vn
			continue
		}
		if val.Kind != yaml.MappingNode {
			return &PathError{Path: path, Segment: seg, Reason: "parent is not a mapping"}
		}
		cursor = val
	}
	last := segments[len(segments)-1]
	if cursor.Kind != yaml.MappingNode {
		return &PathError{Path: path, Segment: last, Reason: "parent is not a mapping"}
	}
	_, _, val := findPair(cursor, last)
	if val == nil {
		kn := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: last}
		vn := &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value}
		cursor.Content = append(cursor.Content, kn, vn)
		return nil
	}
	if val.Kind != yaml.ScalarNode {
		return &PathError{Path: path, Segment: last, Reason: "existing value is not scalar"}
	}
	val.Value = value
	val.Tag = tag
	val.Style = 0
	return nil
}

// PathSetBool sets a boolean value at the given YAML path. The node is tagged
// !!bool so yaml.v3 unmarshal correctly decodes it into bool or *bool fields.
func PathSetBool(root *yaml.Node, path string, value bool) error {
	s := "false"
	if value {
		s = "true"
	}
	return pathSetScalar(root, path, s, "!!bool")
}

// PathSetInt sets an integer value at the given YAML path.
func PathSetInt(root *yaml.Node, path string, value int) error {
	return pathSetScalar(root, path, fmt.Sprintf("%d", value), "!!int")
}

// PathSet updates or creates a scalar at the given path. Missing intermediate
// mappings are created (as empty MappingNodes with Tag !!map). If the target
// already exists and is scalar, its Value is replaced in place preserving
// comments. If it exists and is non-scalar, an error is returned.
// PathSet updates or creates a string scalar at the given path.
func PathSet(root *yaml.Node, path, value string) error {
	return pathSetScalar(root, path, value, "!!str")
}

// PathDelete removes the (key, value) pair at the given path. Returns
// PathError if any segment does not exist.
func PathDelete(root *yaml.Node, path string) error {
	segments := splitPath(path)
	if len(segments) == 0 {
		return fmt.Errorf("config: empty path")
	}
	cursor, err := documentRoot(root)
	if err != nil {
		return err
	}
	for _, seg := range segments[:len(segments)-1] {
		_, _, val := findPair(cursor, seg)
		if val == nil {
			return &PathError{Path: path, Segment: seg, Reason: "not found"}
		}
		if val.Kind != yaml.MappingNode {
			return &PathError{Path: path, Segment: seg, Reason: "parent is not a mapping"}
		}
		cursor = val
	}
	last := segments[len(segments)-1]
	idx, _, _ := findPair(cursor, last)
	if idx < 0 {
		return &PathError{Path: path, Segment: last, Reason: "not found"}
	}
	cursor.Content = append(cursor.Content[:idx], cursor.Content[idx+2:]...)
	return nil
}

// PathRename renames a mapping key at the given path. `path` points at the
// parent; `oldKey` and `newKey` are the key names inside that parent.
func PathRename(root *yaml.Node, parentPath, oldKey, newKey string) error {
	var cursor *yaml.Node
	if parentPath == "" {
		c, err := documentRoot(root)
		if err != nil {
			return err
		}
		cursor = c
	} else {
		c, err := PathGet(root, parentPath)
		if err != nil {
			return err
		}
		cursor = c
	}
	if cursor.Kind != yaml.MappingNode {
		return &PathError{Path: parentPath, Segment: oldKey, Reason: "parent is not a mapping"}
	}
	_, keyNode, _ := findPair(cursor, oldKey)
	if keyNode == nil {
		return &PathError{Path: parentPath, Segment: oldKey, Reason: "not found"}
	}
	if idx, _, _ := findPair(cursor, newKey); idx >= 0 {
		return &PathError{Path: parentPath, Segment: newKey, Reason: "target already exists"}
	}
	keyNode.Value = newKey
	return nil
}

func splitPath(p string) []string {
	if p == "" {
		return nil
	}
	return strings.Split(p, ".")
}
