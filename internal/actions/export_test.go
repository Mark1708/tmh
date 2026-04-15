package actions

import (
	"strings"
	"testing"
)

func TestExport_MinimalRedactsSecrets(t *testing.T) {
	cfg := parseConfig(t, `
version: 1
roots:
  otr: /tmp/otr
sessions:
  s:
    env:
      AWS_SECRET_KEY: shh
      KUBE_CONTEXT: dev
    windows:
      w: /tmp/otr/x
`)
	out, err := Export(cfg, ExportOptions{Minimal: true})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "shh") {
		t.Fatalf("secret leaked:\n%s", s)
	}
	if !strings.Contains(s, "<redacted>") {
		t.Fatalf("redaction marker missing:\n%s", s)
	}
	if !strings.Contains(s, "KUBE_CONTEXT: dev") {
		t.Fatalf("non-secret env wrongly stripped:\n%s", s)
	}
}

func TestExport_RewritesAbsoluteDirsViaRoots(t *testing.T) {
	cfg := parseConfig(t, `
version: 1
roots:
  otr: /tmp/otr
sessions:
  s:
    windows:
      w: /tmp/otr/products/x
`)
	out, err := Export(cfg, ExportOptions{Minimal: true})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "root: otr") {
		t.Fatalf("absolute dir not rewritten via root:\n%s", s)
	}
	if strings.Contains(s, "w: /tmp/otr/products/x") {
		t.Fatalf("absolute dir survived export --minimal:\n%s", s)
	}
}
