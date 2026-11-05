# enkente Ingestion & Chat-Log API

The `enkente serve` command exposes a small REST surface for pushing chat
messages into the datastore and reading them back. This covers the
document-centric half of requirements §2 (chat-log storage, session
namespacing) and the ingest/query half of §3.2. Concept and graph endpoints
are not yet implemented.

## Running

```sh
enkente serve --port 8080 --db enkente.db
```

`--db` names the embedded BoltDB file. Pass `--db ""` to run without
persistence; `/ingest` then still fans messages out to the worker swarm and
the TUI, but nothing is stored and the read endpoints answer `503`.

`enkente tail --log logs.json --db logs.db` persists messages from an
Antigravity log file into a datastore, keyed by the log's own `sessionId`
and `messageId`, so re-tailing a log is idempotent.

BoltDB permits one process per file. A second `serve` or `tail` pointed at a
datastore that is already open fails within two seconds with a "locked by
another process" error rather than hanging. Use separate `--db` files for
concurrent processes.

## Message document

Every stored message is an Antigravity-shaped JSON document:

```json
{
  "sessionId": "live",
  "messageId": 1,
  "type": "user",
  "user": "brett",
  "message": "Hello!",
  "timestamp": "2026-03-18T09:15:06.1234567Z"
}
```

Messages are namespaced by `sessionId`; within a session, `messageId` is
unique and ordering is by `messageId`. A session's message list is served in
exactly this shape, so it can be saved to a file and fed back through
`enkente tail` or `parser.ParseChatLog`.

## Endpoints

### `POST /ingest`

Body: JSON object with the fields below.

| Field       | Required | Notes |
|-------------|----------|-------|
| `message`   | yes      | Must contain non-whitespace text. |
| `type`      | no       | Defaults to `user`. The TUI colours `system` distinctly. |
| `user`      | no       | Free-form identity string (the VS Code bridge resolves one). |
| `sessionId` | no       | Defaults to `live`. |
| `timestamp` | no       | RFC 3339 with a zone (`Z` or `±hh:mm`). Fractional seconds are accepted, which covers the .NET `"o"` format from the PowerShell forwarders for UTC and local-kind values; a zone-less `"o"` value (DateTimeKind.Unspecified) is rejected. Defaults to server time. |

The request body is capped at 1 MiB.

Responses:

| Status | Meaning |
|--------|---------|
| `201 Created`  | Persisted. Body carries the stored message with its assigned `messageId`. |
| `202 Accepted` | Persistence is disabled; dispatched to workers only. `messageId` is `0`. |
| `400`          | Malformed JSON, empty `message`, or unparseable `timestamp`. |
| `413`          | Body larger than 1 MiB. |
| `500`          | The datastore write failed. The message is not dispatched. |

The response is written as soon as the message is stored; dispatch to the
worker swarm happens afterwards, so a slow swarm cannot stall clients.

Body shape:

```json
{"status": "accepted", "message": { ...stored message... }}
```

### `GET /health`

```json
{"status": "ok", "persistence": true}
```

### `GET /sessions`

Lists every session holding messages, ordered by session ID.

```json
[{"sessionId": "live", "messageCount": 3}, {"sessionId": "proj-a", "messageCount": 12}]
```

### `GET /sessions/{sessionId}/messages`

All messages in the session, ordered by `messageId`. `404` if the session
does not exist.

### `GET /sessions/{sessionId}/messages/{messageId}`

One message. `404` if the session or message does not exist, `400` if
`messageId` is not a non-negative integer.

## Storage layout

Inside the `ChatLogs` bucket, each session is a nested BoltDB bucket named by
its `sessionId`. Each message is stored under an 8-byte big-endian key of its
`messageId`, which makes a cursor walk return messages in numeric order. The
nested bucket's sequence counter is kept at or above the highest stored
`messageId`, so an API-ingested message appended after a log import never
reuses one of the imported IDs. The reverse is not protected: a log import
is an upsert by `(sessionId, messageId)` and will overwrite an API-ingested
message that happens to share its ID. Keep log imports and live API
sessions in distinct `sessionId`s unless the log is imported first.
