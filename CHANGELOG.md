# Changelog

All notable changes to tmh are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html) from
1.0.0 onwards. The **public API surface** for semver purposes is:

- the CLI subcommand and flag set,
- the `config.yml` schema (fields, types, required fields, coerced values),
- exit codes.

Internal Go packages (`internal/...`) are explicitly **not** part of the
public API.

## [1.0.0] — 2026-05-XX (upcoming)

First public release. Highlights (English summary):

- **Declarative tmux sessions** — keep your tmux layout in a commented YAML
  and verify live state against it with `tmh diff` / `tmh watch`.
- **Drift detection as verification** (not enforcement) — tmh shows what
  changed; you decide whether to `push` the live state into the config, or
  `init` the config into tmux.
- **Rich TUI dashboard** with palette, fuzzy filter, marks, snapshots +
  undo, inline per-pane process visibility, toast notifications, theming.
- **20+ CLI subcommands** (`attach`, `new`, `init`, `ls`, `ps`, `sync`,
  `diff`, `reload`, `watch`, `status`, `doctor`, `snapshot`, `undo`,
  `export`, `import`, `tmux audit`, …).
- **SQLite state store** (pure Go, no CGO) for history, marks, hook
  trust-hashes, snapshots.
- **i18n out of the box** — English + Russian bundled.
- **Hardened defaults** — config, history, and state DB files are written
  with `0600` perms; hooks require trust-prompt on first use and on hash
  change.

Detailed Russian-language changelog for the post-MVP refactor cycle that
culminated in 1.0.0 follows below; it is preserved verbatim for historical
reference.

---

Большой рефакторинг после MVP: персистентность, process visibility, улучшения UX.

---

### Added

#### Block 1 — Foundation

**1.1 История действий (JSONL persistence)**
- `internal/state/history.go` — append-only JSONL в `~/.local/state/tmh/history.jsonl`
- Формат: `{"ts":"RFC3339","action":"...","target":"...","result":"ok|err","details":"..."}`
- Size cap: `defaults.history.max_entries` (по умолчанию 1000), amortized rewrite при >1.2×max
- Age cap: `defaults.history.retention` (7d/30d/90d/forever, по умолчанию 30d)
- Corrupt handling: переименование в `history.jsonl.corrupt-<ts>`, старт с чистого листа
- Archive on clear: `defaults.history.archive_on_clear: true` → `history.jsonl.archived-<ts>`
- Загрузка async через `tea.Cmd` в `Init()`, запись debounced
- Очистка из TUI: History screen → `X` с подтверждением

**1.2 Toast infrastructure upgrade**
- `internal/ui/toast/toast.go` — `Kind` enum (Info/Success/Error) с TTL (2s/3s/5s)
- Tag-compare counter: старый `tea.Tick` не гасит более новый toast
- Визуальное оформление: prefix `[✓]`/`[!]`/`[i]` + цветовая схема по Kind

**1.3 Settings — полный редизайн**
- Master-detail layout: категории слева, поля справа
- 7 категорий: Внешний вид, Отображение, История, Метки, Tmux, Поведение, Горячие клавиши
- Live-apply для тема/язык/display-полей, Ctrl+S для tmux-полей
- Tmux-секция пишет в `~/.config/tmh/tmux.conf` (include-файл), не в `~/.tmux.conf`
- Dirty-state перехватывает Esc → confirm

**1.4 Auto-refresh infrastructure**
- `internal/ui/refresh/refresh.go` — periodic batch `tmux list-panes -a`
- Input-pause: fetch не запускается при открытой palette/filter
- Seq-based debounce: устаревшие результаты игнорируются
- `internal/ui/pane/provider.go` — thread-safe кэш `PaneInfo` с TTL 2s

#### Block 2 — Process visibility

**2.1 PaneCmdProvider**
- `pane.Provider` с `Get`, `SetAll`, `Invalidate`, `CommandsForSession`, `CommandsForWindow`, `FindByCommand`, `Stats`
- `IsIdleShell()` — определяет idle-shells (bash, zsh, sh, fish и login-варианты)

