package actions

import (
	"fmt"
	"os"

	"git.mark1708.ru/me/tmh/internal/config"
)

// ImportMode is replace (full overwrite) or merge (additive, conflicts win
// for the imported side).
type ImportMode int

const (
	ImportReplace ImportMode = iota
	ImportMerge
)

// ImportFile loads a YAML file and applies it on top of `dst` according to
// mode. Returns the merged Config; the caller is responsible for persisting
// it via config.Write.
func ImportFile(dst *config.Config, path string, mode ImportMode) (*config.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("import: %w", err)
	}
	src, err := config.Parse(data)
	if err != nil {
		return nil, err
	}
	if mode == ImportReplace || dst == nil {
		return src, nil
	}
	return mergeConfigs(dst, src), nil
}

// mergeConfigs returns a new config combining base and overlay. Overlay
// wins on key conflicts. The yaml.Node tree comes from base so subsequent
// writes preserve its formatting/comments — newly added entries appended
// via path.go go to the end of their respective sections.
func mergeConfigs(base, overlay *config.Config) *config.Config {
	out := *base
	out.Roots = mergeStringMap(base.Roots, overlay.Roots)
	out.Templates = mergeTemplateMap(base.Templates, overlay.Templates)
	out.Layouts = mergeLayoutMap(base.Layouts, overlay.Layouts)
	out.Profiles = mergeProfileMap(base.Profiles, overlay.Profiles)
	out.Sessions = mergeSessionMap(base.Sessions, overlay.Sessions)
	return &out
}

func mergeStringMap(a, b map[string]string) map[string]string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func mergeTemplateMap(a, b map[string]config.Window) map[string]config.Window {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make(map[string]config.Window, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func mergeLayoutMap(a, b map[string]config.Layout) map[string]config.Layout {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make(map[string]config.Layout, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func mergeProfileMap(a, b map[string]config.Profile) map[string]config.Profile {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make(map[string]config.Profile, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func mergeSessionMap(a, b map[string]config.Session) map[string]config.Session {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make(map[string]config.Session, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
