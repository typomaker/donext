# Codex App Server protocol spike

Дата проверки: 2026-08-30.

## Окружение

- Codex CLI: `codex-cli 0.149.0-alpha.4.3` из Codex Desktop.
- App Server запускался командой `codex app-server --stdio`.
- Схема с experimental API была сгенерирована командой
  `codex app-server generate-json-schema --experimental --out <temporary-directory>`.
- Полный сгенерированный bundle намеренно не добавлен в репозиторий: он привязан к
  установленной версии Codex и должен генерироваться заново при изменении версии.
- Рабочая директория `donext` во время spike не являлась Git-репозиторием. Для
  lifecycle использовался отдельный временный абсолютный `cwd`.

## Framing и handshake

Stdio transport использует newline-delimited JSON: одно JSON-сообщение на строку,
без заголовка `Content-Length`. Запрос содержит `id`, `method` и `params`; ответ —
тот же `id` и `result` либо `error`. Notifications не содержат `id`. Поле
`jsonrpc` в фактически отправленных и полученных сообщениях отсутствовало.

Минимальный handshake:

1. Клиент отправляет `initialize` с обязательным `clientInfo.name` и
   `clientInfo.version`. `capabilities` допускается опустить или передать `null`.
2. Сервер отвечает `userAgent`, `codexHome`, `platformFamily` и `platformOs`.
3. Клиент отправляет notification `initialized` с пустыми `params`.
4. После handshake можно отправлять остальные запросы.

Для orchestrator transport должен параллельно читать stdout и stderr. stdout
зарезервирован для protocol messages, тогда как диагностические JSON-логи и
warnings приходят в stderr.

## Подтверждённый lifecycle

В реальном запуске выполнена следующая последовательность:

1. `thread/start` с абсолютным `cwd`, `ephemeral: false`,
   `approvalPolicy: "never"` и `sandbox: "read-only"`.
2. Из `result.thread.id` получен persisted thread
   `01a0517e-9783-7b60-9c71-af438e42d411`. Ответ подтвердил переданный `cwd` и
   `ephemeral: false`.
3. `thread/name/set` с `threadId` и именем
   `donext ORCH-001 protocol spike` вернул пустой успешный result; сервер также
   прислал `thread/name/updated`.
4. `turn/start` получил обязательные `threadId` и массив `input` с text item.
   Сервер вернул turn ID `01a0517e-9c30-7a52-a7a3-1c8287e024da`.
5. После промежуточных notifications (`turn/started`, `item/started`, delta,
   `item/completed`, token usage и status updates) сервер прислал terminal
   notification `turn/completed`. Его `params.threadId` соответствует thread, а
   `params.turn.status` равен `completed`; финальный agent message был
   `SPIKE_OK`.
6. `thread/list` с фильтром по точному `cwd` вернул thread с заданным именем,
   preview, `ephemeral: false` и статусом `idle`.
7. stdin первого процесса был закрыт, процесс штатно завершился. После запуска
   нового App Server, нового handshake и повторного `thread/list` тот же ID,
   имя, `cwd`, preview и session path сохранились; runtime status стал
   `notLoaded`.

Для определения конца turn следует использовать только `turn/completed` и поле
`params.turn.status`. Схема текущей версии допускает значения `completed`,
`interrupted`, `failed` и `inProgress`; для terminal notification ожидаются
первые три. Timeout годится только как operational safeguard, но не как признак
успешного завершения.

## Обязательные запросы и существенные поля

- `thread/start`: все поля формально optional, но orchestrator должен явно
  передавать абсолютный `cwd` и `ephemeral: false`; policy-поля следует задавать
  из конфигурации, а не наследовать неявно.
- `thread/name/set`: обязательны `threadId` и `name`.
- `thread/list`: поля optional; ответ пагинирован (`data`, `nextCursor`). Фильтр
  `cwd` делает проверку persistence однозначной. Если `sourceKinds` не указан
  или пуст, сервер применяет default-набор interactive sources.
- `turn/start`: обязательны `threadId` и `input`. Text input имеет вид
  `{ "type": "text", "text": "..." }`.
- `turn/completed`: обязательны `threadId` и `turn`; status и error находятся
  внутри `turn`.

App Server может присылать notifications, не относящиеся напрямую к lifecycle
orchestrator (например, MCP startup, rate limits, token usage и remote-control
status). Адаптер должен маршрутизировать известные события и безопасно
игнорировать либо кратко логировать неизвестные. Также сервер может инициировать
JSON-RPC requests для approval или user input; их нельзя ошибочно принять за
notifications или terminal events.

## Persistence и Codex Desktop

Сервер сообщил стандартный session path внутри
`~/.codex/sessions/2026/08/30/`. Файл и внутренние session/index данные вручную
не читались и не изменялись: persistence проверена только публичным
`thread/list` после полного restart App Server.

Codex Desktop успешно открыл persisted thread по его ID через штатную навигацию
приложения (`navigated: true`). Следовательно, thread, созданный App Server этой
версии, видим/открываем в GUI при известном ID. Spike не подтверждает, что такой
thread всегда автоматически появится в sidebar или в конкретном project/section:
`thread/start` был выполнен без `projectId`, а источник в `thread/list`
отображался как `vscode`. Orchestrator должен хранить thread ID в lifecycle
metadata и считать прямое открытие по ID надёжным способом перехода; sidebar
visibility остаётся поведением GUI, а не частью протокольной гарантии.

Дополнительная сверка schema той же установленной версии при реализации
ORCH-016 подтвердила project API: `project/list` возвращает persisted проекты с
`id`, `name` и массивом абсолютных `roots`, а `thread/start.projectId` задаёт
optional project identity. Описание schema гарантирует, что durable thread
сохраняет эту assignment. Поэтому явное сопоставление canonical root с
`projectId` является протокольным способом группировки; одного `cwd` для этого
недостаточно.

## Вывод для реализации

Protocol-specific JSON и методы следует изолировать в одном adapter. Adapter
должен выполнять handshake, коррелировать responses по `id`, читать поток
непрерывно, маршрутизировать events по thread/turn ID, распознавать terminal
status только из `turn/completed`, отдельно обрабатывать server requests и не
зависеть от полного набора notification methods конкретной версии.
