# ROADMAP

## Цель

Создать локальный stateless CLI `donext` поверх Codex App Server, который из
текущей директории последовательно выполняет roadmap проекта. Для каждого goal
создаётся новый persisted Codex thread, а следующий goal запускается только после
успешного завершения предыдущего.

## Правила ведения roadmap

1. Источником следующей задачи является раздел «Текущие шаги».
2. По умолчанию выполняется первый незавершённый шаг сверху вниз.
3. За один Codex thread выполняется ровно один шаг.
4. Шаг можно считать завершённым только после реализации, тестирования и необходимого обновления документации.
5. Завершённый шаг удаляется из «Текущих шагов» и переносится в конец раздела «История шагов».
6. Запись в истории сохраняет исходный номер, название, дату завершения и краткий результат с перечислением выполненных проверок.
7. Незавершённые, частично выполненные и заблокированные шаги остаются в «Текущих шагах». Их текущее состояние указывается вложенным пунктом `Статус`.
8. Новые обязательные шаги добавляются в логически правильное место, а не автоматически в конец списка.
9. Нумерация шага стабильна после начала реализации. Перенумеровывать существующие шаги только ради косметического порядка не нужно.
10. Нельзя переносить шаг в историю при падающих релевантных тестах или неизвестном результате проверки.
11. Перед началом шага необходимо прочитать `AGENTS.md` и этот файл полностью.
12. Если текущих шагов не осталось, агент не изменяет репозиторий и завершает ответ маркером `ORCHESTRATOR_NO_WORK`.

## Границы MVP

В MVP входят:

- `donext` из текущей директории;
- `donext --once`;
- `donext --dry-run`;
- `donext --prompt TEXT|@FILE|-`;
- `donext --weekly-usage-budget N`;
- `donext status` для текущего проекта;
- запуск одного App Server на один процесс `donext`;
- новый persisted thread для каждого goal;
- ожидание реального terminal event turn;
- остановка при `ORCHESTRATOR_NO_WORK`, failure или interruption;
- независимые project locks, state/recovery, тесты и README.

В MVP не входят Web UI, daemon/service orchestrator, scheduler, remote orchestration, собственная база задач и принудительное управление Git commits.

## Текущие шаги

## Отложенные улучшения

Эти пункты не являются частью MVP и переносятся в «Текущие шаги» только отдельным решением:

- интерактивный selector проекта;
- подключение к уже запущенному App Server daemon/socket;
- автоматическое уточнение имени thread после определения goal;
- защитный `--max-goals`;
- дополнительные transport implementations;
- packaging и release automation.

## История шагов

### ORCH-014 — Заменить абсолютный порог недельным бюджетом запуска

- Завершено: 2026-08-30.
- Результат: `--max-used-percent` заменён на `--weekly-usage-budget N`; protocol
  adapter сохраняет duration и reset metadata обоих rate-limit окон и выбирает
  единственное недельное окно по `windowDurationMins == 10080`. До первого goal
  фиксируется baseline, перед последующими считается consumed delta; достижение
  или превышение бюджета останавливает цикл до создания нового thread со статусом
  `weekly_usage_budget_reached` и полным budget summary. Отсутствующие,
  неоднозначные, сбросившиеся и аномальные данные обрабатываются fail-closed;
  активный turn не прерывается. README обновлён. Проверки: `go test -race
  -count=10 ./internal/codex ./internal/cli`; `go test ./...`; `go vet ./...`;
  `go build ./cmd/donext`; `git diff --check`.

### ORCH-001 — Проверить протокол и persistence установленного Codex

- Завершено: 2026-08-30.
- Результат: для `codex-cli 0.149.0-alpha.4.3` сгенерирована и изучена schema,
  подтверждены NDJSON framing и initialize handshake, выполнен реальный persisted
  thread/turn lifecycle с `thread/name/set` и terminal `turn/completed`, а
  `thread/list` подтвердил сохранение после restart App Server. Thread успешно
  открыт по ID в Codex Desktop; ограничения sidebar/project visibility описаны в
  `docs/app-server-spike.md`. Проверки: schema generation; реальный stdio smoke
  test; повторный `thread/list` из нового процесса; штатная GUI navigation.