**2.2 Вариант 4 — агрегация процессов**
- Сессия в дереве показывает уникальные не-idle процессы (напр. `claude vim`)
- Окно показывает процесс первой не-shell панели

**2.3 Вариант 10 — preview привязан к paneID**
- `ShiftTab` в detail-панели листает панели для preview
- Заголовок: `── preview [pane 0: nvim] ──`
- При смене окна — сброс на `preview_default_pane` из Settings

**2.4 Вариант 6 — `/` inline-фильтр дерева**
- Двухслойная индексация: `filtered []int` + cursor-by-ID
- Счётчик `3/42` в футере
- `Enter` удерживает фильтр с blur, `Esc` сбрасывает
- Cursor-by-ID: при пересчёте фильтра курсор остаётся на том же элементе

**2.5 Вариант 9 — процессы + cwd в detail-панели**
- Для каждой панели окна: маркер `▶`, индекс, команда (10 chars), сокращённый cwd
- `shortenPath()` сворачивает home-dir в `~/`

**2.6 Вариант 20 — `tmh ps` CLI**
- `cmd/tmh/cmd/ps.go` — `tmh ps [--session <name>] [--format table|json|tsv]`
- Колонки: SESSION, WINDOW, PANE, CMD, PID, CWD
- Консистентен с detail-панелью TUI

#### Block 3 — Phase 2 enhancements

**3.1 Вариант 15 — Tab-toggle inline панелей**
- Трёхуровневое плоское дерево: session (0), window (1), pane (2)
- `Tab` на строке окна — развернуть/свернуть панели inline
- `←` на строке панели — свернуть и вернуться к окну
- `SelectedLevel()` используется для контекстных действий

**3.2 Вариант 14 — footer heatmap**
- `live N · idle N` в правой части футера
- Управляется `defaults.display.show_footer_heatmap`
- Обновляется в том же тике что и auto-refresh

**3.3 Вариант 18 — `command:` в config + process drift**
- `config.Drift` расширен: `ReasonCode: "command_differs"`, `ConfigCommand`, `LiveCommand`
- При `command: nvim` в конфиге и реально запущенном `zsh` — drift в detail-панели

**3.4 Вариант 11 — inline drift в detail**
- `drift   nvim ≠ expected: zsh` в accent-цвете для window/pane rows

**3.5 Вариант 8 — palette action "goto process"**
- Параметрический action в палитре: `:goto process` → запросить имя → jump
- `pane.Provider.FindByCommand()` — поиск по substring (case-insensitive)
- Перед прыжком пушит текущую позицию в last-location ring

**3.6 Вариант 16 — контекстный `d`**
- `d` на строке сессии → `KillSession`
- `d` на строке окна → `KillWindow`
- `d` на строке панели → `KillPane`
- Confirm всегда уточняет конкретный target

#### Block 4 — UX improvements

**4.1 Last-location (`''`)**
- `internal/state/marks.go` — ring 10 позиций (target + cursorIdx), JSON persistence
- Любой прыжок (`attach`, `'<letter>`) пушит текущую позицию
- `''` — pop + jump (vi-стиль: swap current ↔ prev без дублирования ring)
- Футер показывает `'' ← prev` когда ring непуст
- `NewMarksStoreAt(path)` для тестов

**4.2 Marks (`m<letter>` / `'<letter>`)**
- Двухступенчатый input: `m` / `'` → 2s timeout → второй символ = буква метки
- `pendingOpExpiredMsg` с tag-compare не ломает конкурентные timeout'ы
- Toast: "метка a установлена" / "метка a не найдена"
- При kill target'а → `InvalidateMark(target)` автоматически
- JSON persistence через `MarksStore`

**4.3 Undo visibility в футере**
- `↶ kill session atlas` в футере после destructive-действия
- `u` очищает hint при успешном undo

