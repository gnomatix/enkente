# enkente Ingestion & Chat-Log API

The `enkente serve` command exposes a REST surface for pushing chat messages
into the datastore and reading them back. It covers:

- **Chat-log storage and namespacing** (requirements §2): sessions, and
  filters by user, project and time window.
- **Explicit references** (§1): `#tag` and `@user` mentions, with who
  introduced each one.
- **Curated entity records** (§1 and the curation part of §3).
- **A node-link graph export** (§2 rich data-encoding).
- **Session summaries and topic clusters** (§1). These use a built-in
  keyword baseline, not NLTK.
- **The passive and active jargon/ambiguity listener** (§1).

The NLTK-based analysis that `docs/requirements.md` names is not implemented.

## Running

```sh
enkente serve --port 8080 --db enkente.db
```

`--db` names the embedded BoltDB file. Pass `--db ""` to run without
persistence; `/ingest` then still fans messages out to the worker swarm and
the TUI, but nothing is stored and the read endpoints answer `503`.

`--headless` runs the same server without the TUI: each message is logged
to stdout, and SIGINT or SIGTERM shuts it down cleanly (in-flight requests
are drained for up to five seconds and the datastore is closed). Use it
where no terminal exists: CI, containers, systemd.

`enkente tail --log logs.json --db logs.db` persists messages from an
Antigravity log file into a datastore, keyed by the log's own `sessionId`
and `messageId`, so re-tailing a log is idempotent.

Three headless subcommands work on a datastore without a terminal, for
scripts and CI:

```sh
enkente import   --log logs.json --db enkente.db [--session <id>]
enkente export   --db enkente.db [--session <id>] [--format json|jsonl|csv] [--compact]
enkente sessions --db enkente.db                                 # session table
```

`import` is idempotent (same identity overwrites) and all-or-nothing: the
log is validated first and written in one transaction, so a bad log never
leaves a partial or newly created datastore behind. `--session` forces
every imported message into one session, which is needed for logs without
a `sessionId`; the import is refused if that would make two messages share
a `messageId`. `export` without `--session` emits every session, ordered
by session and then message ID; with an unknown `--session` it exits
non-zero. Its JSON output is re-ingestable by `import`, `tail`, or
`parser.ParseChatLog`.

`export --format` chooses the output shape for external analysis or
migration (requirements §5.3): `json` (the default) is the indented
Antigravity-shaped array described below, with `--compact` collapsing it
to one line; `jsonl` writes one compact JSON object per line with the same
field names, for streaming tools and line-oriented NoSQL loaders; `csv`
writes a header row `sessionId,messageId,type,user,message,timestamp,project`
followed by one row per message, quoted by `encoding/csv`, with timestamps
in RFC 3339 nanosecond form exactly as stored (no time-zone
normalisation). `--compact` affects only `json`. An unknown `--format` is
a usage error naming the accepted values, rejected before the datastore is
opened, so a typo never creates an empty database file.

BoltDB permits one process per file. A second `serve` or `tail` pointed at a
datastore that is already open fails within two seconds with a "locked by
another process" error rather than hanging. Use separate `--db` files for
concurrent processes.

### PowerShell forwarders

