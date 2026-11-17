package api

import (
	"errors"
	"net/http"

	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/storage"
	"github.com/gnomatix/enkente/pkg/summary"
)

// Defaults and caps for GET /sessions/{sessionId}/summary.
const (
	DefaultSummaryKeywords   = 15
	DefaultSummaryHighlights = 5
	MaxSummaryItems          = 100
)

// handleSessionSummary returns an on-demand digest of one session:
// participants, tags, TF-IDF keywords (scored against every other session)
// and extractive highlights. Query: keywords=<n>, highlights=<n>.
func (s *Server) handleSessionSummary(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	sessionID := r.PathValue("sessionId")
	nk, ok := queryInt(r, "keywords", DefaultSummaryKeywords)
	if !ok {
		writeError(w, http.StatusBadRequest, "keywords must be a non-negative integer")
		return
	}
	nh, ok := queryInt(r, "highlights", DefaultSummaryHighlights)
	if !ok {
		writeError(w, http.StatusBadRequest, "highlights must be a non-negative integer")
		return
	}
	nk, nh = min(nk, MaxSummaryItems), min(nh, MaxSummaryItems)

	corpus, ok := s.loadCorpus(w, sessionID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, summary.Baseline{}.Summarize(sessionID, corpus, nk, nh))
}

// DefaultTopics is the default number of topic seeds.
const DefaultTopics = 8

// handleSessionTopics clusters a session's messages into topics. Query:
// k=<n> seeds (default DefaultTopics, max MaxSummaryItems).
func (s *Server) handleSessionTopics(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	sessionID := r.PathValue("sessionId")
	k, ok := queryInt(r, "k", DefaultTopics)
	if !ok {
		writeError(w, http.StatusBadRequest, "k must be a non-negative integer")
		return
	}
	corpus, ok := s.loadCorpus(w, sessionID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, summary.Baseline{}.Topics(sessionID, corpus, min(k, MaxSummaryItems)))
}

// loadCorpus reads every session's messages (TF-IDF needs them all for
// document frequency), writing 404 when sessionID does not exist.
func (s *Server) loadCorpus(w http.ResponseWriter, sessionID string) (map[string][]parser.AntigravityMessage, bool) {
	sessions, err := s.store.ListSessions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list sessions")
		return nil, false
	}
	corpus := map[string][]parser.AntigravityMessage{}
	for _, info := range sessions {
		msgs, err := s.store.ListMessagesFiltered(info.SessionID, -1, 0, storage.MessageFilter{})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read messages")
			return nil, false
		}
		corpus[info.SessionID] = msgs
	}
	if _, found := corpus[sessionID]; !found {
		// Distinguish an empty existing session from a missing one.
		if _, err := s.store.ListMessagesFiltered(sessionID, -1, 1, storage.MessageFilter{}); errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "session not found")
			return nil, false
		}
	}
	return corpus, true
}
