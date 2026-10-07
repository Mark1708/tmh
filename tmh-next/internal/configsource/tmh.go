// Package configsource projects the existing tmh YAML configuration into the
// desired workspace state used by the production control plane.
package configsource

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark1708/tmh-next/internal/domain"
	"gopkg.in/yaml.v3"
)

func LoadDesired(path string) (map[string]domain.WorkspaceState, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tmh config %s: %w", path, err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("parse tmh config %s: %w", path, err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("parse tmh config %s: root must be a mapping", path)
	}
	root := document.Content[0]
	roots, err := stringMap(mappingValue(root, "roots"), "roots")
	if err != nil {
		return nil, err
	}
	templates, err := templateCommands(mappingValue(root, "templates"))
	if err != nil {
		return nil, err
	}
	sessions := mappingValue(root, "sessions")
	if sessions == nil {
		return map[string]domain.WorkspaceState{}, nil
	}
	if sessions.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("sessions must be a mapping")
	}

	result := make(map[string]domain.WorkspaceState, len(sessions.Content)/2)
	for i := 0; i < len(sessions.Content); i += 2 {
		nameNode, sessionNode := sessions.Content[i], sessions.Content[i+1]
		name := strings.TrimSpace(nameNode.Value)
		if name == "" {
			return nil, fmt.Errorf("session name is empty")
		}
		if _, duplicate := result[name]; duplicate {
			return nil, fmt.Errorf("duplicate session %q", name)
		}
		windows := mappingValue(sessionNode, "windows")
		state := domain.WorkspaceState{}
		if windows != nil {
			if windows.Kind != yaml.MappingNode {
				return nil, fmt.Errorf("session %q windows must be a mapping", name)
			}
			state.Windows = make([]domain.WindowSpec, 0, len(windows.Content)/2)
			for j := 0; j < len(windows.Content); j += 2 {
				windowName := strings.TrimSpace(windows.Content[j].Value)
				windowNode := windows.Content[j+1]
				cwd, command, err := resolveWindow(name, windowName, windowNode, roots, templates)
				if err != nil {
					return nil, err
				}
				state.Windows = append(state.Windows, domain.WindowSpec{
					ID: "window:" + windowName, Index: len(state.Windows) + 1,
					Surfaces: []domain.SurfaceSpec{{
						ID: "surface:" + windowName + ":primary", Title: command, CWD: cwd, Split: "none",
					}},
				})
			}
		}
		result[name] = state
	}
	return result, nil
}

func resolveWindow(session, name string, node *yaml.Node, roots, templates map[string]string) (string, string, error) {
	if node == nil || node.Kind != yaml.MappingNode {
		return "", "", fmt.Errorf("session %q window %q must be a mapping", session, name)
	}
	dir := scalarValue(node, "dir")
	rootName := scalarValue(node, "root")
	relative := scalarValue(node, "path")
	template := scalarValue(node, "template")
	command := ""
	if template != "" {
		var ok bool
		command, ok = templates[template]
		if !ok {
			return "", "", fmt.Errorf("session %q window %q references unknown template %q", session, name, template)
		}
	}
	if dir != "" {
		if rootName != "" || relative != "" {
			return "", "", fmt.Errorf("session %q window %q mixes dir with root/path", session, name)
		}
		return expandHome(dir), command, nil
	}
	if rootName == "" {
		if relative != "" {
			return "", "", fmt.Errorf("session %q window %q has path without root", session, name)
		}
		return "", command, nil
	}
	base, ok := roots[rootName]
	if !ok {
		return "", "", fmt.Errorf("session %q window %q references unknown root %q", session, name, rootName)
	}
	base = expandHome(base)
	if filepath.IsAbs(relative) {
		return "", "", fmt.Errorf("session %q window %q path must be relative to root", session, name)
	}
	joined := filepath.Clean(filepath.Join(base, relative))
	rel, err := filepath.Rel(filepath.Clean(base), joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("session %q window %q path escapes root", session, name)
	}
	return joined, command, nil
}

func templateCommands(node *yaml.Node) (map[string]string, error) {
	result := map[string]string{}
	if node == nil {
		return result, nil
	}
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("templates must be a mapping")
	}
	for i := 0; i < len(node.Content); i += 2 {
		name, value := node.Content[i].Value, node.Content[i+1]
		if value.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("template %q must be a mapping", name)
		}
		result[name] = scalarValue(value, "command")
	}
	return result, nil
}

func stringMap(node *yaml.Node, field string) (map[string]string, error) {
	result := map[string]string{}
	if node == nil {
		return result, nil
	}
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s must be a mapping", field)
	}
	for i := 0; i < len(node.Content); i += 2 {
		result[node.Content[i].Value] = node.Content[i+1].Value
	}
	return result, nil
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func scalarValue(node *yaml.Node, key string) string {
	value := mappingValue(node, key)
	if value == nil || value.Kind != yaml.ScalarNode {
		return ""
	}
	return strings.TrimSpace(value.Value)
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return filepath.Clean(path)
}
