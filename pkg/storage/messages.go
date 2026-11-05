package storage

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"

	"go.etcd.io/bbolt"

	"github.com/gnomatix/enkente/pkg/parser"
)

// ErrNotFound is returned when a requested session or message does not exist.
var ErrNotFound = errors.New("storage: not found")

// SessionInfo summarises one chat session stored in the ChatLogs bucket.
type SessionInfo struct {
	SessionID    string `json:"sessionId"`
	MessageCount int    `json:"messageCount"`
}

// Chat messages are stored as JSON documents under ChatBucket, namespaced by
// session: each session ID is a nested bucket, and each message is keyed by
// its message ID encoded as an 8-byte big-endian integer. Big-endian keys
// sort numerically under BoltDB's byte-wise ordering, so a cursor walk over a
// session bucket yields messages in message-ID order. The nested bucket's
// sequence counter is kept at or above the highest stored message ID so that
// AppendMessage can hand out the next free ID atomically.

func messageKey(messageID int) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, uint64(messageID))
	return key
}

func sessionBucket(tx *bbolt.Tx, sessionID string, create bool) (*bbolt.Bucket, error) {
	chat := tx.Bucket([]byte(ChatBucket))
	if chat == nil {
		return nil, fmt.Errorf("bucket %s not found", ChatBucket)
	}
	if create {
		return chat.CreateBucketIfNotExists([]byte(sessionID))
	}
	return chat.Bucket([]byte(sessionID)), nil
}

func validateMessage(msg parser.AntigravityMessage) error {
	if msg.SessionID == "" {
		return errors.New("storage: message has empty sessionId")
	}
	if msg.MessageID < 0 {
		return fmt.Errorf("storage: message has negative messageId %d", msg.MessageID)
	}
	return nil
}

func writeMessage(b *bbolt.Bucket, msg parser.AntigravityMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	if err := b.Put(messageKey(msg.MessageID), data); err != nil {
		return err
	}
	// Keep the per-session sequence ahead of every explicitly supplied ID so
	// AppendMessage never collides with a message ingested from a log file.
	if uint64(msg.MessageID) > b.Sequence() {
		return b.SetSequence(uint64(msg.MessageID))
	}
	return nil
}

// PutMessage stores msg under its own session and message ID, replacing any
// existing document with the same identity. Re-ingesting the same log file
// is therefore idempotent.
func (s *BoltStorage) PutMessage(msg parser.AntigravityMessage) error {
	if err := validateMessage(msg); err != nil {
		return err
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		b, err := sessionBucket(tx, msg.SessionID, true)
		if err != nil {
			return err
		}
		return writeMessage(b, msg)
	})
}

// AppendMessage stores msg under its session with a freshly assigned message
// ID, ignoring any MessageID already set on msg. IDs within a session are
// strictly increasing and never reuse an ID written by PutMessage. The stored
// message, including its assigned ID, is returned.
func (s *BoltStorage) AppendMessage(msg parser.AntigravityMessage) (parser.AntigravityMessage, error) {
	msg.MessageID = 0
	if err := validateMessage(msg); err != nil {
		return msg, err
	}
	err := s.db.Update(func(tx *bbolt.Tx) error {
		b, err := sessionBucket(tx, msg.SessionID, true)
		if err != nil {
			return err
		}
		seq, err := b.NextSequence()
		if err != nil {
			return err
		}
		msg.MessageID = int(seq)
		return writeMessage(b, msg)
	})
	return msg, err
}

// GetMessage retrieves one message by session and message ID.
// It returns ErrNotFound when either the session or the message is absent.
func (s *BoltStorage) GetMessage(sessionID string, messageID int) (*parser.AntigravityMessage, error) {
	if messageID < 0 {
		return nil, ErrNotFound
	}
	var msg parser.AntigravityMessage
	err := s.db.View(func(tx *bbolt.Tx) error {
		b, err := sessionBucket(tx, sessionID, false)
		if err != nil {
			return err
		}
		if b == nil {
			return ErrNotFound
		}
		v := b.Get(messageKey(messageID))
		if v == nil {
			return ErrNotFound
		}
		return json.Unmarshal(v, &msg)
	})
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// ListMessages returns every message in a session ordered by message ID.
// It returns ErrNotFound when the session does not exist.
func (s *BoltStorage) ListMessages(sessionID string) ([]parser.AntigravityMessage, error) {
	messages := []parser.AntigravityMessage{}
	err := s.db.View(func(tx *bbolt.Tx) error {
		b, err := sessionBucket(tx, sessionID, false)
		if err != nil {
			return err
		}
		if b == nil {
			return ErrNotFound
		}
		return b.ForEach(func(k, v []byte) error {
			if v == nil {
				return nil // nested bucket, not a message
			}
			var msg parser.AntigravityMessage
			if err := json.Unmarshal(v, &msg); err != nil {
				return fmt.Errorf("unmarshal message %x in session %s: %w", k, sessionID, err)
			}
			messages = append(messages, msg)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return messages, nil
}

// ListSessions returns a summary of every session that holds messages,
// ordered by session ID.
func (s *BoltStorage) ListSessions() ([]SessionInfo, error) {
	sessions := []SessionInfo{}
	err := s.db.View(func(tx *bbolt.Tx) error {
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
			count := 0
			if err := sub.ForEach(func(_, mv []byte) error {
				if mv != nil {
					count++
				}
				return nil
			}); err != nil {
				return err
			}
			sessions = append(sessions, SessionInfo{SessionID: string(k), MessageCount: count})
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return sessions, nil
}
