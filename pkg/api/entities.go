package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/storage"
)

// EntityStore is the curation surface for #tag and @user records. The
// server enables the /entities routes only when its MessageStore also
// implements this interface (*storage.BoltStorage does).
type EntityStore interface {
	PutEntity(e storage.Entity, now time.Time) (storage.Entity, error)
	AddEntityNote(kind parser.MentionKind, name string, note storage.EntityNote, now time.Time) (storage.Entity, error)
	DeleteEntity(kind parser.MentionKind, name string) error
	ListEntities() ([]storage.Entity, error)
	ViewEntity(kind parser.MentionKind, name string) (*storage.EntityView, error)
}

func (s *Server) entityStore(w http.ResponseWriter) (EntityStore, bool) {
	if !s.requireStore(w) {
		return nil, false
	}
	es, ok := s.store.(EntityStore)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "the configured store does not support entities")
		return nil, false
	}
	return es, true
}

// entityPath parses {kind}/{name}, writing a 400 on a bad kind.
func entityPath(w http.ResponseWriter, r *http.Request) (parser.MentionKind, string, bool) {
	kind, ok := parser.ParseMentionKind(r.PathValue("kind"))
	if !ok {
		writeError(w, http.StatusBadRequest, "kind must be tag or user")
		return "", "", false
	}
	return kind, r.PathValue("name"), true
}

// writeEntityErr maps storage errors to status codes.
func writeEntityErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrInvalidMention):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, storage.ErrNotFound):
		writeError(w, http.StatusNotFound, "entity not found")
	default:
		writeError(w, http.StatusInternalServerError, "entity store error")
	}
}

func (s *Server) handleListEntities(w http.ResponseWriter, r *http.Request) {
	es, ok := s.entityStore(w)
	if !ok {
		return
	}
	list, err := es.ListEntities()
	if err != nil {
		writeEntityErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetEntity(w http.ResponseWriter, r *http.Request) {
	es, ok := s.entityStore(w)
	if !ok {
		return
	}
	kind, name, ok := entityPath(w, r)
	if !ok {
		return
	}
	view, err := es.ViewEntity(kind, name)
	if err != nil {
		writeEntityErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// entityBody is the PUT body: the curated fields only. Kind and name come
// from the path.
type entityBody struct {
	Description string               `json:"description"`
	Attributes  map[string]string    `json:"attributes"`
	Notes       []storage.EntityNote `json:"notes"`
}

func (s *Server) handlePutEntity(w http.ResponseWriter, r *http.Request) {
	es, ok := s.entityStore(w)
	if !ok {
		return
	}
	kind, name, ok := entityPath(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxIngestBodyBytes)
	var body entityBody
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "body must be JSON with description, attributes and/or notes")
		return
	}
	saved, err := es.PutEntity(storage.Entity{
		Kind: kind, Name: name, Description: body.Description,
		Attributes: body.Attributes, Notes: body.Notes,
	}, s.now())
	if err != nil {
		if errors.Is(err, storage.ErrInvalidMention) || errors.Is(err, storage.ErrNotFound) {
			writeEntityErr(w, err)
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleAddEntityNote(w http.ResponseWriter, r *http.Request) {
	es, ok := s.entityStore(w)
	if !ok {
		return
	}
	kind, name, ok := entityPath(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxIngestBodyBytes)
	var note storage.EntityNote
	if err := json.NewDecoder(r.Body).Decode(&note); err != nil {
		writeError(w, http.StatusBadRequest, "body must be JSON with a text field")
		return
	}
	saved, err := es.AddEntityNote(kind, name, note, s.now())
	if err != nil {
		if errors.Is(err, storage.ErrInvalidMention) {
			writeEntityErr(w, err)
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) handleDeleteEntity(w http.ResponseWriter, r *http.Request) {
	es, ok := s.entityStore(w)
	if !ok {
		return
	}
	kind, name, ok := entityPath(w, r)
	if !ok {
		return
	}
	if err := es.DeleteEntity(kind, name); err != nil {
		writeEntityErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