### ORCH-002 — Инициализировать Go CLI и конфигурацию проектов

- Завершено: 2026-08-30.
- Результат: создан Go module и `cmd/orchestrator`, добавлена строгая
  YAML-конфигурация Codex и произвольного количества проектов, загрузка prompt из
  Markdown, валидация project key, абсолютного repository path и prompt file.
  Реализованы `projects` и `run <project> --dry-run` без запуска Codex, добавлены
  `config.example.yaml` и default prompt. Проверки: `go test ./internal/config
  ./internal/cli`; `go test ./...`; `go vet ./...`; `go build
  ./cmd/orchestrator`; ручной запуск `projects` и `run demo --dry-run` на
  временной конфигурации.

### ORCH-003 — Реализовать минимальный App Server adapter

- Завершено: 2026-08-30.
- Результат: добавлен mockable доменный интерфейс `Codex` и изолированный stdio
  adapter, запускающий `codex app-server --stdio`; реализованы NDJSON framing,
  initialize handshake, конкурентная correlation ответов, `thread/start`,
  `thread/name/set`, `turn/start`, `turn/interrupt`, маршрутизация terminal events
  и server requests, безопасное игнорирование неизвестных notifications, а также
  обработка RPC/protocol errors, EOF, stderr и process exit. Protocol-level тесты
  используют только in-memory fake transport и покрывают полный lifecycle,
  interrupt, out-of-order responses, server requests и ошибки. Проверки:
  `go test -race -count=10 ./internal/codex`; `go test ./...`; `go vet ./...`;
  `go build ./cmd/orchestrator`.

### ORCH-004 — Реализовать один полный запуск через `--once`

- Завершено: 2026-08-30.
- Результат: команда `run <project> --once` проверяет Git-состояние без
  блокировки dirty/non-Git проекта, запускает один App Server, создаёт и именует
  новый persisted thread в repository проекта, отправляет ровно один configured
  prompt и ожидает terminal `turn/completed`. Добавлена классификация
  `completed`, `failed`, `interrupted` и `no_work`, причём маркер
  `ORCHESTRATOR_NO_WORK` учитывается только из финального completed agent message;
  вывод всегда содержит project, thread ID и terminal status, а failure и
  interruption возвращают ненулевой exit status. Проверки:
  `go test -race -count=10 ./internal/codex ./internal/cli`; `go test ./...`;
  `go vet ./...`; `go build ./cmd/orchestrator`.

### ORCH-005 — Добавить continuous execution

- Завершено: 2026-08-30.
- Результат: `run <project>` без флагов теперь выполняет goals непрерывно,
  создавая новый persisted thread после каждого успешного goal и сохраняя один
  App Server process на весь цикл. Цикл останавливается на `no_work`, failure или
  interruption и не создаёт лишний thread после terminal stop condition;
  `--once` сохраняет однократный режим, а `--dry-run` остаётся отдельным режимом.
  Fake-Codex тесты покрывают последовательность `completed → completed → no-work`,
  уникальность thread и остановку после failure. Проверки: `go test -race
  -count=10 ./internal/cli`; `go test ./...`; `go vet ./...`; `go build
  ./cmd/orchestrator`.

### ORCH-006 — Реализовать state, locking и recovery

- Завершено: 2026-08-30.
- Результат: добавлен отдельный минимальный JSON state каждого проекта без
  transcript с атомарной записью через temporary file, `fsync` и rename;
  OS-level `flock` независимо ограничивает конкурентный запуск по project key,
  не блокируя другие проекты. CLI сохраняет `running` и terminal lifecycle с
  thread/turn ID, распознаёт stale `running` после получения свободного lock и
  при restart всегда создаёт новый thread. Тесты покрывают persistence через
  новый Store, атомарную замену, независимость locks, конкурентный отказ и
  recovery abandoned thread. Проверки: `go test -race -count=10
  ./internal/state ./internal/cli`; `go test ./...`; `go vet ./...`; `go build
  ./cmd/orchestrator`.

### ORCH-007 — Реализовать корректный interrupt и интерактивные запросы

