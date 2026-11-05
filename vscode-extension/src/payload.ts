/**
 * Shape of a POST /ingest body. Field names follow the Antigravity chat-log
 * schema the server stores; sessionId is omitted (server default "live")
 * unless configured, and the timestamp is the client's send time.
 *
 * This module deliberately does not import 'vscode' so it can be unit-tested
 * with plain Node outside the editor host.
 */
export interface IngestPayload {
    type: string;
    user: string;
    message: string;
    timestamp: string;
    sessionId?: string;
}

export function buildIngestPayload(type: string, user: string, message: string, sessionId: string, now: Date = new Date()): IngestPayload {
    const payload: IngestPayload = { type, user, message, timestamp: now.toISOString() };
    const session = sessionId.trim();
    if (session) {
        payload.sessionId = session;
    }
    return payload;
}
