// tmh-gen emits build-time artefacts that are published alongside the
// tmh binary: the JSON Schema for config.yml, man pages, and shell
// completions generated from the cobra command tree.
//
// Usage:
//
//	go run ./cmd/tmh-gen
//
// Produces:
//
//	schemas/tmh.schema.json
//	docs/man/tmh*.1
//	docs/completions/{bash,zsh,fish}/tmh
//
// The Makefile target `make docs` is the canonical entry point.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	tmhcmd "github.com/mark1708/tmh/cmd/tmh/cmd"
	"github.com/mark1708/tmh/internal/config"

	"github.com/spf13/cobra/doc"
)

func main() {
	repoRoot := "."
	if len(os.Args) > 1 {
		repoRoot = os.Args[1]
	}

	if err := run(repoRoot); err != nil {
		fmt.Fprintln(os.Stderr, "tmh-gen:", err)
		os.Exit(1)
	}
}

func run(root string) error {
	if err := writeSchema(root); err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	if err := writeMan(root); err != nil {
		return fmt.Errorf("man: %w", err)
	}
	if err := writeCompletions(root); err != nil {
		return fmt.Errorf("completions: %w", err)
	}
	return nil
}

func writeSchema(root string) error {
	b, err := config.GenerateSchema()
	if err != nil {
		return err
	}
	out := filepath.Join(root, "schemas")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	p := filepath.Join(out, "tmh.schema.json")
	return os.WriteFile(p, b, 0o644)
}

func writeMan(root string) error {
	cmd := tmhcmd.NewRoot("dev")
	out := filepath.Join(root, "docs", "man")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	header := &doc.GenManHeader{Title: "TMH", Section: "1"}
	return doc.GenManTree(cmd, header, out)
}

func writeCompletions(root string) error {
	cmd := tmhcmd.NewRoot("dev")
	base := filepath.Join(root, "docs", "completions")

	for _, sh := range []string{"bash", "zsh", "fish"} {
		dir := filepath.Join(base, sh)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		p := filepath.Join(dir, "tmh")
		f, err := os.Create(p)
		if err != nil {
			return err
		}
		switch sh {
		case "bash":
			err = cmd.GenBashCompletionV2(f, true)
		case "zsh":
			err = cmd.GenZshCompletion(f)
		case "fish":
			err = cmd.GenFishCompletion(f, true)
		}
		f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
