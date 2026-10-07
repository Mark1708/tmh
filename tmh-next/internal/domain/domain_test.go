package domain

import (
	"encoding/json"
	"testing"
)

func TestConfigRules(t *testing.T) {
	valid := Config{DefaultPage: "dashboard", LeaderDisplay: LeaderIcon, DefaultBackend: "tmux",
		HistoryMode: HistoryPersistent, RetentionHours: 720, Overflow: OverflowReject,
		RemoteTrust: TrustLocalOnly}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	cases := map[string]func(*Config){
		"off-with-retention": func(c *Config) { c.HistoryMode = HistoryOff; c.RetentionHours = 5 },
		"memory-over-24h":    func(c *Config) { c.HistoryMode = HistoryMemory; c.RetentionHours = 25 },
		"memory-zero":        func(c *Config) { c.HistoryMode = HistoryMemory; c.RetentionHours = 0 },
		"persistent-zero":    func(c *Config) { c.RetentionHours = 0 },
		"archive-nonpersist": func(c *Config) {
			c.HistoryMode = HistoryMemory
			c.RetentionHours = 12
			c.Overflow = OverflowArchiveOldest
		},
		"raw-nonlocal":    func(c *Config) { c.AllowRawInput = true; c.RemoteTrust = TrustPrompt },
		"allowlist-empty": func(c *Config) { c.RemoteTrust = TrustAllowlisted },
		"bad-mode":        func(c *Config) { c.HistoryMode = "sometimes" },
		"bad-overflow":    func(c *Config) { c.Overflow = "explode" },
		"bad-leader":      func(c *Config) { c.LeaderDisplay = "sparkle" },
	}
	for name, mutate := range cases {
		c := valid
		mutate(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: accepted invalid config", name)
		}
	}
	// allowlisted with a machine passes
	withMachine := valid
	withMachine.RemoteTrust = TrustAllowlisted
	withMachine.AllowlistedMachines = []string{"homelab"}
	if err := withMachine.Validate(); err != nil {
		t.Errorf("allowlisted with machine rejected: %v", err)
	}
}

func TestKindsAndScenarios(t *testing.T) {
	for _, sc := range []string{"default", "empty", "degraded"} {
		if _, ok := ParseScenario(sc); !ok {
			t.Errorf("scenario %q rejected", sc)
		}
	}
	if _, ok := ParseScenario("chaos"); ok {
		t.Error("unknown scenario accepted")
	}
	for _, kind := range []ResourceKind{KindWorkspace, KindActive, KindTerminal, KindAgent, KindHistory,
		KindEvent, KindSnapshot, KindPlan, KindBackend, KindMachine, KindConfig} {
		if !ValidResourceKind(kind) {
			t.Errorf("kind %q invalid", kind)
		}
	}
	if ValidResourceKind("team") {
		t.Error("unknown kind accepted")
	}
	if n := len(AllActionKinds()); n != 32 {
		t.Fatalf("action kinds = %d, want 32", n)
	}
}

func TestCatalogCloneAndErrors(t *testing.T) {
	c := &Catalog{Revision: 7, Config: Config{DefaultPage: "dashboard", LeaderDisplay: LeaderIcon,
		DefaultBackend: "tmux", HistoryMode: HistoryOff, Overflow: OverflowReject, RemoteTrust: TrustLocalOnly}}
	clone := c.Clone()
	clone.Revision = 99
	clone.Config.DefaultPage = "agents"
	if c.Revision != 7 || c.Config.DefaultPage != "dashboard" {
		t.Fatal("clone shares state with original")
	}
	err := Fail(CodeNotFound, "widget %s missing", "w-1")
	if got := err.Error(); got != "not_found: widget w-1 missing" {
		t.Fatalf("error text = %q", got)
	}
	if ErrCodeOf(err) != CodeNotFound || ErrCodeOf(nil) != "unknown" {
		t.Fatal("error code extraction broken")
	}
	if !IsRevisionConflict(ErrRevisionConflict) || IsRevisionConflict(err) {
		t.Fatal("revision conflict detection broken")
	}
	b, marshalErr := json.Marshal(Action{Kind: ActionSend, Target: Ref(KindTerminal, "term-0001"), Value: "hi"})
	if marshalErr != nil || len(b) == 0 {
		t.Fatalf("action marshal failed: %v", marshalErr)
	}
	if (Action{Kind: ActionSend, Target: Ref(KindTerminal, "t1")}).Identity() != "send/terminal:t1" {
		t.Fatal("action identity malformed")
	}
}
