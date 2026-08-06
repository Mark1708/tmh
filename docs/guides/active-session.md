# Active session (optional real tmux session)

> **Status:** Optional feature, disabled by default. Requires explicit enable in `config.yml`.
>
> **Safety:** Never destroys physical windows. Orphan-safe recovery. Fail-closed on collision.

## Overview

The active session feature creates a real tmux session named `active` that temporarily links recently selected windows from your declared sessions. This lets you use native tmux navigation (`prefix n`, `prefix p`, `prefix 1`, `prefix l`) across all sessions while keeping each window anchored in its original declarative structure.

**Key concepts:**

- **Opt-in:** Feature is disabled by default. Enable explicitly in `config.yml`.
- **Physical window preservation:** Windows are linked via `tmux link-window`, not moved. The original session remains the owner.
- **Identity:** Windows are tracked by physical `@ID` (like `@123`), stable for the tmux server lifetime.
- **Interaction-driven expiry:** TTL slides on each window selection. Not a wall-clock timer during inactivity.
- **Orphan safety:** Last link cannot be removed automatically. Window becomes orphaned, not deleted.
- **Collision fail-closed:** Ownership marker prevents overwriting user-created `active` sessions.

## Configuration

Add to `~/.config/tmh/config.yml`:

```yaml
defaults:
  tmux_integration:
    active:
      enabled: true   # default: false
      ttl: 5h         # optional; default when omitted: 5h
```

**Configuration rules:**

- `enabled` defaults to `false`. Omit the block entirely to keep the feature disabled.
- `ttl` is a duration string (e.g., `5h`, `2h30m`, `30m`).
- TTL range: `(0, 720h]` — must be positive and at most 30 days.
- If `enabled: true` and `ttl` is omitted, the effective TTL is `5h`.
- Explicit `ttl` is validated even when `enabled: false` to catch misconfiguration early.

**Example configurations:**

```yaml
# Recommended: 5-hour sliding window
defaults:
  tmux_integration:
    active:
      enabled: true
      ttl: 5h

# Short TTL for ephemeral workflows
defaults:
  tmux_integration:
    active:
      enabled: true
      ttl: 1h

# Disabled (default behavior)
# Simply omit the entire block or set enabled: false
```

## Setup and enable

After enabling the feature in `config.yml`, run the setup command to install the managed tmux hook:

```sh
tmh tmux setup --append
```

This adds a managed block to `~/.tmux.conf`:

```tmux
# BEGIN tmh-managed block: active session hook
set-hook -g 'session-window-changed[1708]' \
  'run-shell "tmh active touch #{window_id}"'
# END tmh-managed block: active session hook
```

**Important:** Source your updated tmux conf:

```sh
tmux source-file ~/.tmux.conf
```

**What the hook does:**

- Runs `tmh active touch @ID` whenever you navigate to a different window.
- Extends the TTL for that window by `ttl` hours.
- Uses indexed slot `[1708]` to avoid conflicts with your own hooks.
- Hidden from `tmh --help` and shell completions (internal integration only).

**Audit the hook:**

```sh
tmh tmux audit
```

The audit checks for:
- Hook presence in the correct indexed slot.
- Hook conflicts (if you or another tool occupies slot `[1708]`).
- Safe teardown instructions if you disable the feature.

## How it works

### Lifecycle state machine

When you run `tmh attach session:window`:

1. **Validate:** Check that the feature is enabled and TTL is valid.
2. **Check ownership:** Verify `active` session exists with correct `@tmh-active-owner = tmh/v1` marker.
   - If marker missing or different → **collision**: abort, show error.
   - If session missing → create detached session with marker.
3. **Promote:** Link the physical window into `active` via `tmux link-window`.
   - Choose a free index accounting for `base-index`.
   - Store tracking metadata in SQLite (`~/.local/state/tmh/state.db`).
4. **Attach:** Switch your tmux client to the linked window in `active`.
5. **Touch:** Hook extends TTL on each native navigation (`prefix n`, `prefix p`, etc.).

