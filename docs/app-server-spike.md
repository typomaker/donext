# Codex App Server protocol spike

Verified on 2026-08-30.

## Environment

- Codex CLI: `codex-cli 0.149.0-alpha.4.3` bundled with Codex Desktop.
- App Server command: `codex app-server --stdio`.
- Experimental schemas were generated with
  `codex app-server generate-json-schema --experimental --out <temporary-directory>`.
- The generated bundle is deliberately not committed because it is tied to the
  installed Codex version and must be regenerated after upgrades.
- A separate absolute temporary `cwd` was used for the lifecycle test.

## Framing and handshake

The stdio transport uses newline-delimited JSON, one message per line, without a
`Content-Length` header. Requests contain `id`, `method`, and `params`; responses
contain the same `id` and either `result` or `error`. Notifications have no `id`.
Observed messages did not contain a `jsonrpc` field.

Minimal handshake:

1. Send `initialize` with required `clientInfo.name` and `clientInfo.version`.
   `capabilities` may be omitted or set to `null`.
2. Receive `userAgent`, `codexHome`, `platformFamily`, and `platformOs`.
3. Send an `initialized` notification with empty `params`.
4. Send normal requests after initialization completes.

The transport must read stdout and stderr concurrently. Stdout is reserved for
protocol messages; diagnostic JSON logs and warnings arrive on stderr.

## Confirmed lifecycle

The real smoke test performed this sequence:

1. `thread/start` with an absolute `cwd`, `ephemeral: false`,
   `approvalPolicy: "never"`, and `sandbox: "read-only"`.
2. `result.thread.id` returned persisted thread
   `01a0517e-9783-7b60-9c71-af438e42d411` and confirmed `cwd` and persistence.
3. `thread/name/set` named it `donext ORCH-001 protocol spike`; the server also
   emitted `thread/name/updated`.
4. `turn/start` received `threadId` and a text input array, returning turn
   `01a0517e-9c30-7a52-a7a3-1c8287e024da`.
5. After intermediate notifications, the server emitted terminal notification
   `turn/completed`. Its thread ID matched, status was `completed`, and the final
   agent message was `SPIKE_OK`.
6. `thread/list`, filtered by the exact `cwd`, returned the named persistent
   thread with preview and `idle` status.
7. After closing stdin, restarting App Server, repeating the handshake, and
   calling `thread/list` again, the same ID, name, cwd, preview, and session path
   remained available with runtime status `notLoaded`.

Only `turn/completed` and `params.turn.status` should determine turn completion.
The verified schema includes `completed`, `interrupted`, `failed`, and
`inProgress`; terminal notifications are expected to use the first three. A
timeout is only an operational safeguard, never evidence of success.

## Important requests and fields

- `thread/start`: fields are formally optional, but the orchestrator explicitly
  sends absolute `cwd`, `ephemeral: false`, and the selected policies.
- `thread/name/set`: requires `threadId` and `name`.
- `thread/list`: optional, paginated parameters and result (`data`, `nextCursor`).
  An exact `cwd` filter makes persistence checks deterministic.
- `turn/start`: requires `threadId` and `input`. Text input is represented as
  `{ "type": "text", "text": "..." }`.
- `turn/completed`: requires `threadId` and `turn`; status and error are nested
  inside `turn`.

App Server may emit notifications unrelated to the orchestrator lifecycle, such
as MCP startup, rate limits, token usage, and remote-control status. The adapter
must route known events and safely ignore or briefly log unknown ones. The server
may also initiate JSON-RPC approval or user-input requests; these must not be
mistaken for notifications or terminal events.

## Persistence and Codex Desktop

App Server reported the standard session path under
`~/.codex/sessions/2026/08/30/`. Session files and internal indexes were not read
or modified; persistence was verified only through public protocol calls after a
complete App Server restart.

Codex Desktop successfully opened the persisted thread by ID. The initial spike
did not prove automatic sidebar or project placement because `thread/start` did
not include `projectId`, and `thread/list` reported the source as `vscode`.
Direct navigation by stored thread ID therefore remains a reliable fallback.

Additional schema verification for ORCH-016 confirmed the project API:
`project/list` returns persisted projects with `id`, `name`, and absolute
`roots`; optional `thread/start.projectId` assigns project identity, and durable
threads persist that assignment. Explicit canonical-root matching is therefore
the protocol mechanism for Desktop grouping; `cwd` alone is insufficient.

## Implementation conclusion

Protocol-specific JSON and methods belong in one adapter. The adapter must
perform the handshake, correlate responses by ID, read continuously, route
events by thread and turn IDs, recognize terminal status only through
`turn/completed`, handle server requests separately, and remain compatible with
unknown notifications.