- Завершено: 2026-08-30.
- Результат: первый `Ctrl+C`/`SIGTERM` запрещает продолжение continuous loop,
  вызывает `turn/interrupt` и ожидает terminal event в пределах трёхсекундного
  grace period; повторный сигнал или истечение grace period принудительно
  завершает App Server. Approval и user-input server requests получают явный RPC
  error, активный turn прерывается, а запуск завершается контролируемой ошибкой с
  thread ID и сохранённым terminal state. В project config добавлены проверяемые
  `approval_policy` и `sandbox`, соответствующие schema установленного App Server.
  Проверки: генерация и сверка JSON schema `codex app-server`; `go test -race
  -count=10 ./internal/config ./internal/codex ./internal/cli`; `go test ./...`;
  `go vet ./...`; `go build ./cmd/orchestrator`.

### ORCH-008 — Реализовать status и lifecycle logging

- Завершено: 2026-08-30.
- Результат: добавлены сводный `orchestrator status` и подробный
  `orchestrator status <project>` с repository, lock, turn и timestamp. Состояние
  `running` сопоставляется с живым project lock, поэтому abandoned persisted
  state показывается как `stale`, а не как активный процесс. Для каждого проекта
  ведётся metadata-only lifecycle log App Server, thread и turn с UTC timestamp,
  project key, идентификаторами и terminal status без prompt, reasoning,
  transcript или command output. Проверки: `go test -race -count=10
  ./internal/state ./internal/cli`; `go test ./...`; `go vet ./...`; `go build
  ./cmd/orchestrator`.

### ORCH-009 — Завершить документацию и проверку MVP

- Завершено: 2026-08-30.
- Результат: создан README с требованиями, установкой, конфигурацией, quick start,
  всеми MVP-командами и условиями остановки; описаны state directory, атомарный
  state, независимые locks, recovery, dirty/non-Git repository behavior, Codex
  sessions и ограничения GUI visibility. Добавлена явная opt-in процедура smoke
  test с предупреждением о расходовании лимитов. Definition of Done сверена:
  автоматические тесты используют fake/in-memory App Server, документация
  соответствует реализованному CLI, ручной запуск с настоящим App Server в
  изолированном временном Git repository завершился статусом `completed` и
  подтвердился через `status`. Проверки: `go test ./...`; `go vet ./...`;
  `go build ./cmd/orchestrator`; реальный `run smoke --once` с persisted thread
  `01a0519f-d009-7943-8ba8-010bcd9dd655`; `status smoke`.

### ORCH-010 — Ограничить continuous run по расходу контекста

- Завершено: 2026-08-30.
- Результат: schema установленного App Server подтвердила доступность как
  `thread/tokenUsage/updated` с token usage и context window, так и более
  подходящего для continuous run запроса `account/rateLimits/read` с primary и
  secondary `usedPercent`. Добавлен флаг `--max-used-percent N`: перед каждым
  goal он проверяет наибольший расход аккаунтной квоты, штатно останавливается со
  статусом `limit_reached` до создания нового thread и fail-closed завершает
  запуск при недоступности данных. Текущий turn не прерывается. Protocol adapter,
  fake CLI tests и README обновлены. Проверки: `go test -race -count=10
  ./internal/codex ./internal/cli`; `go test ./...`; `go vet ./...`; `go build
  ./cmd/orchestrator`; `git diff --check`.

### ORCH-011 — Перевести идентификацию и runtime metadata на текущий проект

- Завершено: 2026-08-30.
- Результат: добавлена единая идентификация проекта по canonical Git root либо
  canonical non-Git directory с разрешением symlink; basename используется для
  отображения, а устойчивый hash абсолютного пути — как внутренний project ID.
  State, lifecycle logs и locks независимо хранятся в
  `~/.donext/{state,logs,locks}` по project ID; прежний flat/XDG state безопасно
  игнорируется без неявной миграции. `status` переведён на проект текущей
  директории и показывает canonical repository, project ID, lock и recovery
  state. README и тесты обновлены для Git root, non-Git, symlink, одинаковых
  basename, layout, locking и recovery. Проверки: `go test -race -count=10
  ./internal/project ./internal/state ./internal/cli`; `go test ./...`; `go vet
  ./...`; `go build ./cmd/orchestrator`; `git diff --check`.