### Window states

| State | Meaning | Lifecycle |
|-------|---------|-----------|
| `tracked` | Window is linked in `active` and has at least one source link. | TTL extended on each touch. Expired safe → unlink from `active`. |
| `orphaned` | `active` link is the only remaining link. | Cannot be removed automatically. Requires manual `recover` or `remove`. |

### Expiry (safe vs orphan)

**Safe expiry:** When a tracked window expires and has at least one non-active link:
1. Unlink the alias from `active`.
2. Verify the source window still exists.
3. Delete the tracking row from SQLite.
4. Physical window remains in its original session.

**Orphan expiry:** When a tracked window expires and the active link is the last link:
1. Do not unlink (cannot destroy the last link).
2. Mark the row as `orphaned` in SQLite.
3. Window stays in `active` but requires manual recovery.

This ensures the physical window is never lost due to TTL.

## Commands

### Status

```sh
tmh active status [--json]
```

Shows the current state of the active session feature:

```
enabled: true
session: active
owned: true
ttl: 5h
server_epoch: <server-identifier>
tracked: 3
orphaned: 0

Entries:
  @123 "editor" (tracked)
    active index: 2
    source: work (index 1)
    promoted at: 2026-08-05T10:00:00Z
    last selected at: 2026-08-05T14:30:00Z
    expires at: 2026-08-05T19:30:00Z

  @124 "api" (tracked)
    active index: 3
    source: work (index 2)
    promoted at: 2026-08-05T09:30:00Z
    last selected at: 2026-08-05T14:25:00Z
    expires at: 2026-08-05T19:25:00Z
```

JSON output (`--json`) is English-stable for script consumers.

### Prune

```sh
tmh active prune
```

Removes expired tracked windows and cleans up stale database entries:

```
pruned 2 expired tracked windows
deleted 1 stale database row
found 0 orphaned windows (requires manual recover)
```

**Safety:** Prune only removes aliases from `active`. Physical windows remain intact.

### Remove

```sh
tmh active remove @ID
```

Removes an active alias for a specific window ID:

```
removed @123 from active session
```

**Safety rules:**

- Fails if the window is orphaned (last link).
- Verifies the source window still has a non-active link before unlinking.
- Returns an actionable error if the window cannot be safely removed.

### Recover

```sh
tmh active recover @ID --to session
```

Moves an orphaned window back to a target session:

```
recovered @124 to session work
```

**Recovery process:**

1. Validates the `@ID` format and that the window is actually orphaned.
2. Verifies the target session exists and is not `active`.
3. Links the window into the target session at a free index.
4. Unlinks the alias from `active` (only after destination link verified).
5. Deletes the tracking row from SQLite.
6. Fails atomically if any step fails (partial recovery is retryable).

## Ownership and collision

### Ownership marker

The `active` session is marked with a tmux user option:

```tmux
@tmh-active-owner = tmh/v1
```

**Collision detection:**

- If `active` exists but the marker is missing or different → **collision**.
- On collision, tmh does **not**:
  - Create any links.
  - Write to SQLite.
  - Modify the marker.
  - Delete the session.

**Collision status:**

```sh
tmh active status
```

Shows:
```
collision: true
reason: active session exists but is not owned by tmh (marker missing or invalid)
```

**Resolution:**

1. Rename the conflicting session: `tmux rename-session active old-active`.
2. Or disable the feature in `config.yml` if you want to keep your own `active` session.

### User-created `active` session

If you manually create a session named `active`:

- When the feature is **disabled**: Your session works normally. tmh does not interfere.
- When the feature is **enabled**: Collision detected. Your session is untouched. tmh fails closed.

## Limitations

### No background daemon

Expiry is **not** enforced by a background timer. Instead:

- TTL is extended by the hook on each native navigation.
- Expiry is checked on the next relevant `tmh` invocation (attach, status, prune).
- During complete inactivity, an expired alias may persist longer than TTL.

