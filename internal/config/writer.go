package config

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// WriteOptions tunes how Write persists a Config.
type WriteOptions struct {
	// BackupDir, if non-empty, receives a timestamped copy of the previous
	// file contents before the new content is written.
	BackupDir string
	// MaxBackups caps how many files BackupDir may contain (oldest trimmed).
	// Zero means no trimming.
	MaxBackups int
	// PreserveBlanks, when true, restores blank lines between top-level keys
	// based on the original file's layout.
	PreserveBlanks bool
}

// Write persists the yaml.Node tree on c atomically to path. If PreserveBlanks
// is true, the caller should have captured the original file content via the
// path itself; Write reads it back to diff top-level key positions.
func Write(c *Config, path string, opts WriteOptions) error {
	if c == nil || c.Node == nil {
		return fmt.Errorf("config: nothing to write (nil Node)")
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(c.Node); err != nil {
		return fmt.Errorf("config: encode: %w", err)
	}
	_ = enc.Close()

	out := buf.Bytes()
	if opts.PreserveBlanks {
		if orig, err := os.ReadFile(path); err == nil {
			out = reinjectBlankLines(out, orig)
		}
	}

	// ensure dir exists
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("config: mkdir %s: %w", filepath.Dir(path), err)
	}

	// backup existing file
	if opts.BackupDir != "" {
		if err := backupFile(path, opts.BackupDir, opts.MaxBackups); err != nil {
			return fmt.Errorf("config: backup: %w", err)
		}
	}

	// atomic write: tmp -> fsync -> rename
	tmp, err := writeTemp(filepath.Dir(path), out)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("config: rename %s → %s: %w", tmp, path, err)
	}
	return nil
}

func writeTemp(dir string, data []byte) (string, error) {
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		return "", err
	}
	name := filepath.Join(dir, fmt.Sprintf(".tmh.%d.%s.tmp", os.Getpid(), hex.EncodeToString(suffix)))
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	if err := f.Sync(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

func backupFile(path, backupDir string, maxKeep int) error {
	orig, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return err
	}
	ts := time.Now().Format("20060102-150405")
	base := filepath.Base(path)
	out := filepath.Join(backupDir, fmt.Sprintf("%s.%s.bak", base, ts))
	if err := os.WriteFile(out, orig, 0o644); err != nil {
		return err
	}
	if maxKeep > 0 {
		return trimBackups(backupDir, base, maxKeep)
	}
	return nil
}

func trimBackups(dir, base string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var matches []os.DirEntry
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), base+".") || !strings.HasSuffix(e.Name(), ".bak") {
			continue
		}
		matches = append(matches, e)
	}
	if len(matches) <= keep {
		return nil
	}
	// rely on timestamp suffix being lexicographically sortable.
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && matches[j-1].Name() > matches[j].Name(); j-- {
			matches[j-1], matches[j] = matches[j], matches[j-1]
		}
	}
	excess := len(matches) - keep
	for i := 0; i < excess; i++ {
		_ = os.Remove(filepath.Join(dir, matches[i].Name()))
	}
	return nil
}

// reinjectBlankLines restores blank lines between top-level keys. yaml.v3
// drops those on marshal; we parse the original file's column-1 key
// positions and insert a blank line before each newly-encoded top-level key
// that had one in the original.
func reinjectBlankLines(encoded, original []byte) []byte {
	topLevelKeys := detectBlankBeforeKeys(original)
	if len(topLevelKeys) == 0 {
		return encoded
	}
	var out bytes.Buffer
	lines := bytes.Split(encoded, []byte("\n"))
	for i, line := range lines {
		if i > 0 && isTopLevelKey(line) {
			key := extractKey(line)
			if _, ok := topLevelKeys[key]; ok {
				out.WriteByte('\n')
			}
		}
		out.Write(line)
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.Bytes()
}

func detectBlankBeforeKeys(data []byte) map[string]struct{} {
	keys := make(map[string]struct{})
	lines := bytes.Split(data, []byte("\n"))
	prevBlank := false
	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			prevBlank = true
			continue
		}
		if isTopLevelKey(line) && prevBlank {
			keys[extractKey(line)] = struct{}{}
		}
		if bytes.HasPrefix(trimmed, []byte("#")) {
			// comments don't break the blank-line streak
			continue
		}
		prevBlank = false
	}
	return keys
}

func isTopLevelKey(line []byte) bool {
	if len(line) == 0 {
		return false
	}
	if line[0] == ' ' || line[0] == '\t' || line[0] == '#' || line[0] == '-' {
		return false
	}
	return bytes.Contains(line, []byte(":"))
}

func extractKey(line []byte) string {
	idx := bytes.IndexByte(line, ':')
	if idx < 0 {
		return ""
	}
	return string(bytes.TrimSpace(line[:idx]))
}
