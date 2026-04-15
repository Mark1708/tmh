package actions

import "git.mark1708.ru/me/tmh/internal/config"

// liveSnapshotFromPaths builds a synthetic LiveSnapshot where each path
// becomes a session "s<i>" with a single window "w" at that path. Useful
// for root-inference tests that need to exercise the LCP algorithm without
// spinning up a full tmuxtest.MockRunner.
func liveSnapshotFromPaths(paths ...string) config.LiveSnapshot {
	var snap config.LiveSnapshot
	for i, p := range paths {
		snap.Sessions = append(snap.Sessions, config.LiveSession{
			Name:    "s" + string(rune('a'+i)),
			Windows: []config.LiveWindow{{Name: "w", Dir: p}},
		})
	}
	return snap
}