This design avoids running a persistent daemon process.

### No exact wall-clock expiry

TTL is interaction-driven:
- `prefix n` / `prefix p` / `prefix 1` → hook runs → TTL slides.
- Explicit `tmh attach session:window` → TTL slides.
- If you stay on one window and never navigate, its TTL does not extend.

### Index renumbering

tmux `renumber-windows` can change window indices at any time. The implementation:
- Uses physical `@ID` for identity, not index.
- Re-aliases by `@ID` immediately before each mutation.
- Re-checks by `@ID` after each mutation.

## Safety guarantees

### Never destroys physical windows

All operations that could remove aliases check that at least one non-active link exists:

- `prune` safe expiry → verifies source link before unlinking.
- `remove @ID` → fails if orphaned.
- `recover @ID` → verifies destination link before active unlink.

### Source kill cleanup

When tmh kills a source session or window:

1. Removes the active alias while the source still exists (verified).
2. If cleanup fails, the kill is aborted.
3. This prevents aliases pointing to destroyed windows.

### SQLite integrity

- Uses parameterized SQL throughout.
- Transactions on multi-step operations.
- WAL mode with `busy_timeout=5s`.
- Server epoch cleanup on tmux restart.

### Fail-closed on collision

Ownership marker ensures tmh never touches a user-created `active` session. All operations fail early with a clear error message.

## Disable and rollback

### Disable the feature

1. Check current state: `tmh active status`
2. Recover orphans: `tmh active recover @ID --to session` (for each orphaned window)
3. Remove active aliases: `tmh active prune` (only safe unlinks)
4. Set `defaults.tmux_integration.active.enabled: false` in `config.yml`
5. Update tmux conf: `tmh tmux setup --append` (removes the hook slot)
6. Source tmux conf: `tmux source-file ~/.tmux.conf`

**Result:** The `active` session may still exist with stale aliases, but the hook is removed and future attaches work as before.

### Downgrade to older version

1. Follow the disable procedure above.
2. Remove the YAML block if the older version's schema rejects it.
3. The SQLite `active_windows` table remains (older versions ignore unknown tables).
4. Do not manually edit `state.db`.

### Binary rollback safety

Older binaries:
- Ignore unknown config fields (`tmux_integration.active`).
- Ignore unknown SQLite tables (`active_windows`).
- Continue to work with your existing sessions.

## Troubleshooting

### Hook not firing

**Symptom:** TTL not extending on native navigation.

**Diagnosis:**

```sh
tmh tmux audit
```

Check for:
- Hook presence in slot `[1708]`.
- Hook conflicts (another tool using the same slot).
- Hook syntax errors.

**Fix:**

```sh
tmh tmux setup --append
tmux source-file ~/.tmux.conf
```

### Collision error

**Symptom:** `active status` shows `collision: true`.

**Cause:** You created your own `active` session, or another tmux tool manages it.

**Fix:**

1. Rename your session: `tmux rename-session active my-active`
2. Or disable the feature: `defaults.tmux_integration.active.enabled: false`

### Orphaned windows

**Symptom:** `prune` reports orphaned windows that cannot be removed.

**Cause:** Window was last linked in `active`, and the source session was deleted or the window was killed by another tool.

**Fix:**

```sh
# See which windows are orphaned
tmh active status

# Recover each orphan to an existing session
tmh active recover @124 --to work
```

### State DB corruption

**Symptom:** `active status` or `prune` fails with SQLite errors.

**Fix:**

```sh
mv ~/.local/state/tmh/state.db ~/.local/state/tmh/state.db.broken.$(date +%s)
```

tmh will create a fresh DB on next run. You'll lose:
- Active session tracking data.
- Snapshots / undo / trust decisions.

### Verification checklist

After setup:

