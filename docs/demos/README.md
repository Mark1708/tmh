# tmh demos

This directory contains Charm VHS tapes and their rendered GIF outputs.

| Tape | Output | Shows |
|------|--------|-------|
| [`demo-picker.tape`](./demo-picker.tape) | [`demo-picker.gif`](./demo-picker.gif) | Bare `tmh` fuzzy picker |
| [`demo-tour.tape`](./demo-tour.tape) | [`demo-tour.gif`](./demo-tour.gif) | Full TUI dashboard tour |
| [`demo-workflow.tape`](./demo-workflow.tape) | [`demo-workflow.gif`](./demo-workflow.gif) | Declare, detect drift, and freeze workflow |

Render with `make demo`. The render script uses a temporary HOME and a
private tmux socket so it does not touch your real tmux server or tmh
config.
