# tmh

Single-binary TUI-хаб для tmux. Декларативные YAML-сессии, sync между live и config, перезагрузка dotfiles, конфиг который не стыдно положить в git и поделиться с коллегой.

Собран потому что zsh-функции вокруг `tmux` плохо масштабируются: один INI-файл, 9 алиасов, нет диффа, нет undo, нет sharing.

---

## Содержание

- [Установка](#установка)
- [Первый запуск](#первый-запуск)
- [Быстрый старт](#быстрый-старт)
- [Конфиг `config.yml`](#конфиг-configyml)
- [CLI-справочник](#cli-справочник)
- [TUI-дашборд](#tui-дашборд)
- [Hooks и trust](#hooks-и-trust)
- [Snapshots и undo](#snapshots-и-undo)
- [Sharing с коллегой](#sharing-с-коллегой)
- [Troubleshooting](#troubleshooting)
- [Архитектура](#архитектура)

---

## Установка

### Из self-hosted git через `go install`

```sh
export GOPRIVATE=git.mark1708.ru/*
go install git.mark1708.ru/me/tmh/cmd/tmh@latest
```

Нужен Go 1.25+ (из-за `modernc.org/sqlite`). Если `go get` не резолвит путь — проверь что `ssh-add -l` видит твой ключ для `git.mark1708.ru`.

### Homebrew direct-formula

```sh
brew install https://git.mark1708.ru/me/tmh/raw/branch/main/homebrew/tmh.rb
```

### Из исходников

```sh
git clone ssh://git@git.mark1708.ru:2224/me/tmh.git
cd tmh
go build -o ~/.local/bin/tmh ./cmd/tmh
```

Проверка:

```sh
tmh version
tmh doctor
```

`doctor` проверяет: tmux ≥ 3.2, `$SHELL`, наличие и валидность `config.yml`, `state.db` integrity, PATH, fd, terminal-notifier, GOPRIVATE.

---

## Первый запуск

Если `~/.config/tmh/config.yml` отсутствует, `tmh` предложит 4 варианта (при TTY):

1. **start empty** — минимальный config с `version: 1` и пустыми секциями.
2. **import from live tmux** — запустить `sync --pull --bootstrap`, автоматически собрать roots через LCP-алгоритм, импортировать все живые сессии как записи в `sessions:`.
3. **import from file / URL** — прочитать готовый YAML (например от коллеги).
4. **quit**.

Рекомендуется вариант 2, если у тебя уже запущен tmux — поднимется честный YAML со всеми окнами:

```sh
tmh sync --bootstrap
```

После этого `cat ~/.config/tmh/config.yml` содержит `roots:` (инфернутые префиксы) и `sessions:` с `root: <ключ>` и `path: ...` для каждого окна.

В non-TTY режиме (pipe, CI) создаётся пустой config молча, `tmh` продолжает работать — команды `ls`, `attach`, `kill`, `reload --shell`, `popup`, `scratch`, `window` работают без config (pass-through).

---

## Быстрый старт

```sh
# Импортировать текущий tmux в config.
tmh sync --bootstrap

# После ребута машины — поднять всё одной командой.
tmh init

# Посмотреть что есть и где drift.
tmh ls
tmh diff

# Переключиться между окнами.
tmh attach epcp:lk             # вне tmux → attach
                               # внутри tmux → switch-client

# Синхронизация dotfiles в живые сессии (ключевая фича).
tmh reload --shell             # source ~/.zshrc во все idle shell-панели
tmh reload --shell --busy      # + поставить в очередь busy-панели
tmh reload --tmux              # tmux source-file ~/.tmux.conf
tmh reload --all               # оба сразу

# Без параметров — TUI-дашборд.
tmh
```

---

## Конфиг `config.yml`

Файл хранится в `~/.config/tmh/config.yml` (или по пути `$TMH_CONFIG`). YAML со структурными ссылками — без Mustache-шаблонизатора.

### Полный пример

```yaml
version: 1

# Именованные корневые каталоги, чтобы не дублировать длинные префиксы.
roots:
  otr: /Users/mark/Documents/Projects/otr
  me:  /Users/mark/Documents/Projects/me
  kb:  /Users/mark/Documents/Projects/me/products/kb/repos/manager/bases

# Значения по умолчанию, применяются если глубже не переопределены.
defaults:
  layout: 3-pane
  shell:  zsh
  popup:  { width: 80%, height: 60% }
  env:
    EDITOR: nvim

# Переиспользуемые шаблоны окон. В extends можно ссылаться только на
# templates — цепочка запрещена (ErrTemplateChain на валидации).
templates:
  kb_base:
    layout: 2-pane
    command: nvim .

# Произвольные tmux layout-хэши для экспериментальных раскладок.
# Получить свой: выставь окно как нравится → tmh layout save <имя>.
layouts:
  my-ide:
    hash: "5c3b,239x56,0,0{119x56,0,0,0,..."
    description: "left 50% editor, right top/bottom stacks"

# Профили — фильтр по group + опциональные env/defaults/hooks поверх.
profiles:
  work:
    groups: [work, otr]
    env:
      AWS_REGION: eu-central-1
  personal:
    groups: [me, kb]

# Сессии.
sessions:
  epcp:
    group: [work, otr]
    root:  otr
    path:  products/epcp/repos
    env:
      KUBE_CONTEXT: epcp-dev
      AWS_PROFILE:  epcp
    on_attach:
      - mise use
    windows:
      # shorthand: bare string = { dir: <value> }, путь относительно root
      lk:        lk-mosru-epcp
      mdr:       mdr
      filings:   filings
      # полная форма с шаблоном и command
      kb:
        extends: kb_base
        root:    kb
        path:    epcp
```

### Схема окна

```yaml
windows:
  имя:
    dir:      string           # абсолютный или относительный
    root:     string           # ключ из roots.<...>
    path:     string           # альтернатива dir для root-based
    layout:   string           # 1-pane | 2-pane | 3-pane | <layouts.<ключ>>
    command:  string           # команда для главной панели
    extends:  string           # ключ из templates.<...>
    env:      { KEY: VALUE }   # env overrides
    focus:    bool             # активное окно после init
    panes:                     # явная раскладка панелей
      - dir: ...
        command: ...
        env: {}
        focus: true
```

Короткая форма `имя: "строка"` эквивалентна `имя: { dir: "строка" }`.

### Разрешение путей

1. `dir` абсолютный → используется как есть.
2. `root` задан → `roots[root] / (path || dir)`.
3. `session.root` задан + `dir` относительный → `roots[session.root] / session.path / dir`.
4. Иначе → `$PWD / dir`.

Опциональный shorthand: строка начинается с `$key/...` — раскрывается в `{ root: key, path: ... }`. `$$` эскейп для литерального `$`. `tmh config lint` нормализует shorthand в канонический вид.

### Env merge

Порядок (более глубокий уровень переопределяет):

```
defaults.env
  → profiles[active].env
    → sessions[x].env
      → sessions[x].windows[y].env
        → sessions[x].windows[y].panes[z].env
```

Словарь не заменяется целиком — merge пары ключ-значение.

### Hooks

```yaml
on_create:  "docker compose up -d"    # scalar → [scalar]
on_attach:
  - mise use
  - kubectl config use-context dev
on_destroy:
  - docker compose down
```

Поддерживаются на уровне `sessions.<x>.hooks.*` и `profiles.<x>.hooks.*`. Profile-hooks конкатенируются **до** session-hooks.

При первом запуске конфига с hooks `tmh` запрашивает подтверждение и сохраняет хеш файла в `state.trust`. Повторный prompt только при изменении конфига.

### Валидация

```sh
tmh config validate
```

Проверяет:

- все `root:` ссылаются на существующий `roots.<ключ>` (`ErrUnknownRoot`),
- все `extends:` на `templates.<ключ>` (`ErrUnknownTemplate`),
- глубина `extends` ровно 1 (`ErrTemplateChain`),
- все `layout:` — builtin или `layouts.<ключ>` (`ErrUnknownLayout`),
- `panes[]` совместимы с builtin layout (`ErrLayoutMismatch`).

---

## CLI-справочник

```
tmh                       открывает TUI-дашборд
tmh version               напечатать версию
tmh doctor                проверка окружения
tmh completion {zsh|bash|fish}   скрипт автодополнения
```

### Сессии

```
tmh attach [имя|имя:окно]    attach (вне tmux) или switch-client (внутри)
tmh new [--name] [--dir] [--layout] [--group] [--save] [--attach]
                              без флагов — интерактивный wizard (huh-форма)
tmh init [--profile] [--only a,b]   поднять всё из config (недостающее)
tmh kill <pattern>           убить сессии по substring
tmh ls [--json] [--filter live|conf|drift]   дерево сессий/окон
tmh window [--dir]           новое ad-hoc окно в текущей сессии
tmh scratch [--dir] [--ttl 1h]   эфемерная сессия с опциональным TTL
```

### Sync и diff

```
tmh sync                    (default: --push) live ← config
tmh sync --pull [--all]     config ← live (добавить новые, обновить drift)
tmh sync --bootstrap        импорт всех live сессий в пустой config
tmh sync --dry-run          показать действия без применения
tmh diff [--json]           печать списка drift
```

Drift-статусы:

| Status  | Значение |
|---------|----------|
| `ok`    | окно в live и config идентично (root/dir совпадают) |
| `drift` | `pane_current_path` первой панели ≠ resolved dir |
| `new`   | окно в tracked-сессии появилось live, но не в config |
| `gone`  | окно в config, но не запущено |

### Dotfile sync

```
tmh reload                     (default --all) shell + tmux
tmh reload --shell             source ~/.zshrc в idle shell-панелях
tmh reload --tmux              tmux source-file ~/.tmux.conf
tmh reload --busy              не-idle панели в очередь, source когда освободятся
tmh reload --pick              интерактивно выбрать панели
tmh reload --respawn           kill-server + tmh init из snapshot (WIP)
tmh reload --status            показать deferred queue
tmh watch [--auto]             fsnotify + 300ms debounce на dotfiles
tmh status                     сегмент для tmux status-right: · / ⚠zsh / ⚠drift:N / ⏳N
```

### Config CRUD

```
tmh edit                     $EDITOR config.yml
tmh config get <path>        скаляр на stdout (для скриптов)
tmh config show [path]       pretty-print секции
tmh config set <path> <val>  set scalar
tmh config add <path> ...    добавить новую секцию с полями
tmh config rm <path>         удалить секцию
tmh config rename <path> <новое>  переименовать ключ
tmh config lint              нормализовать shorthand
tmh config validate          schema + static checks
```

### Snapshots / undo / export / import

```
tmh snapshot save <имя>       снимок живого состояния
tmh snapshot ls
tmh snapshot restore <имя>
tmh snapshot rm <имя>
tmh undo                      откатить последнее destructive действие
tmh export [имя] [--minimal]  YAML на stdout; --minimal чистит секреты
tmh import <путь> --merge|--replace
```

### Layouts и popup

```
tmh layout save <имя> [--description]   сохранить текущую раскладку окна
tmh popup <cmd> [--width] [--height] [--no-env] [--no-cwd] [--session] [--window]
                  запустить команду в tmux-popup с env/cwd из config
```

---

## TUI-дашборд

```
tmh                    без аргументов — дашборд
```

### Раскладка

```
┌─ tmh · ~/.config/tmh/config.yml ──── ⚠drift:2 ───────────┐
│  profile: —  ·  group: [all]  ·  / поиск                  │
├───────────────────────────────────────────────────────────┤
│  SESSIONS                  │  DETAIL                      │
│  ▼ ● epcp    7w   work     │  session: epcp               │
│    ├─ ● lk   3p   ok       │  live    true                │
│    ├─   mdr  3p   ok       │  windows 7                   │
│    ├─ ! jr   3p   drift    │  status  ok                  │
│    └─ …                    │                              │
│  ▼ ● kb      8w   kb       │                              │
├───────────────────────────────────────────────────────────┤
│ a attach · R dotfiles · s sync · S settings · : palette   │
└───────────────────────────────────────────────────────────┘
```

### Keymap

**Навигация**
- `j` / `k` / `↑↓` — вверх / вниз
- `h` / `l` — свернуть / развернуть сессию
- `g` / `G` — к началу / в конец
- `PgUp` / `PgDn` — постранично

**Действия с сессиями**
- `enter` / `a` — attach (tmux перехватывает терминал, возврат через `prefix d`)
- `n` — новая сессия через мастер (huh-форма)
- `d` — kill выбранной сессии (с подтверждением)
- `u` — undo последнего kill (recreate из snapshot)

**Sync / reload**
- `r` — обновить дерево TUI из tmux (без побочных эффектов)
- `R` — `source ~/.zshrc` + `tmux source-file ~/.tmux.conf`
- `s` — `sync --push` (создать недостающие окна)
- `D` — экран drift

**Прочее**
- `:` / `Ctrl+P` — палитра команд (fuzzy-поиск)
- `S` — настройки (живой выбор темы)
- `?` / `esc` — help on / off
- `q` / `Ctrl+C` — выход

### Темы

4 Catppuccin flavour: mocha, macchiato, frappe, latte. Менять через `S` (settings) либо `Ctrl+T` (cycle).

---

## Hooks и trust

`on_create`, `on_attach`, `on_destroy` — списки shell-команд которые выполняются на событиях. `sh -c`, env и cwd берутся из resolved config.

### Первый запуск конфига с hooks

```
⚠  config.yml содержит shell hooks:
    sessions.epcp.on_attach: mise use

Trust and run? [y/N]
```

После `y` SHA-256 хеш файла пишется в `~/.local/state/tmh/state.db`. Пока конфиг не меняется — повторный prompt не появится. После любой правки — повторно.

Флаг `tmh --no-hooks` полностью отключает выполнение (например для аудита).

---

## Snapshots и undo

**Snapshots** — именованные точки восстановления структуры всех живых сессий (окна + cwd + layout). Команды в панелях не сохраняются — выводится hint какой процесс был.

```sh
tmh snapshot save pre-demo
# ... развалил всё ...
tmh snapshot restore pre-demo
```

**Undo** — короткая история последнего destructive действия (пока только `kill_session`). Перед kill `tmh` сохраняет snapshot сессии в `events` таблицу, `tmh undo` восстанавливает из payload.

---

## Sharing с коллегой

Экспорт с вычищенными секретами и абсолютными путями:

```sh
tmh export --minimal > team.yml
```

`--minimal` делает:

- env ключи `*_TOKEN`, `*_KEY`, `*_SECRET`, `*_PASSWORD`, `*_PWD`, `*_API_KEY` → `<redacted>`;
- абсолютные `dir:` переписываются в `{ root, path }` когда префикс совпадает с объявленным root.

Коллега:

```sh
GOPRIVATE=git.mark1708.ru/* go install git.mark1708.ru/me/tmh/cmd/tmh@latest
tmh import team.yml --merge
tmh init
```

`--merge` — overlay на существующий конфиг (приходящая сторона побеждает на конфликтах). `--replace` — полная замена.

---

## Troubleshooting

**`tmh` зависает после `attach`**
`prefix d` внутри tmux отдетачит и вернёт TUI. Если совсем застряло — `Ctrl+\` (SIGQUIT) или `pkill -INT tmh` из другого терминала.

**`reload --tmux` → `No such file or directory: ~/.tmux.conf`**
Обновись — это был баг до текущей версии (tilde не expand'ился). Теперь работает.

**`state.db` corrupted**
```sh
tmh doctor --fix-state
```
Переименует испорченный файл в `state.db.broken.<ts>` и создаст чистый. Потерянные snapshots/undo/trust ожидаются.

**Ad-hoc сессия не видится как drift**
По дизайну: сессии не объявленные в config — ignored. Добавь в config через `tmh sync --pull`.

**Hooks не запускаются**
Проверь что `tmh --no-hooks` не в алиасе. Если config был изменён — будет повторный trust-prompt.

**`go install` падает с 410 Gone / unknown revision**
Добавь `export GOPRIVATE=git.mark1708.ru/*` чтобы обойти proxy/sum.

---

## Архитектура

```
cmd/tmh/              cobra entrypoint + subcommands
internal/
  config/             парсер/резолвер/валидатор/atomic writer, diff
  tmux/               Runner interface (CLIRunner) — единственный seam к tmux
  tmux/tmuxtest/      MockRunner для тестов (не импортируется production-кодом)
  actions/            side-effect API; CLI и TUI — тонкие фронтенды
  state/              SQLite WAL + busy_timeout: events / snapshots / trust / reload_queue
  errors/             типизированные sentinels
  ui/                 bubbletea: dashboard, palette, settings, diff, confirm, help
  xdg/                XDG пути
```

Принципы:

- Все side-effects — в `internal/actions`, CLI и TUI только вызывают.
- `internal/tmux.Runner` — единственная точка контакта с `tmux`. Тесты используют `tmuxtest.MockRunner`.
- Мутации `config.yml` — через `config.PathSet/Delete/Rename` + `config.Write` с сохранением комментариев.
- Все ошибки — типизированные sentinels в `internal/errors`, wrap через `fmt.Errorf("...: %w", ...)`.

Подробности — в [CONTRIBUTING.md](CONTRIBUTING.md) и [docs/](docs/).

---

## Лицензия

MIT — см. [LICENSE](LICENSE).