```sh
# 1. Verify feature is enabled
tmh active status  # should show enabled: true

# 2. Verify hook is installed
tmh tmux audit    # should show clean bill of health

# 3. Test navigation
tmh attach work:editor
# Use prefix n, prefix p, prefix 1, prefix l
# Attach should go through active session

# 4. Test expiry
# Set a short TTL (e.g., 30m), navigate, wait, check status
tmh active status --json | jq '.entries[] | select(.expired)'
```

## Architecture details

### Layering

```
cmd/tmh/cmd/active.go     → CLI wrapper
internal/actions/active.go → Promotion, touch, prune, remove, recover
internal/state/active.go   → SQLite persistence
internal/tmux/runner.go    → LinkWindow, UnlinkWindow, ListWindowLinks
```

### State DB schema

Additive table (does not affect other tables):

```sql
CREATE TABLE IF NOT EXISTS active_windows (
  server_key          TEXT NOT NULL,
  server_epoch        TEXT NOT NULL,
  window_id           TEXT NOT NULL,
  source_session_id   TEXT NOT NULL,
  source_session_name TEXT NOT NULL,
  window_name         TEXT NOT NULL,
  promoted_at         INTEGER NOT NULL,
  last_selected_at    INTEGER NOT NULL,
  expires_at          INTEGER NOT NULL,
  state               TEXT NOT NULL CHECK (state IN ('tracked', 'orphaned')),
  orphaned_at         INTEGER,
  PRIMARY KEY (server_epoch, window_id)
);
```

**Indexes:**

- `(server_epoch, expires_at)` — for expiry queries
- `(server_key, server_epoch)` — for stale cleanup on server restart

### Server epoch

tmux `window_id` is stable only for the lifetime of the tmux server. After restart, IDs may be reused.

**Epoch formula:**

```
server_key   = socket_path
server_epoch = canonical(socket_path, start_time, pid)
```

On each operation, tmh:
1. Checks if the current epoch matches stored rows.
2. Deletes stale rows from previous server restarts.
3. Only then matches `@ID` to tracking data.

This prevents accidental linking to wrong windows after tmux restart.

## References

- [Architecture guide](./architecture.md) — overall codebase layering
- [Versioning policy](./versioning.md) — public API surface and release cadence
- [README](../../README.md) — main project documentation

## FAQ

**Q: Can I use `move-window` instead of `link-window`?**

No. `move-window` would physically move the window, breaking the declarative model and causing drift. tmh only uses `link-window`.

**Q: What happens if I kill the `active` session manually?**

The aliases disappear, but the physical windows remain in their source sessions. On next navigation, tmh recreates the `active` session and re-promotes windows as needed.

**Q: Can I change the session name from `active`?**

No. The name is fixed for safety and simplicity. If you need your own `active` session, disable the feature or rename yours.

**Q: Does this work with `tmux attach -t session` (without tmh)?**

Yes, but TTL will not be extended (no hook runs). Use `tmh attach` or the picker for full TTL extension.

**Q: Can I have multiple active sessions?**

No. Only one `active` session managed by tmh. Multiple sessions would break the single-session navigation model.

**Q: Is the `active` session included in `tmh ls`, `tmh diff`, `tmh sync`, or `tmh freeze`?**

No. The `active` session is excluded from all declarative workflows when the feature is enabled. It appears in `tmh active status` only.

**Q: What if I set a very long TTL (e.g., 720h)?**

The window will stay in `active` for up to 30 days. This is fine for long-running workflows. The maximum is capped at 720h for safety.

**Q: Does the feature work with `tmuxinator` or `tmuxp`?**

Yes. The `active` session is managed by tmh's hook and does not interfere with other tools' session management, as long as they don't create a conflicting `active` session.

**Q: Can I use this in a tmux configuration that uses `automatic-rename`?**

`tmux tmux audit` will report `automatic-rename=on` as a conflict. Disable it for best results: `set-option -gw automatic-rename off`.

**Q: What happens if the hook fails (e.g., `tmh` binary not in PATH)?**

The hook silently fails as a no-op. Native navigation still works, but TTL does not extend. The window may expire sooner than expected.