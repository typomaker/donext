# donext

`donext` — локальный stateless CLI поверх установленного Codex App Server. Он
последовательно выполняет roadmap проекта из текущей директории: для каждого
шага создаёт новый persisted Codex thread и ждёт его реального terminal event.
Пользовательский config, реестр проектов и собственная копия истории не нужны.

## Требования и установка

- Go 1.25+;
- `codex` в `PATH` с действующей авторизацией;
- проект с canonical `ROADMAP.md` и правилами выбора следующего шага;
- опциональный `AGENTS.md` с инструкциями проекта.

Соберите бинарник локально или установите его в `GOBIN`:

```sh
go build -o donext ./cmd/donext
# или
go install github.com/albertsultanov/donext/cmd/donext@latest
```

Основной сценарий всегда начинается в нужном проекте:

```sh
cd /path/to/project
donext
```

`donext` не читает и не парсит roadmap сам. Он запускает Codex с canonical root
текущего Git-репозитория (для non-Git директории — с canonical текущей
директорией) и встроенным prompt, который поручает Codex выполнить ровно первый
незавершённый шаг. Codex автоматически обнаруживает применимые `AGENTS.md` по
обычным правилам; CLI не копирует их содержимое в prompt и не требует отдельной
настройки.

## Команды и режимы

```text
donext [--once|--dry-run] [--prompt TEXT|@FILE|-]
       [--approval-policy POLICY] [--sandbox MODE]
       [--weekly-usage-budget N]
donext status
```

Примеры:

```sh
donext                         # выполнять шаги до условия остановки
donext --once                  # выполнить не более одного шага
donext --dry-run               # показать запуск, не запуская Codex
donext --prompt 'Выполни шаг ORCH-123'
donext --prompt @prompt.md
printf 'Выполни шаг ORCH-123\n' | donext --prompt -
donext --approval-policy never --sandbox workspace-write
donext --weekly-usage-budget 5
donext status
```

Без `--once` после каждого успешно завершённого goal создаётся новый persisted
thread. Цикл останавливается при `ORCHESTRATOR_NO_WORK` в отдельной строке
финального ответа, failure, interruption, интерактивном запросе или достижении
недельного бюджета. Failure и interruption возвращают ненулевой код; штатные
`completed`, `no_work` и `weekly_usage_budget_reached` — нулевой.

`--dry-run` показывает canonical project path, точную команду
`codex app-server --stdio`, effective approval policy, sandbox и prompt. Он не
создаёт `.donext`, не запускает App Server и не читает account rate limits.
`--once` и `--dry-run` взаимоисключающие.

### Prompt

`--prompt VALUE` однозначно задаёт источник:

- обычное значение — literal text, даже если существует одноимённый файл;
- `@PATH` — содержимое обязательного читаемого файла;
- `-` — перенаправленный stdin.

Пустой prompt, отсутствующий или нечитаемый `@FILE` и интерактивный terminal
stdin отклоняются до запуска App Server. Без флага используется встроенный
roadmap prompt с правилом одного шага и маркером `ORCHESTRATOR_NO_WORK`. Prompt
не сохраняется в `.donext` и не попадает в lifecycle logs.

### Policies и интерактивные запросы

В каждый `thread/start` явно передаются безопасные defaults:

- `--approval-policy never` (также: `on-request`, `untrusted`);
- `--sandbox workspace-write` (также: `read-only`, `danger-full-access`).

App Server всегда запускается как `codex app-server --stdio` из `PATH`.
Approval и user-input requests отклоняются, активный turn прерывается, а CLI
завершается с ошибкой: unattended orchestration не отвечает на них за
пользователя.

### Codex Desktop project

После запуска App Server `donext` читает `project/list` и ищет единственный
проект Desktop, один из roots которого совпадает с canonical root текущего
проекта (включая разрешение symlink). Его `projectId` передаётся в каждый
`thread/start`, поэтому persisted thread отображается под соответствующим
проектом Codex Desktop, а не только в Recents.

`donext` не создаёт и не изменяет проекты Desktop. Если совпадения нет, запуск
продолжается с предупреждением и thread остаётся непривязанным. Ошибка
`project/list` или несколько Desktop projects с одним root останавливают запуск
до создания thread, чтобы не получить неверную привязку.

### Недельный бюджет

`--weekly-usage-budget N`, где `N` от 1 до 100, разрешает текущему процессу
потратить не более `N` процентных пунктов недельной квоты. Перед первым goal CLI
фиксирует baseline единственного окна длительностью 10080 минут, а перед каждым
следующим сравнивает текущий расход с baseline. При достижении бюджета новый
thread не создаётся; процесс штатно завершается со статусом
`weekly_usage_budget_reached` и печатает baseline, текущий расход, delta и
budget.

Активный turn не прерывается, поэтому завершившийся goal может превысить бюджет.
Если недельное окно отсутствует, неоднозначно, сбросилось или содержит аномальные
значения, запуск останавливается с ошибкой (fail-closed). В режиме `--once`
baseline проверяется, но повторной проверки после единственного goal нет.

## Project state, locking и recovery

Runtime metadata находится только внутри canonical project root:

```text
.donext/
  state/
  logs/
  locks/
```

Это и означает stateless на уровне CLI: между запусками нет глобального списка
проектов или пользовательского config; нужный проект определяется текущей
директорией. Минимальный project-local state нужен только для lifecycle,
locking и recovery. State записывается атомарно, а lifecycle logs не содержат
prompt, transcript или command output.

Project lock не допускает два одновременных запуска одного проекта, но не мешает
параллельным запускам в разных проектах. После аварийно прерванного процесса
следующий запуск сообщает о stale state и создаёт новый thread. Dirty или
non-Git проект не блокируется; CLI только выводит предупреждение, а `.donext`
исключается из собственной проверки dirty state.

`donext status` ничего не запускает и показывает для текущего проекта canonical
repository, устойчивый project ID, effective status, lock, последние thread/turn
ID и время обновления. Persisted `running` без живого lock отображается как
`stale`; при отсутствии state отображается `idle`.

## Проверка и opt-in smoke test

Автоматические проверки не запускают реальный Codex и не расходуют
пользовательские лимиты:

```sh
go test ./...
go vet ./...
go build ./cmd/donext
```

Реальный App Server smoke test не входит в обычный test suite. Запускайте его
только отдельно и осознанно: он создаст persisted Codex thread и потратит квоту.

```sh
cd /path/to/disposable-smoke-project
/absolute/path/to/donext --once --approval-policy never --sandbox read-only
```
