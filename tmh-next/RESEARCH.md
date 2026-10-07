# RESEARCH.md — source evidence for tmh-next demo

Date: 2026-10-06. All checkouts are **detached shallow clones at the pinned SHA** under
`/Users/mark/tmp/tmh-next-research/<name>`; `git rev-parse HEAD` was verified byte-exact
against the pinned SHA for every repo before implementation. Moving upstream HEAD was
**not** consulted and never substitutes a reviewed commit.

## 1. Studied application repos (source matrix)

| Repo | URL | Detached SHA | Commit context (subject, date) | Anchors read | Adopted | Rejected |
|---|---|---|---|---|---|---|
| pop | https://github.com/charmbracelet/pop | `cc5a0a73fb7f0841ad49394c7edf14e08e99efd2` | `v0.5.0`, 2026-08-20 | `model.go`: `type State int`, states `editingFrom/editingTo/pickingFile/hoveringSendButton`, `switch m.state` in `Update`/`View`/cursor math; `auth_ui.go`: `authStateIntro/Preparing/Waiting/Exchanging`, spinner fan-out; `keymap.go`, `style.go` (semantic styles); `main.go` | explicit state machine; state-aware keys/help; typed async messages; `tea.NewView` usage; inline recoverable errors | OAuth/HTTP flows, config/release plumbing. Note: planned anchor `model_test.go` does **not exist** at this SHA (no test file in tree) — recorded as deviation, replaced by reading `model.go` state machine directly |
| mods | https://github.com/charmbracelet/mods | `0425d0d7861e4bbc396200b3bd2eee1825715300` | `docs(README): update sunset notice`, 2026-03-09 | `mods.go`: `(m *Mods) Update`, `receiveCompletionStreamCmd(msg completionOutput)`, `resolveModel`; `internal/stream/stream.go` typed stream boundary; `mods_test.go` deterministic command tests | start/ready/error states; typed stream boundary; follow-only-at-bottom discipline; deterministic command tests | providers, credentials, MCP/cache/editor branches, whole-document rerendering. Planned anchor `keymap.go` does **not exist** at this SHA (bindings inline in `mods.go`) — deviation recorded |
| wishlist | https://github.com/charmbracelet/wishlist | `36196a7935f24f2fae8325b06e3611b595819e02` | `chore(deps): bump the all group with 3 updates (#447)`, 2026-10-01 | `wishlist.go`, `listitem.go` (stable item identity, `FilterValue`), `styles.go` (injected styles), `wishlist_test.go` | Bubbles list adapter pattern; stable item identity; filter-aware keys; margin-aware sizing | SSH server/auth/discovery, server config |
| vhs | https://github.com/charmbracelet/vhs | `24fa2254a9806091e6ee6a980e9f3bcfe0a9ba53` | `v0.12.1`, 2026-09-24 | `vhs.go`: `type VHS struct`, `New()`; `command.go` typed commands; `evaluator.go`: `Evaluate(ctx, tape, out, opts...)`; `style.go` | explicit lifecycle; typed intents; single-owner sequencing; readiness/error/cleanup invariants (also our demo.tape target tool) | recorder/PTTY/SSH/export implementation details |
| soft-serve | https://github.com/charmbracelet/soft-serve | `6b169bc3af4d407aec43fb64c606d02af520bd0e` | `v0.12.3`, 2026-10-05 | `pkg/ssh/ui.go` (root routing), `pkg/ui/pages/selection/selection.go`: `type Selection struct` (list→detail), `pkg/ui/pages/repo/repo.go`, `pkg/ui/components/selector/selector.go` | root routing; list → detail; child message fan-out; contextual help; recoverable error screen | SSH/database/auth/Git backend, server-only concerns |
| glow | https://github.com/charmbracelet/glow | `8b0600fa445535b9c53194c0d4f7fe3cf700ef3e` | `chore: bump go (#1051)`, 2026-10-02 | `ui/ui.go` (responsive root router), `ui/stash.go`: `type stashModel struct`, `ui/styles.go` | responsive root router; list/detail composition; typed async results; terminal-background adaptive styles | Markdown renderer, filesystem/editor integration, raw terminal probing |
| skate | https://github.com/charmbracelet/skate | `42427dcc6e405bb1e4bfd46c2b5990e29bdda8a3` | `v1.1.0`, 2026-09-28 | `main.go` (Cobra CLI: `set/get`, `errDBNotFound` actionable error), `go.mod` | small explicit CLI/data boundary; actionable errors | whole current architecture: synchronous Cobra CLI, not a Bubble Tea TUI; historical Skate TUI not used as current architecture |

No nested `.git` directories, source files, auth/server/release scaffolding were copied
into the Go module — checkouts are read-only evidence.

## 2. Charm v2 API appendix (pinned modules)

Recorded from `go mod download -json` inside the module on 2026-10-06 (Go 1.26.0,
`GOTOOLCHAIN=auto` downloaded go1.26.0 over local go1.22.5):

| Module | Version | Dir sum (h1) | GoMod sum (h1) |
|---|---|---|---|
| `charm.land/bubbletea/v2` | v2.0.10 | `oolvo20VBpI0PfqE7iFjkZ1bx0WpmXGfnKz5Yldjq5o=` | `QOatcnhOjYIfxzUSTz6raF7Ex4R/rIuHa3SnBdCCpMc=` |
| `charm.land/bubbles/v2` | v2.2.1 | `Fq1+qm5hV6GkvzLQDhCBpXXE5tLgvh1PRriCLwSvIQU=` | `wdMgn+sje1KNXdwFizIWjbf328fIUBxqEmJ/vYPo8yc=` |
| `charm.land/lipgloss/v2` | v2.0.6 | `EaGKeuA8FvF+v2BT5VmZd2LoYLaMZJXA5n34th8nCIQ=` | `ipDDJNSGa1hlwDtSfW1s2/xR8Vdhbut4PXh2zEKZd0Q=` |
| `charm.land/huh/v2` | v2.0.3 | `2cJsMqEPwSywGHvdlKsJyQKPtSJLVnFKyFbsYZTlLkU=` | `93eEveeeqn47MwiC3tf+2atZ2l7Is88rAtmZNZ8x9Wc=` |

