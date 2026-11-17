package storage

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.etcd.io/bbolt"

	"github.com/gnomatix/enkente/pkg/parser"
)

// Entity is the curated record for a #tag or @user: the knowledge people
// accumulate about it beyond what the chat text says. It is stored in the
// Concepts bucket under "<kind>:<name>".
type Entity struct {
	Kind parser.MentionKind `json:"kind"`
	Name string             `json:"name"`
	// Description is a short curated definition.
	Description string `json:"description,omitempty"`
	// Attributes holds free-form key/value facts (e.g. "owner": "alice").
	Attributes map[string]string `json:"attributes,omitempty"`
	// Notes are accumulated stories or anecdotes, oldest first.
	Notes     []EntityNote `json:"notes,omitempty"`
	UpdatedAt time.Time    `json:"updatedAt"`
}

// EntityNote is one story or anecdote attached to an entity.
type EntityNote struct {
	Text    string    `json:"text"`
	Author  string    `json:"author,omitempty"`
	AddedAt time.Time `json:"addedAt"`
}

// EntityView joins the curated record (nil when nobody has curated the
// entity yet) with what the chat log says about it (nil when it has never
// been mentioned).
type EntityView struct {
	Kind     parser.MentionKind `json:"kind"`
	Name     string             `json:"name"`
	Curated  *Entity            `json:"curated"`
	Mentions *MentionSummary    `json:"mentions"`
}

// normalizeEntity validates kind and name with the mention rules and returns
// the canonical (lower-case) name and storage key.
func normalizeEntity(kind parser.MentionKind, name string) (string, []byte, error) {
	if kind != parser.MentionTag && kind != parser.MentionUser {
		return "", nil, fmt.Errorf("%w: kind %q", ErrInvalidMention, kind)
	}
	got := parser.ExtractMentions(string(kindSigil(kind)) + name)
	if len(got) != 1 || got[0].Name != strings.ToLower(name) {
		return "", nil, fmt.Errorf("%w: %s %q", ErrInvalidMention, kind, name)
	}
	return got[0].Name, []byte(string(kind) + ":" + got[0].Name), nil
}

// PutEntity creates or replaces the curated record for an entity. Kind and
// Name in e are normalized; UpdatedAt is set to now. Notes without AddedAt
// get now.
func (s *BoltStorage) PutEntity(e Entity, now time.Time) (Entity, error) {
	name, key, err := normalizeEntity(e.Kind, e.Name)
	if err != nil {
		return Entity{}, err
	}
	e.Name = name
	e.UpdatedAt = now.UTC()
	for i := range e.Notes {
		if strings.TrimSpace(e.Notes[i].Text) == "" {
			return Entity{}, fmt.Errorf("note %d: text is empty", i)
		}
		if e.Notes[i].AddedAt.IsZero() {
			e.Notes[i].AddedAt = e.UpdatedAt
		}
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return Entity{}, err
	}
	err = s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket([]byte(ConceptBucket)).Put(key, raw)
	})
	return e, err
}

// AddEntityNote appends a story to an entity, creating its record if needed.
func (s *BoltStorage) AddEntityNote(kind parser.MentionKind, name string, note EntityNote, now time.Time) (Entity, error) {
	if strings.TrimSpace(note.Text) == "" {
		return Entity{}, fmt.Errorf("note text is empty")
	}
	name, key, err := normalizeEntity(kind, name)
	if err != nil {
		return Entity{}, err
	}
	var e Entity
	err = s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(ConceptBucket))
		if v := b.Get(key); v != nil {
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
		} else {
			e = Entity{Kind: kind, Name: name}
		}
		e.UpdatedAt = now.UTC()
		if note.AddedAt.IsZero() {
			note.AddedAt = e.UpdatedAt
		}
		e.Notes = append(e.Notes, note)
		raw, err := json.Marshal(e)
		if err != nil {
			return err
		}
		return b.Put(key, raw)
	})
	return e, err
}

// GetEntity returns the curated record, or ErrNotFound.
func (s *BoltStorage) GetEntity(kind parser.MentionKind, name string) (*Entity, error) {
	_, key, err := normalizeEntity(kind, name)
	if err != nil {
		return nil, err
	}
	var e Entity
	err = s.db.View(func(tx *bbolt.Tx) error {
		v := tx.Bucket([]byte(ConceptBucket)).Get(key)
		if v == nil {
			return ErrNotFound
		}
		return json.Unmarshal(v, &e)
	})
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// DeleteEntity removes the curated record. Mentions in the chat log are
// untouched. It returns ErrNotFound when there is no record.
func (s *BoltStorage) DeleteEntity(kind parser.MentionKind, name string) error {
	_, key, err := normalizeEntity(kind, name)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(ConceptBucket))
		if b.Get(key) == nil {
			return ErrNotFound
		}
		return b.Delete(key)
	})
}

// ListEntities returns every curated record, ordered by kind then name.
func (s *BoltStorage) ListEntities() ([]Entity, error) {
	out := []Entity{}
	err := s.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket([]byte(ConceptBucket)).ForEach(func(k, v []byte) error {
			if v == nil || !strings.Contains(string(k), ":") {
				return nil
			}
			var e Entity
			if err := json.Unmarshal(v, &e); err != nil {
				return fmt.Errorf("entity %q: %w", k, err)
			}
			out = append(out, e)
			return nil
		})
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out, err
}

// ViewEntity joins the curated record with the mention summary. It returns
// ErrNotFound only when the entity is neither curated nor mentioned.
func (s *BoltStorage) ViewEntity(kind parser.MentionKind, name string) (*EntityView, error) {
	canon, _, err := normalizeEntity(kind, name)
	if err != nil {
		return nil, err
	}
	view := &EntityView{Kind: kind, Name: canon}
	if e, err := s.GetEntity(kind, canon); err == nil {
		view.Curated = e
	} else if err != ErrNotFound {
		return nil, err
	}
	summaries, err := s.ListMentions(kind)
	if err != nil {
		return nil, err
	}
	for i := range summaries {
		if summaries[i].Name == canon {
			view.Mentions = &summaries[i]
			break
		}
	}
	if view.Curated == nil && view.Mentions == nil {
		return nil, ErrNotFound
	}
	return view, nil
}
