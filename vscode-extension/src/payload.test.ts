import { test } from 'node:test';
import assert from 'node:assert/strict';

import { buildIngestPayload } from './payload';

const fixed = new Date('2026-03-18T09:15:06.123Z');

test('omits sessionId when empty so the server default applies', () => {
    const p = buildIngestPayload('user', 'brett', 'Hello!', '', fixed);
    assert.deepEqual(p, { type: 'user', user: 'brett', message: 'Hello!', timestamp: '2026-03-18T09:15:06.123Z' });
    assert.ok(!('sessionId' in p));
});

test('includes a trimmed sessionId when configured', () => {
    const p = buildIngestPayload('user', 'brett', 'Hello!', '  proj-a ', fixed);
    assert.equal(p.sessionId, 'proj-a');
});

test('treats a whitespace-only sessionId as unset', () => {
    const p = buildIngestPayload('user', 'brett', 'Hello!', '   ', fixed);
    assert.ok(!('sessionId' in p));
});

test('serialises to the field names the ingestion API expects', () => {
    const json = JSON.parse(JSON.stringify(buildIngestPayload('system', 'anon-1234', 'x', 's', fixed)));
    assert.deepEqual(Object.keys(json).sort(), ['message', 'sessionId', 'timestamp', 'type', 'user']);
});

test('timestamp is RFC 3339 in UTC', () => {
    const p = buildIngestPayload('user', 'u', 'm', '');
    assert.match(p.timestamp, /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/);
});
