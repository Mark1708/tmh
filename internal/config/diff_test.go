package config

import "testing"

func TestDiff_OK(t *testing.T) {
	cfg := mustParse(t, `
version: 1
sessions:
  epcp:
    windows:
      lk: /tmp/lk
      mdr: /tmp/mdr
`)
	r, _ := Resolve(cfg, "")
	live := LiveSnapshot{
		Sessions: []LiveSession{{
			Name: "epcp",
			Windows: []LiveWindow{
				{Name: "lk", Dir: "/tmp/lk"},
				{Name: "mdr", Dir: "/tmp/mdr"},
			},
		}},
	}
	d := Diff(r, live)
	for _, e := range d {
		if e.Status != StatusOK {
			t.Fatalf("unexpected %+v", e)
		}
	}
	if HasTracked(d) {
		t.Fatal("should be fully OK")
	}
}

func TestDiff_Drift(t *testing.T) {
	cfg := mustParse(t, `
version: 1
sessions:
  epcp:
    windows:
      jr: /tmp/jr
`)
	r, _ := Resolve(cfg, "")
	live := LiveSnapshot{
		Sessions: []LiveSession{{
			Name:    "epcp",
			Windows: []LiveWindow{{Name: "jr", Dir: "/tmp/jr-frontend"}},
		}},
	}
	d := Diff(r, live)
	if len(d) != 1 || d[0].Status != StatusDrift {
		t.Fatalf("got %+v", d)
	}
	if d[0].ConfigDir != "/tmp/jr" || d[0].LiveDir != "/tmp/jr-frontend" {
		t.Fatalf("bad dirs: %+v", d[0])
	}
}

func TestDiff_New_TrackedWindowInLive(t *testing.T) {
	cfg := mustParse(t, `
version: 1
sessions:
  epcp:
    windows:
      lk: /tmp/lk
`)
	r, _ := Resolve(cfg, "")
	live := LiveSnapshot{
		Sessions: []LiveSession{{
			Name: "epcp",
			Windows: []LiveWindow{
				{Name: "lk", Dir: "/tmp/lk"},
				{Name: "preview", Dir: "/tmp/preview"},
			},
		}},
	}
	d := Diff(r, live)
	var newOne *Drift
	for i, e := range d {
		if e.Window == "preview" {
			newOne = &d[i]
		}
	}
	if newOne == nil || newOne.Status != StatusNew {
		t.Fatalf("expected preview=new, got %+v", d)
	}
}

func TestDiff_Gone(t *testing.T) {
	cfg := mustParse(t, `
version: 1
sessions:
  epcp:
    windows:
      lk: /tmp/lk
      missing: /tmp/missing
`)
	r, _ := Resolve(cfg, "")
	live := LiveSnapshot{
		Sessions: []LiveSession{{
			Name:    "epcp",
			Windows: []LiveWindow{{Name: "lk", Dir: "/tmp/lk"}},
		}},
	}
	d := Diff(r, live)
	var gone *Drift
	for i, e := range d {
		if e.Window == "missing" {
			gone = &d[i]
		}
	}
	if gone == nil || gone.Status != StatusGone {
		t.Fatalf("expected missing=gone, got %+v", d)
	}
}

func TestDiff_AdHocSessionIgnored(t *testing.T) {
	cfg := mustParse(t, `version: 1`)
	r, _ := Resolve(cfg, "")
	live := LiveSnapshot{
		Sessions: []LiveSession{
			{Name: "scratch-1427", Windows: []LiveWindow{{Name: "w", Dir: "/tmp"}}},
		},
	}
	d := Diff(r, live)
	if len(d) != 0 {
		t.Fatalf("ad-hoc sessions should not drift, got %+v", d)
	}
}

func TestDiff_SessionGone(t *testing.T) {
	cfg := mustParse(t, `
version: 1
sessions:
  epcp:
    windows:
      lk: /tmp/lk
`)
	r, _ := Resolve(cfg, "")
	live := LiveSnapshot{}
	d := Diff(r, live)
	if len(d) != 1 || d[0].Status != StatusGone {
		t.Fatalf("got %+v", d)
	}
}
