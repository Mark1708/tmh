Output ./artifacts/tmh-next-tour.gif

# Recorded with VHS 0.11.0 exactly (see VERIFICATION.md); the 0.11 parser
# takes Require without version constraints, so the pin is documented here
# and asserted by the acceptance gate.
Set Shell bash
Set FontSize 14
Set Width 1600
Set Height 1000
Set TypingSpeed 60ms

# Deterministic capture: no periodic ticks, default fixture.
Type "./bin/tmh-next --demo --live=false --scenario default"
Enter
Sleep 700ms

# Picker-first startup: quick switch overlay over the Dashboard.
Sleep 600ms

# Dashboard
Escape
Sleep 600ms

# Animated guided help: watch attach steps, switch to terminal creation,
# pause, then launch its Workspaces destination.
Type "?"
Sleep 3200ms
Type "l"
Sleep 2200ms
Type " "
Sleep 600ms
Enter
Sleep 900ms
Escape
Sleep 500ms

# Agents via command palette (filter settles, one Enter chooses)
Ctrl+p
Sleep 500ms
Type "Agents"
Sleep 1200ms
Enter
Sleep 700ms

# Select the needs_input agent and prompt it (mock)
Type "j"
Sleep 250ms
Type "p"
Sleep 500ms
Type "use fiscal 2025 and summarize drift"
Sleep 400ms
Enter
Sleep 900ms

# Snapshots → create a restore plan → Reconcile
Ctrl+p
Sleep 500ms
Type "Snapshots"
Sleep 1200ms
Enter
Sleep 700ms
Type "jjj"
Sleep 400ms
Type "r"
Sleep 1000ms

# Reconcile: pull mode, apply with confirmation, then field-level undo
Type "Tab"
Sleep 500ms
Type "a"
Sleep 500ms
Type "y"
Sleep 900ms
Type "u"
Sleep 900ms

# Backends: probe tmux
Ctrl+p
Sleep 500ms
Type "Backends"
Sleep 1200ms
Enter
Sleep 700ms
Type "j"
Sleep 300ms
Type "p"
Sleep 900ms

# Machines: ping the local node
Ctrl+p
Sleep 500ms
Type "Machines"
Sleep 1200ms
Enter
Sleep 700ms
Type "jjp"
Sleep 900ms

# Settings: category browser → focused History form → validated save
Ctrl+p
Sleep 500ms
Type "Settings"
Sleep 1200ms
Enter
Sleep 700ms
Type "jj"
Sleep 600ms
Enter
Sleep 800ms
Enter@400ms
Enter@400ms
Enter@400ms
Type "y"
Sleep 1000ms

# Search: jump to the owning agent context
Ctrl+p
Sleep 500ms
Type "Search"
Sleep 1200ms
Enter
Sleep 700ms
Type "ledger"
Sleep 1200ms
Enter
Sleep 1000ms

# Clean quit
Type "q"
Sleep 600ms
