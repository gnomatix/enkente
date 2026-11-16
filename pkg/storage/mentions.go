package storage

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"go.etcd.io/bbolt"

	"github.com/gnomatix/enkente/pkg/parser"
)

// MentionSummary counts how often one #tag or @user is referenced.
// Messages is the number of messages that mention it (a message that repeats
// a mention counts once); Sessions is the number of distinct sessions.
// FirstSeen is the earliest message that mentions it, which records who
// introduced the entity and when.
type MentionSummary struct {
	Kind      parser.MentionKind         `json:"kind"`
	Name      string                     `json:"name"`
	Messages  int                        `json:"messages"`
	Sessions  int                        `json:"sessions"`
	FirstSeen *parser.AntigravityMessage `json:"firstSeen"`
}

// forEachMessage calls fn for every stored message, session by session in
// key order and by message ID within a session.
func forEachMessage(tx *bbolt.Tx, fn func(msg parser.AntigravityMessage) error) error {
	chat := tx.Bucket([]byte(ChatBucket))
	if chat == nil {
		return fmt.Errorf("bucket %s not found", ChatBucket)
	}
	return chat.ForEach(func(k, v []byte) error {
		if v != nil {
			return nil // plain key, not a session bucket
		}
		sub := chat.Bucket(k)
		if sub == nil {
			return nil
		}
		return sub.ForEach(func(_, raw []byte) error {
			var msg parser.AntigravityMessage
			if err := json.Unmarshal(raw, &msg); err != nil {
				return fmt.Errorf("session %q: decode message: %w", k, err)
			}
			return fn(msg)
		})
	})
}

// earlier reports whether a was posted before b, breaking timestamp ties by
// session and message ID so the result does not depend on scan order.
func earlier(a, b parser.AntigravityMessage) bool {
	if !a.Timestamp.Equal(b.Timestamp) {
		return a.Timestamp.Before(b.Timestamp)
	}
	if a.SessionID != b.SessionID {
		return a.SessionID < b.SessionID
	}
	return a.MessageID < b.MessageID
}

// ListMentions summarizes every #tag and @user found in stored messages,
// ordered by message count (descending) then kind and name. Pass an empty
// kind for both kinds.
//
// Mentions are derived from message text on each call rather than kept in a
// separate index, so edits and deletes can never leave stale entries. The
// cost is a full scan, which is acceptable for chat-log sized stores.
func (s *BoltStorage) ListMentions(kind parser.MentionKind) ([]MentionSummary, error) {
	type acc struct {
		summary  MentionSummary
		first    parser.AntigravityMessage
		sessions map[string]bool
	}
	byMention := map[parser.Mention]*acc{}
	err := s.db.View(func(tx *bbolt.Tx) error {
		return forEachMessage(tx, func(msg parser.AntigravityMessage) error {
			for _, m := range parser.ExtractMentions(msg.Message) {
				if kind != "" && m.Kind != kind {
					continue
				}
				a := byMention[m]
				if a == nil {
					a = &acc{summary: MentionSummary{Kind: m.Kind, Name: m.Name}, first: msg, sessions: map[string]bool{}}
					byMention[m] = a
				} else if earlier(msg, a.first) {
					a.first = msg
				}
				a.summary.Messages++
				a.sessions[msg.SessionID] = true
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	out := make([]MentionSummary, 0, len(byMention))
	for _, a := range byMention {
		a.summary.Sessions = len(a.sessions)
		first := a.first
		a.summary.FirstSeen = &first
		out = append(out, a.summary)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Messages != out[j].Messages {
			return out[i].Messages > out[j].Messages
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// MessagesMentioning returns the messages that reference the given #tag or
// @user, oldest first, stopping after limit messages when limit > 0. The name
// is matched case-insensitively. An entity nobody mentioned yields an empty
// slice, not an error.
func (s *BoltStorage) MessagesMentioning(kind parser.MentionKind, name string, limit int) ([]parser.AntigravityMessage, error) {
	want := parser.ExtractMentions(string(kindSigil(kind)) + name)
	if len(want) != 1 || want[0].Kind != kind || want[0].Name != strings.ToLower(name) {
		return nil, fmt.Errorf("invalid %s name %q", kind, name)
	}
	target := want[0]
	out := []parser.AntigravityMessage{}
	err := s.db.View(func(tx *bbolt.Tx) error {
		return forEachMessage(tx, func(msg parser.AntigravityMessage) error {
			for _, m := range parser.ExtractMentions(msg.Message) {
				if m == target {
					out = append(out, msg)
					break
				}
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return earlier(out[i], out[j]) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func kindSigil(kind parser.MentionKind) rune {
	if kind == parser.MentionUser {
		return '@'
	}
	return '#'
}