Primary anchors read in the module cache (`$GOMODCACHE/charm.land/...`):

### bubbletea/v2@v2.0.10 — `tea.go`, `color.go`, `options.go`, `commands.go`, `key.go`, `UPGRADE_GUIDE_V2.md`

- `type Model interface { Init() Cmd; Update(Msg) (Model, Cmd); View() View }` — `View()` returns **`tea.View`**, not `string`.
- `type View struct { Content string; AltScreen bool; ReportFocus bool; WindowTitle string; MouseMode; ForegroundColor, BackgroundColor color.Color; ... }`; `func NewView(s string) View`.
- Keys: `tea.KeyPressMsg` (struct `{ Code rune; Text string; Mod KeyMod; ... }`), `msg.String()` yields `"q"`, `"space"`, `"ctrl+c"`, `"enter"`, `"esc"`, `"tab"`, `"up"`, ...; `tea.KeyMsg` is the press+release interface.
- Async: `tea.Cmd func() Msg`, `tea.Batch`, `tea.Sequence`, `tea.Tick(d, func(time.Time) Msg)`, `tea.Every`, `tea.Quit`.
- Background: `tea.RequestBackgroundColor() Msg` (returned from `Init`), response `tea.BackgroundColorMsg` with `.IsDark()` (and `ForegroundColorMsg`).
- Program: `tea.NewProgram(model, opts...) *Program`; `p.Run() (Model, error)`; test-friendly options `WithOutput(io.Writer)`, `WithInput(io.Reader)` (nil disables input+queries), `WithColorProfile`, `WithWindowSize(w,h)`, `WithoutRenderer`, `WithoutSignals`.

### bubbles/v2@v2.2.1 — `list/`, `table/`, `viewport/`, `textinput/`, `help/`, `key/` (+`UPGRADE_GUIDE_V2.md`)

- `list.New(items []Item, delegate ItemDelegate, width, height int) Model`; value-update `(Model, tea.Cmd)`; `SetItems`, `Select(i)`, `Index()`, `VisibleItems()`, `SelectedItem()`, `FilterState()` (`Unfiltered/Filtering/FilterApplied`), `SetSize/SetWidth/SetHeight`, `DefaultStyles(isDark bool)`; built-in `/` filter with `Esc` semantics.
- `viewport.New(opts ...Option)` (`WithWidth`, `WithHeight`); `SetContent`, `AtBottom() bool`, `GotoBottom()`, `YOffset()/SetYOffset()`, `SoftWrap`.
- `textinput.New()`; `SetWidth`, `Value()/SetValue`, `Focus()/Blur`, `DefaultStyles(isDark)`.
- `table.New(table.WithColumns/WithRows/WithFocused/WithWidth)`; `SetRows`, `Cursor()`, `SelectedRow()`, `SetWidth/SetHeight`.
- `help.New()`; `SetWidth`, `View(k help.KeyMap)`, `ShortHelp()/FullHelp()`; `help.DefaultStyles(isDark)`.
- `key.NewBinding(key.WithKeys(...), key.WithHelp(k, d))`; `Binding.Enabled()`, `.Help()`, `.SetEnabled(bool)`.

### lipgloss/v2@v2.0.6 — `canvas.go`, `layer.go`, `size.go`, `join.go`, `color.go`

- `NewCanvas(width, height int) *Canvas`; `Compose(uv.Drawable) *Canvas`; `Render() string` (trailing space trimmed); `Clear/Resize`.
- `NewLayer(content string, children ...*Layer) *Layer`; chainable `.X(int)`, `.Y(int)`, `.Z(int)`, `.ID(string)`; `NewCompositor(layers ...*Layer) *Compositor` (z-sorted flatten, `Render()`, `Hit(x, y)`); Compositor and Layer implement `uv.Drawable` → composable onto `Canvas`.
- `lipgloss.Width(s string) int` / `Height(s string) int` — ANSI-aware; `JoinHorizontal/JoinVertical(pos, strs...)`; `lipgloss.Color("#hex") color.Color`.

### huh/v2@v2.0.3 — `README.md`, `form.go` (+ `internal/compat/model.go`, `theme.go`)

- `huh.NewForm(groups ...*Group) *Form`; `NewGroup(fields ...Field)`; fields `NewInput/NewSelect[T]/NewConfirm/NewNote` with `.Title/.Description/.Value(*T)/.Options(...)/.Validate(func)`.
- `Form.WithWidth/WithHeight/WithTheme(Theme)/WithKeyMap/WithShowHelp/WithShowErrors`; lifecycle as embedded model: `f.Init() tea.Cmd`, `f.Update(tea.Msg) (huh.Model, tea.Cmd)` where `type Model = compat.Model` (v1-style: `View() string`) — **caller must store the returned model each Update**; `f.State` (`StateNormal/StateCompleted/StateAborted`); `f.Errors() []error`.
- Cross-field rules go in a final `Confirm.Validate` closure over the draft; `StateCompleted` after final submit. Catppuccin theme available for consistent look (`ThemeCatppuccin`), plus `ThemeBase()`.

## 3. Upstream HEAD

Not consulted. The pinned revisions above are the sole reviewed evidence; if newer
upstream is ever interesting it must be recorded as separate optional evidence, never
replacing the reviewed SHA.