**4.4 Context-sensitive `?` help**
- `modeHelpText()` — диспетчеризация по `m.current`:
  - Dashboard: полный keymap + marks-клавиши
  - Settings: навигация по полям
  - History: scroll + clear
  - Palette: filter + navigate
  - Confirm: y/n/t (dry-run)
- `buildModeHelp()` — переиспользуемый builder для modal-помощи

**4.5 Параметрические palette actions**
- `PaletteAction.NeedsParam bool` + `ParamRun func(string) tea.Cmd`
- Palette переключается в режим ввода параметра после выбора action'а
- `Esc` возвращает к выбору без выполнения
- Добавлены: `mark: set mark`, `goto: jump to process`

**4.6 Dry-run mode**
- `confirmModel.DryRunDesc string` — описание что было бы сделано
- `t` в confirm-dialog → info-toast `[dry-run] would kill <target>` без выполнения
- Kill-confirm всегда получает `DryRunDesc`

#### Cross-cutting

**Structured logging**
- `internal/slogx/slogx.go` — `Init()` читает `TMH_LOG` env
- JSON-формат, output в `~/.local/state/tmh/tmh.log`
- Rotation: 5 MB × 3 файла (`tmh.log`, `tmh.log.1`, `tmh.log.2`, `tmh.log.3`)
- `TMH_LOG` не задан → logger полностью отключён (no-op handler)
- Уровни: `debug`, `info`, `warn`, `error`

**Config — новые поля**
- `defaults.history`: `max_entries`, `retention`, `archive_on_clear`
- `defaults.display`: `show_processes_in_tree`, `show_footer_heatmap`, `preview_default_pane`, `tree_density`
- `defaults.behaviour`: `auto_refresh_interval`, `dry_run_default`, `confirm_on_kill`, `optimistic_rendering`
- `defaults.marks`: `persist_across_sessions`
- `defaults.tmux_integration`: `mouse_mode`, `escape_time_ms`, `base_index`, `pane_base_index`, `status_right_integration`
- Все поля optional; старые конфиги без них не ломаются

---

### Fixed

- **Data race**: `m.cfg` больше не пишется из `tea.Cmd` goroutine — перенесено в `dataLoadedMsg.Cfg`, присвоение в `Update`
- **Data race**: `gotoProcCmd` больше не обращается к `m.dashboard` из worker goroutine — заменено на `gotoProcMsg` pattern
- **Logic bug**: `''` (last-location) pop-before-push создавал дублирующиеся записи в ring при повторных нажатиях — исправлено: push только если текущий target отличается от pop'нутого
- **Pane cache key**: `CommandsForWindow` теперь фильтрует по точному prefix `session:windowIdx.` вместо случайного совпадения подстроки
- **`restoreCursorByID`**: корректно работает на трёхуровневом дереве после добавления pane-level строк
- **`os.Rename` temp file**: при ошибке rename временный файл теперь удаляется вместо того чтобы оставаться на диске

---

### Changed

- `dashboardRow.IsSession bool` → `dashboardRow.Level int` (levelSession/levelWindow/levelPane) — breaking internal API
- `PaletteAction` расширен полями `NeedsParam`, `ParamPrompt`, `ParamRun` — backward-compatible (nil = старое поведение)
- `confirmModel` расширен полем `DryRunDesc` — backward-compatible (empty = нет dry-run)
- `dataLoadedMsg` расширен полем `Cfg` — `m.cfg` назначается только в Update

---

### Testing

Новые тесты:
- `internal/state/marks_test.go` — 10 тестов: set/get, persistence, invalidate, AllMarks, clear, ring cap, newest-first, empty pop, corrupt fallback, atomic write
- `internal/config/migration_test.go` — smoke-parse всех fixtures, new-fields defaults, legacy full resolve
- `internal/ui/ui_test.go` — filter cursor-by-ID (cursor сохраняется при смене фильтра), filter boundary check (пустые результаты не паникуют)
- `testdata/configs/` — minimal_v1.yml, empty.yml, legacy_no_new_fields.yml
