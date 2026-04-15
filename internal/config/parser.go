package config

import (
	"fmt"
	"os"

	errs "git.mark1708.ru/me/tmh/internal/errors"
	"gopkg.in/yaml.v3"
)

// Load reads and parses a YAML file into a Config. The raw yaml.Node tree is
// retained on Config.Node so comment-preserving writes are possible.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", errs.ErrConfigNotFound, path)
		}
		return nil, fmt.Errorf("%w: reading %s: %v", errs.ErrConfigInvalid, path, err)
	}
	return Parse(data)
}

// Parse decodes a YAML byte slice into a Config. Preserves the underlying
// yaml.Node tree for later mutation.
func Parse(data []byte) (*Config, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("%w: %v", errs.ErrConfigInvalid, err)
	}
	// A document with just whitespace decodes into a zero Node; handle it as
	// an empty config so first-run paths can still proceed.
	if root.Kind == 0 {
		return &Config{Node: &root}, nil
	}
	var cfg Config
	if err := root.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("%w: %v", errs.ErrConfigInvalid, err)
	}
	cfg.Node = &root
	return &cfg, nil
}