### ORCH-011A — Хранить runtime metadata внутри проекта

- Завершено: 2026-08-30.
- Результат: state, lifecycle logs и locks перенесены в
  `<canonical-project-root>/.donext/{state,logs,locks}`. Запуск из вложенной
  директории или через symlink использует `.donext` canonical Git/non-Git root;
  прежний `~/.donext` не читается и не мигрирует автоматически. Служебная папка
  исключена из проверки dirty Git state и добавлена в `.gitignore` этого
  репозитория. README и тесты обновлены. Проверки: `go test -race -count=10
  ./internal/state ./internal/project ./internal/cli`; `go test ./...`; `go vet
  ./...`; `go build ./cmd/orchestrator`; `git diff --check`.

### ORCH-012 — Сделать CLI `donext` stateless и удалить пользовательский config

- Завершено: 2026-08-30.
- Результат: бинарник и usage переименованы в `donext`, основной запуск переведён
  на текущую директорию без `run <project>`, а `status` оставлен единственной
  подкомандой. YAML config, список projects, старый binary entrypoint и config
  examples удалены; App Server всегда запускается командой `codex` из `PATH`.
  Добавлены и проверены flags `--once`, `--dry-run`, `--approval-policy` и
  `--sandbox` с defaults `never` и `workspace-write`; dry-run показывает
  конкретную команду и effective параметры. Fake CLI tests и README обновлены.
  Проверки: `go test -race -count=10 ./internal/cli`; `go test ./...`; `go vet
  ./...`; `go build ./cmd/donext`; `git diff --check`; ручные help, status,
  dry-run и invalid-policy проверки без запуска App Server.

### ORCH-013 — Объединить способы задания prompt в `--prompt`

- Завершено: 2026-08-30.
- Результат: добавлен единый `--prompt VALUE` с однозначными источниками: literal
  text, обязательный файл через `@PATH` и перенаправленный stdin через `-`; наличие
  одноимённого файла не меняет literal text. Без flag используется встроенный
  roadmap prompt с правилом одного шага и `ORCHESTRATOR_NO_WORK`. Пустой prompt,
  отсутствующий или нечитаемый файл и terminal stdin диагностируются до запуска
  App Server; пользовательский prompt не сохраняется в state и lifecycle logs.
  README и CLI usage обновлены. Проверки: `go test -race -count=10
  ./internal/cli`; `go test ./...`; `go vet ./...`; `go build ./cmd/donext`;
  `git diff --check`.

### ORCH-015 — Завершить документацию и end-to-end проверку нового CLI

- Завершено: 2026-08-30.
- Результат: README полностью переведён на сценарий `cd project && donext` и
  описывает stateless-модель, автоматическое применение `AGENTS.md`, единый
  `--prompt`, project-local `.donext`, `status`, условия остановки и недельный
  бюджет. Устаревшие config examples отсутствуют, реальный App Server smoke test
  оставлен явно opt-in. Ручная проверка без запуска Codex выявила и исправила
  ненулевой exit code для `--help`; добавлен regression test. Вручную проверены
  help, idle status, dry-run, удалённый `--config`, конфликт `--once`/`--dry-run`
  и неверный budget. Проверки: `go test -race -count=10 ./internal/cli`; `go test
  ./...`; `go vet ./...`; `go build ./cmd/donext`; `git diff --check`; ручная
  CLI-матрица на временном non-Git проекте.

### ORCH-016 — Привязывать thread к проекту Codex Desktop

- Завершено: 2026-08-30.
- Результат: App Server adapter пагинированно читает `project/list`;
  `donext` один раз за запуск сопоставляет canonical project root с roots
  существующих проектов Desktop с разрешением symlink и передаёт
  найденный `projectId` в каждый `thread/start`. Отсутствие совпадения
  оставляет thread непривязанным с предупреждением; protocol error и
  неоднозначные roots останавливаются до создания thread. README и
  protocol spike обновлены. Проверки: `go test -race -count=10
  ./internal/codex ./internal/cli`; `go test ./...`; `go vet ./...`; `go build
  ./cmd/donext`; `git diff --check`.