`tools/ag-forward.ps1 "text"` posts one message (`-Type`, `-User`,
`-SessionId`, `-ApiUrl`, `-Quiet`); `vscode-extension/scripts/forward-win.ps1`
posts a ready-made JSON payload. Both require PowerShell 7, honour
`ENKENTE_API_URL`, print the stored message, and on failure write a
non-terminating error (including the server's reason) and exit 1, so a
calling script can check `$LASTEXITCODE`. `make ps-lint` syntax-checks
them; `mise` installs `pwsh` on any platform. `make ps-test` (or
`mise run ps-test`) builds the binary and runs `tools/test-forwarders.ps1`,
which starts `enkente serve --headless` on a free port with a throw-away
datastore and checks both forwarders' exit codes, printed JSON and stored
sessions end to end.

## Message document

Every stored message is an Antigravity-shaped JSON document:

```json
{
  "sessionId": "live",
  "messageId": 1,
  "type": "user",
  "user": "alice",
  "message": "Hello!",
  "timestamp": "2026-03-18T09:15:06.1234567Z",
  "project": "enkente"
}
```

`project` is optional and omitted when empty. Messages are namespaced by `sessionId`; within a session, `messageId` is
unique and ordering is by `messageId`. A session's message list is served in
exactly this shape, so it can be saved to a file and fed back through
`enkente import`, `enkente tail` or `parser.ParseChatLog`.

## Endpoints

### `POST /ingest`

Body: JSON object with the fields below.

| Field       | Required | Notes |
|-------------|----------|-------|
| `message`   | yes      | Must contain non-whitespace text. |
| `type`      | no       | Defaults to `user`. The TUI colours `system` distinctly. |
| `user`      | no       | Free-form identity string (the VS Code bridge resolves one). |
| `project`   | no       | Namespace above the session, such as a repository or programme. Trimmed; omitted from stored JSON when empty. |
| `sessionId` | no       | Defaults to `live`. The VS Code bridge sends the `enkente.sessionId` setting when it is non-empty; `tools/ag-forward.ps1` sends `-SessionId` or `ENKENTE_SESSION_ID`. |
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

### `GET /`

Serves a small built-in web viewer (also at `GET /ui` and `GET /ui/`) as
`text/html`. It is a single self-contained page embedded in the binary: no
external scripts, stylesheets or CDN assets, so it works offline and behind
strict proxies.

The page lists sessions from `GET /sessions` (refreshed every 5 s). Picking
one renders its messages (timestamp, user or type, text) and polls
`GET /sessions/{sessionId}/messages?after=<lastMessageId>` every 2 s,
appending new messages and auto-scrolling. The selected session is kept in
the URL fragment (`/#live`) so a view can be bookmarked. A status line
reports `connected` or the last error; when persistence is disabled the
page still loads and the status line shows the `503` error text from
`/sessions`.

Only these exact paths are served. Any other path (for example `/nope` or
`/ui/x`) is `404`, and the JSON endpoints are unaffected. This is a seed
for the real-time visualization of requirements §4.1, not the mind-map UI.

### `GET /sessions`

Lists every session, ordered by session ID. A session whose last message
was deleted remains listed with `messageCount` 0 until the session itself
is deleted, so its ID counter is preserved.

```json
[{"sessionId": "live", "messageCount": 3}, {"sessionId": "proj-a", "messageCount": 12}]
```

### `GET /sessions/{sessionId}/messages`

Messages in the session, ordered by `messageId`. `404` if the session does
not exist.

| Query param | Notes |
|-------------|-------|
| `after`     | Return only messages with `messageId` greater than this. Use the last `messageId` seen to poll incrementally. |
| `limit`     | Maximum number of matching messages to return. Default and cap are 1000. |
| `user`      | Only messages from this user (case-insensitive). |
| `project`   | Only messages filed under this project (case-insensitive). Unassigned messages never match. |
| `since`     | Only messages with `timestamp` at or after this RFC 3339 time. |
| `until`     | Only messages with `timestamp` before this RFC 3339 time. Adjacent `[since, until)` windows never overlap. |

`after` and `limit` must be non-negative integers. `since` and `until` must
be RFC 3339 timestamps, and when both are given, `until` must be later than
`since`. Any violation returns `400`. Filters combine with `after`, so
incremental polling of one user's messages works. An empty result is `[]`,
not `null`.

### `GET /sessions/{sessionId}/messages/{messageId}`

One message. `404` if the session or message does not exist, `400` if
`messageId` is not a non-negative integer.

### `DELETE /sessions/{sessionId}/messages/{messageId}`

Removes one message. `204` on success, `404` if the session or message
does not exist, `400` for a malformed `messageId`. The session's ID
counter is not rewound, so a deleted ID is never reassigned; deleting the
last message leaves an empty session in place (listing returns `[]`).

### `DELETE /sessions/{sessionId}`

Removes a session and every message in it. `204` on success, `404` if the
session does not exist. A session recreated afterwards starts its IDs
afresh.

### Passive listener (jargon and ambiguity)

Start the server with `--lexicon path.json` to enable it. The lexicon is a
JSON array of terms. For an example, see `examples/lexicon.json`:

```json
[{"term": "database", "senses": ["storage engine", "a dataset"], "synonyms": ["db", "datastore"]}]
```

A term with two or more `senses` is *overloaded*. `synonyms` map other
spellings to the term. How matching works:

- Terms and synonyms match whole words, ignoring case and punctuation.
- Multi-word phrases are allowed. When two phrases start at the same word,
  the longer one wins, so `graph db` is matched before `db`.
- A phrase may belong to only one term. If two terms claim the same phrase,
  the server refuses to start.

The listener is passive. It only adds metadata and never changes or blocks a
message.

- **On ingest:** the `POST /ingest` response gains a `listener` object:
  `{"flags": [...], "overloaded": ["database"]}`. Each flag has `term`,
  `matched` (the phrase as written), `overloaded`, `senses`, `synonym` (true
  when a synonym was used) and `count`.
- **`POST /listener/analyze`:** takes `{"message": "..."}` and returns the
  same object for a draft, without storing it. This supports checking a
  phrasing before sending.
- **Without a lexicon:** ingest responses omit `listener`, and
  `/listener/analyze` returns `503`. A body that is not valid JSON returns
  `400`.

#### Active mode

Add `--listener-active` along with `--lexicon` to make the listener speak up.
If an ingested message uses an overloaded term, the server appends a
clarification message to the same session:

- It is a `system` message from user `listener`, with the same `project` as
  the original.
- It is stored with the next `messageId` and dispatched to the workers like
  any other message.
- It is also returned as `prompt` in the ingest response.

Example prompt:

```
@alice Listener here: which sense of "database" did you mean? (a) storage engine (b) dataset
```

No prompt is posted for messages of type `system` or from user `listener`, so
prompts can't trigger further prompts. `--listener-active` without
`--lexicon` refuses to start.

### Mentions

A mention is an explicit reference in message text: `#topic` for a subject
tag and `@name` for a user. Rules:

- The sigil must start the text or follow a character that can't be part of
  a name. As a result, `user@example.com` and `page#section` are not
  mentions.
- A name may contain letters, digits, `_`, `-` and `.`. Trailing `.` and `-`
  are treated as punctuation and dropped.
- Names are compared case-insensitively and returned in lower case.
- A name made only of digits, such as `#42`, is not a mention.

Mentions are computed from the stored text on every request, so they
immediately reflect upserts and deletes. There is no separate index to fall
out of date.

#### `GET /mentions`

Returns a summary of every mention across all sessions. Results are sorted by
message count, highest first, then by kind and name. The optional
`kind=tag|user` parameter restricts the list to one kind.

```json
[{"kind":"tag","name":"graphdb","messages":3,"sessions":2,
  "firstSeen":{"sessionId":"s1","messageId":0,"user":"alice", "...": "..."}}]
```

- `messages` counts messages that contain the mention. A message that repeats
  it counts once.
- `firstSeen` is the earliest message containing the mention, which shows
  who introduced the entity and when.

An invalid `kind` returns `400`.

#### `GET /mentions/{kind}/{name}/messages`

Returns the messages that reference one tag or user, oldest first, in the
chat-log shape. `{kind}` is `tag` or `user`. `limit=<n>` caps the count at
`n`, up to 1000. If nobody has mentioned the name, the response is `[]`.

Returns `400` when `kind` is invalid, `limit` is not a non-negative integer,
or `name` is not exactly one valid mention.

### Entities (curation)

An entity is a tag or user, identified by `{kind}/{name}` with the same name
rules as mentions. It can carry a curated record: a `description`, free-form
`attributes`, and accumulated `notes`. Notes hold stories or anecdotes about
the entity. Each note has `text`, `author` and `addedAt`.

| Route | Effect |
|-------|--------|
| `GET /entities` | All curated records, ordered by kind then name. |
| `GET /entities/{kind}/{name}` | `{"kind","name","curated","mentions"}`. `curated` is the record, or `null` if nobody has curated it. `mentions` is the summary from `GET /mentions` (including `firstSeen`), or `null` if nobody has mentioned it. Returns `404` only when both are empty. |
| `PUT /entities/{kind}/{name}` | Creates or replaces the record. The body may contain only `description`, `attributes` and `notes`; any other field returns `400`. Notes without `addedAt` are stamped with the server time. Returns `200` with the saved record. |
| `POST /entities/{kind}/{name}/notes` | Appends one note (`{"text","author"}`), creating the record if needed. Returns `201`. |
| `DELETE /entities/{kind}/{name}` | Removes the curated record and leaves mentions in the chat log untouched. Returns `204`, or `404` if there is no record. |

Errors:

- An invalid kind or name returns `400`, and so does an empty note text.
- These routes need a datastore. Without one they return `503`.

Records live in the `Concepts` bucket under the key `<kind>:<name>`.

### `GET /sessions/{sessionId}/summary`

Returns an on-demand summary of one session:

- `messages`, `start` and `end`.
- `participants`: users and their message counts, most active first.
- `tags`: tags in the order they first appeared.
- `keywords`: each has a `term`, `count` and `score`.
- `highlights`: the most representative messages, in chronological order.

`method` is currently `tfidf-baseline`:

- **Keywords** are words of three letters or more, excluding stopwords,
  numbers and `#`/`@` mentions. They are scored by TF-IDF against every
  stored session, so words common to all sessions rank low.
- **Highlights** are the messages whose distinct keywords score highest,
  adjusted for message length.

This baseline needs no NLP library. `docs/requirements.md` names NLTK for
tokenization, part-of-speech tagging and named-entity recognition. An engine
built on it would replace the baseline behind the same response shape.

| Query param | Default | Notes |
|-------------|---------|-------|
| `keywords`   | 15 | Number of keywords to return (max 100; `0` for none). |
| `highlights` | 5  | Number of highlight messages to return (max 100; `0` for none). |

Returns `404` for an unknown session and `400` for a negative or non-integer
count.

### `GET /sessions/{sessionId}/topics`

Groups a session's messages into topics. The baseline method works like
this:

1. Up to `k` seeds are chosen, `8` by default and `100` at most. The
   session's tags come first, as `#tag`, followed by its top keywords.
2. Each message joins the seed it contains with the most weight. A tag
   outweighs any keyword. On a tie, the seed listed earlier wins.
3. Messages that match no seed go into a final `other` topic.

Every message lands in exactly one topic. Seeds with no messages are left
out of the response.

```json
[{"label": "#graphdb", "messageIds": [1]}, {"label": "graph", "messageIds": [2, 4]}, {"label": "other", "messageIds": [3]}]
```

Returns `404` for an unknown session and `400` for a bad `k`.

### `GET /graph`

Returns a node-link graph of who talks about what, in the
`{"nodes":[...],"edges":[...]}` shape that force-directed layouts such as
d3-force and Cytoscape read.

**Nodes:**
- Each node has `id` (`user:<name>` or `tag:<name>`), `kind`, `name`,
  `messages`, `firstSeen` and `lastSeen`.
- Message authors and `@` mentions share the `user:` namespace, so writing
  as `alice` and being mentioned as `@alice` count toward one node.
- `messages` counts the messages a user wrote plus the messages that mention
  the node.

**Edges:**
- Each edge has `source`, `target`, `relation` and a `weight` equal to the
  number of messages behind it.
- `mentions` goes from a message author to each tag or user they referenced.
  Self-mentions are skipped.
- `co-occurs` joins two tags that appear in the same message. Each pair
  appears once, with `source` sorting before `target`.
- Nodes, and edges by `source` then `target`, are listed in a fixed sort
  order.

**Query parameters:**
- `session` restricts the graph to one session. An unknown session returns
  `404`.
- `user`, `project`, `since` and `until` filter the input messages, exactly
  as on message listings.

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
