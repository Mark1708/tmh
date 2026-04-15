# tmh

Single-binary TUI-хаб для tmux. Декларативные YAML-сессии, sync между live и config, перезагрузка dotfiles, конфиг который не стыдно положить в git и поделиться с коллегой.

Собран потому что zsh-функции вокруг `tmux` плохо масштабируются: один INI-файл, 9 алиасов, нет диффа, нет undo, нет sharing.

---

## Содержание

- [Установка](#установка)
- [Первый запуск](#первый-запуск)
- [Быстрый старт](#быстрый-старт)
- [Язык интерфейса](#язык-интерфейса)
- [Конфиг `config.yml`](#конфиг-configyml)
- [CLI-справочник](#cli-справочник)
- [TUI-дашборд](#tui-дашборд)
- [tmux-интеграция](#tmux-интеграция)
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

`doctor` проверяет:

- tmux ≥ 3.2, `$SHELL`, `config.yml` (существование + схема),
- наличие tmux-сервера, опциональных `fd`, `terminal-notifier`, значение `GOPRIVATE`,
- отдельный блок **tmux integration** — аудит опций сервера (`default-terminal`, `mouse`, `escape-time`, `extended-keys`, `base-index`, `pane-base-index`, `renumber-windows`), конфликтующих hook'ов (`after-new-window`, `automatic-rename=on`) и наличия `#(tmh status)` в `status-right`. Рядом с каждым ⚠/✗ печатается готовая строка для `~/.tmux.conf`.

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

## Язык интерфейса

Поддерживаются `en` (по умолчанию) и `ru`. Неподдерживаемые локали (например `de_DE`) молча откатываются на английский — сырые i18n-ключи пользователю не показываются.

Приоритет разрешения (от высшего к низшему):

1. `--lang en|ru` — глобальный флаг, перекрывает всё. Влияет на runtime-сообщения (toasts, ошибки, print-вывод). Текст cobra-help привязывается к языку, выбранному на старте, и `--lang` его не перерисовывает — это ограничение cobra.
2. `defaults.lang: ru` в `config.yml`.
3. Переменные окружения `TMH_LANG`, `LC_ALL`, `LC_MESSAGES`, `LANG` (парсится префикс до `_`/`.`).
4. Fallback — `en`.

Живое переключение из TUI: `S` (settings) → секция **язык** → `↑↓`. Выбор применяется мгновенно и персистится как `defaults.lang` в `~/.config/tmh/config.yml`.

JSON-выводы (`tmh ls --json`, `tmh diff --json`, `tmh tmux audit --json`) остаются на английском независимо от языка — это стабильный контракт для скриптов. У `Drift` есть отдельное стабильное поле `ReasonCode` (например `session_gone`), которое TUI резолвит в локализованный текст при отображении.

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
  lang:   ru                         # en | ru; пусто — авто-детект из env
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

Опциональный shorthand: строка начинается с `$key/...` — раскрывается в `{ root: key, path: ... }`. `$$` эскейп для литерального `$`. Нормализация shorthand в канонический вид пока выполняется в памяти при загрузке (`config.Normalize`); CLI-обёртки для записи нормализованного вида на диск нет.

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

`tmh doctor` валидирует конфиг и печатает `config.yml schema: <err>`, если что-то не так. Проверяется:

- все `root:` ссылаются на существующий `roots.<ключ>` (`ErrUnknownRoot`),
- все `extends:` на `templates.<ключ>` (`ErrUnknownTemplate`),
- глубина `extends` ровно 1 (`ErrTemplateChain`),
- все `layout:` — builtin или `layouts.<ключ>` (`ErrUnknownLayout`),
- `panes[]` совместимы с builtin layout (`ErrLayoutMismatch`).

---

## CLI-справочник

Глобальные флаги любого подкоманды:

```
--config string      путь к config.yml (перекрывает TMH_CONFIG и значения по умолчанию)
--profile string     имя профиля из config.yml
--lang en|ru         язык интерфейса; приоритет над config и env
```

```
tmh                       открывает TUI-дашборд
tmh version               напечатать версию
tmh doctor                проверка окружения + tmux-интеграция
tmh completion {zsh|bash|fish}   скрипт автодополнения
```

### Сессии

```
tmh attach [имя|имя:окно]    attach (вне tmux) или switch-client (внутри)
tmh new [--name] [--dir] [--layout] [--group] [--save] [--attach]
                              без флагов — интерактивный wizard (huh-форма)
tmh init [--only a,b]        поднять всё из config (недостающее)
tmh kill <pattern>           убить сессии по substring
tmh ls [--json]              дерево сессий/окон
tmh window [--dir]           новое ad-hoc окно в текущей сессии
tmh scratch [--dir]          эфемерная сессия
```

### Sync и diff

```
tmh sync --push              live ← config (создать недостающие)
tmh sync --pull [--all]      config ← live (добавить новые, обновить drift)
tmh sync --bootstrap         импорт всех live-сессий в пустой config
tmh sync --dry-run           показать действия без применения
tmh diff [--json]            печать списка drift
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
tmh reload --status            показать deferred queue
tmh reload --rc <path>         переопределить путь к zsh rc
tmh reload --tmux-conf <path>  переопределить путь к tmux conf
tmh watch [--auto]             fsnotify-вотчер на dotfiles
tmh status                     сегмент для tmux status-right: drift/reload/zshrc badges
```

### Snapshots / undo / export / import

```
tmh snapshot save <имя>       снимок живого состояния
tmh snapshot list
tmh snapshot restore <имя>
tmh snapshot delete <имя>
tmh undo                      откатить последнее destructive-действие
tmh export [--minimal] [--only <name>]   YAML на stdout; --minimal чистит секреты
tmh import <путь> --merge|--replace
```

### Layouts, popup и tmux-интеграция

```
tmh layout save <имя> [--description]   сохранить текущую раскладку окна
tmh popup <cmd> [--width] [--height] [--no-env] [--no-cwd] [--session] [--window]
                                        команда в tmux-popup с env/cwd из config
tmh tmux audit [--json]                 печать findings аудита tmux-сервера
tmh tmux setup [--append]               сниппет для ~/.tmux.conf; --append дописывает
```

---

## TUI-дашборд

```
tmh                    без аргументов — дашборд
```

### Раскладка

```
┌─ tmh · ~/.config/tmh/config.yml ──── ⚠ drift:2 ──────────────────┐
│  SESSIONS                   │  DETAIL                             │
│  ▼ ● epcp   7w   ok         │  session: epcp                      │
│    ├─ ● lk   3p   ok        │  live     ✓                         │
│    ├─   mdr  3p   ok        │  attached ✓                         │
│    ├─ ! jr   3p   drift     │  windows  7                         │
│    └─ …                     │  status   ok                        │
│  ▼ ● kb     8w   kb         │                                     │
│                             │  preview                            │
│                             │  $ mise use                         │
│                             │  $ git status                       │
├──────────────────────────────────────────────────────────────────┤
│ a · n · d · R · s · S · : · ^L · ? · q          [ OK reload done ]│
└──────────────────────────────────────────────────────────────────┘
```

Фичи раскладки:

- Булевые поля detail (`live`, `attached`) отображаются как ✓/✗.
- Под detail-полями — асинхронный **preview** (`tmux capture-pane` первой панели фокусной сессии/окна). Обновляется при смене курсора, кэш keyed по target'у.
- **Inline toast** встраивается в правую часть футера и держится 4–5 с (ошибки — 5 с, action-done — 4 с). Все toast-и также уходят в ring-буфер (30 последних записей), доступный через `Ctrl+L`.

### Keymap

**Навигация**
- `j` / `k` / `↑↓` — вверх / вниз
- `h` / `l` — свернуть / развернуть сессию
- `g` / `G` — к началу / в конец
- `PgUp` / `PgDn` — постранично

**Действия с сессиями**
- `enter` / `a` — attach (tmux перехватывает терминал, возврат через `prefix d`)
- `n` — новая сессия через мастер (huh-форма, запускается как подпроцесс)
- `d` — kill выбранной сессии (с подтверждением)
- `u` — undo последнего kill (recreate из snapshot)

**Sync / reload**
- `r` — обновить дерево TUI из tmux (без побочных эффектов)
- `R` — `source ~/.zshrc` + `tmux source-file ~/.tmux.conf`
- `s` — `sync --push` (создать недостающие окна)
- `D` — экран drift

**Прочее**
- `:` / `Ctrl+P` — палитра команд (fuzzy-поиск)
- `S` — настройки: язык / тема / tmux integration (см. ниже)
- `Ctrl+L` — экран истории действий с OK/ERR бейджами
- `Ctrl+T` — cycle темы без захода в settings
- `?` / `esc` — help on / off
- `q` / `Ctrl+C` — выход

### Settings screen

Три секции, переключение между ними — `Tab` / `Shift+Tab`, навигация внутри секции — `↑↓`:

1. **язык** — live-swap `en` / `ru`. Выбор мгновенно перестраивает UIStrings и сохраняется как `defaults.lang` в `~/.config/tmh/config.yml`.
2. **тема** — 4 Catppuccin flavour (mocha, macchiato, frappe, latte) с preview-свотчем.
3. **tmux integration** — read-only аудит-список (✓/⚠/✗) с результатами `tmh tmux audit`.

### Command palette

`:` или `Ctrl+P`. Fuzzy-поиск по действиям + скролл-viewport когда список длиннее высоты модалки. Встроенные entries: data refresh, dotfile reload, sync --push, init, diff, snapshot save, undo, settings, tmux audit, doctor, history, theme cycle, quit, и по одной `attach <session>` для каждой live-сессии.

---

## tmux-интеграция

Чтобы `tmh` нормально отдавал UX (truecolor, быстрый esc, extended-keys, inline status-сегмент), tmux-серверу нужен минимальный набор опций. Проверить текущее состояние и получить готовый сниппет:

```sh
tmh tmux audit          # таблица ✓/⚠/✗ по каждой опции + hint что поправить
tmh tmux audit --json   # то же в JSON для скриптов
tmh tmux setup          # сниппет для ~/.tmux.conf (печать в stdout)
tmh tmux setup --append # дописать managed-блок в ~/.tmux.conf, повторный запуск — no-op
```

Аудит покрывает:

- **baseline** (нужно для работы): `default-terminal tmux-256color` + RGB, `mouse on`, `escape-time 0`, `extended-keys on`;
- **recommended** (UX-никости): `base-index 1`, `pane-base-index 1`, `renumber-windows on`;
- **conflicts**: hook `after-new-window` (гонится с созданием окон из `tmh`), `automatic-rename=on` (перетирает имена окон);
- **integration**: сегмент `#(tmh status)` в `status-right` — без него badge drift/reload не видны в статус-баре.

Рекомендуемый bind для `~/.tmux.conf`:

```tmux
bind R run-shell "tmh reload --all"          # prefix R → dotfiles reload
set -ag status-right ' #(tmh status)'        # drift/reload badges в статус-баре
```

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

Для программного обхода trust-prompt'а (CI, аудит) внутренний пакет `actions.HookOptions.NoHooks=true` пропускает выполнение hooks — сейчас задаётся только через code-path, CLI-флаг не пробрасывается.

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

**`state.db` corrupted**
Пакет `internal/state` экспортирует `FixState(path)`, который переименует испорченный файл в `state.db.broken.<ts>` и создаст чистый. CLI-обёртки пока нет — снести вручную:

```sh
mv ~/.local/state/tmh/state.db ~/.local/state/tmh/state.db.broken.$(date +%s)
```

Потерянные snapshots / undo / trust ожидаются.

**Ad-hoc сессия не видится как drift**
По дизайну: сессии не объявленные в config — ignored. Добавь в config через `tmh sync --pull`.

**Hooks не запускаются**
Если `config.yml` был изменён — будет повторный trust-prompt. Проверь `~/.local/state/tmh/state.db` (таблица `trust`) или просто ответь `y` заново.

**`go install` падает с 410 Gone / unknown revision**
Добавь `export GOPRIVATE=git.mark1708.ru/*` чтобы обойти proxy/sum.

**Drift/reload badge не виден в tmux status-right**
Запусти `tmh tmux audit` — вероятно, отсутствует сегмент `#(tmh status)`. Исправить через `tmh tmux setup --append` либо вручную добавить в `~/.tmux.conf`.

---

## Архитектура

```
cmd/tmh/              cobra entrypoint + subcommands
internal/
  config/             парсер/резолвер/валидатор/atomic writer, diff (+ReasonCode)
  tmux/               Runner interface (CLIRunner) — единственный seam к tmux
  tmux/tmuxtest/      MockRunner для тестов (не импортируется production-кодом)
  actions/            side-effect API; CLI и TUI — тонкие фронтенды
                      (включает AuditTmuxConfig, Setup, snapshots, hooks)
  state/              SQLite WAL + busy_timeout: events / snapshots / trust / reload_queue
  errors/             типизированные sentinels (en-only, стабильный API)
  i18n/               go-i18n v2, embed locales/{en,ru}.json, DetectLang
  ui/                 bubbletea: dashboard, palette, settings, diff, confirm,
                      help, history, errrender (локализация sentinels)
  xdg/                XDG пути
```

Принципы:

- Все side-effects — в `internal/actions`, CLI и TUI только вызывают.
- `internal/tmux.Runner` — единственная точка контакта с `tmux`. Тесты используют `tmuxtest.MockRunner`.
- Мутации `config.yml` — через `config.PathSet/Delete/Rename` + `config.Write` с сохранением комментариев.
- Все ошибки — типизированные sentinels в `internal/errors` (**остаются английскими**, стабильный API для `errors.Is` и тестов). Локализация — только на границе UI через `internal/ui/errrender`.
- JSON-выводы не локализуются: `Drift.Reason` (en) + `Drift.ReasonCode` (стабильный ключ) — TUI резолвит код в локализованный текст через `i18n.T("drift.reason." + code)`.

Подробности — в [CONTRIBUTING.md](CONTRIBUTING.md) и [docs/](docs/).

---

## Лицензия

MIT — см. [LICENSE](LICENSE).
