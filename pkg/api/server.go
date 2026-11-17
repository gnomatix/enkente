package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gnomatix/enkente/pkg/listener"
	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/storage"
)

// DefaultSessionID is the session assigned to ingested messages that do not
// name one. Messages forwarded from live chat bridges land here.
const DefaultSessionID = "live"

// IngestRequest represents a message submitted via the REST API.
// Its field names mirror the Antigravity chat-log schema so that a stored
// session can be exported and re-ingested without translation.
type IngestRequest struct {
	SessionID string `json:"sessionId,omitempty"`
	Type      string `json:"type"`
	User      string `json:"user,omitempty"`
	Message   string `json:"message"`
	// Timestamp is optional and, when present, must be RFC 3339 with a zone
	// (Z or a numeric offset). Fractional seconds are accepted, which covers
	// the .NET "o" round-trip format emitted by the PowerShell forwarders for
	// UTC and local-kind values.
	Timestamp string `json:"timestamp,omitempty"`
	// Project optionally files the message under a namespace above the
	// session. Leading and trailing whitespace is removed.
	Project string `json:"project,omitempty"`
}

// MaxIngestBodyBytes caps the size of a single /ingest request body.
const MaxIngestBodyBytes = 1 << 20

// MessageStore is the persistence surface the server needs. *storage.BoltStorage
// satisfies it; tests may substitute their own implementation.
type MessageStore interface {
	AppendMessage(msg parser.AntigravityMessage) (parser.AntigravityMessage, error)
	GetMessage(sessionID string, messageID int) (*parser.AntigravityMessage, error)
	ListMessagesFiltered(sessionID string, afterID int, limit int, filter storage.MessageFilter) ([]parser.AntigravityMessage, error)
	ListSessions() ([]storage.SessionInfo, error)
	DeleteMessage(sessionID string, messageID int) error
	DeleteSession(sessionID string) error
	ListMentions(kind parser.MentionKind) ([]storage.MentionSummary, error)
	MessagesMentioning(kind parser.MentionKind, name string, limit int) ([]parser.AntigravityMessage, error)
}

// MaxListLimit caps the limit query parameter on message listings.
const MaxListLimit = 1000

// Option configures a Server.
type Option func(*Server)

// WithStore enables persistence: every accepted message is written to the
// store before being dispatched to the worker pool, and the read endpoints
// serve from it.
func WithStore(store MessageStore) Option {
	return func(s *Server) { s.store = store }
}

// WithLexicon enables the passive listener: ingest responses carry jargon and
// ambiguity metadata, and POST /listener/analyze previews a draft.
func WithLexicon(lex *listener.Lexicon) Option {
	return func(s *Server) { s.lexicon = lex }
}

// Server manages the HTTP ingestion endpoint and dispatches messages to a handler.
type Server struct {
	port    int
	handler func(workerID int, msg parser.AntigravityMessage)
	msgChan chan parser.AntigravityMessage
	store   MessageStore
	lexicon *listener.Lexicon
	now     func() time.Time
}

// NewServer creates a new ingestion server on the given port.
// numWorkers goroutines will concurrently consume messages using the handler.
func NewServer(port int, numWorkers int, handler func(workerID int, msg parser.AntigravityMessage), opts ...Option) *Server {
	s := &Server{
		port:    port,
		handler: handler,
		msgChan: make(chan parser.AntigravityMessage, 100),
		now:     time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}

	// Spawn worker swarm
	for i := 0; i < numWorkers; i++ {
		workerID := i
		go func() {
			for msg := range s.msgChan {
				handler(workerID, msg)
			}
		}()
	}

	return s
}

// Handler returns the routed HTTP handler so it can be mounted or tested
// without opening a listening socket.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /ingest", s.handleIngest)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /sessions", s.handleListSessions)
	mux.HandleFunc("GET /sessions/{sessionId}/messages", s.handleListMessages)
	mux.HandleFunc("GET /sessions/{sessionId}/messages/{messageId}", s.handleGetMessage)
	mux.HandleFunc("DELETE /sessions/{sessionId}", s.handleDeleteSession)
	mux.HandleFunc("DELETE /sessions/{sessionId}/messages/{messageId}", s.handleDeleteMessage)
	mux.HandleFunc("POST /listener/analyze", s.handleAnalyze)
	mux.HandleFunc("GET /entities", s.handleListEntities)
	mux.HandleFunc("GET /entities/{kind}/{name}", s.handleGetEntity)
	mux.HandleFunc("PUT /entities/{kind}/{name}", s.handlePutEntity)
	mux.HandleFunc("DELETE /entities/{kind}/{name}", s.handleDeleteEntity)
	mux.HandleFunc("POST /entities/{kind}/{name}/notes", s.handleAddEntityNote)
	mux.HandleFunc("GET /mentions", s.handleListMentions)
	mux.HandleFunc("GET /mentions/{kind}/{name}/messages", s.handleMentionMessages)
	s.registerWebRoutes(mux)
	return mux
}

// Start begins listening for HTTP requests and blocks until the listener
// fails. Use Serve for a server that can be stopped.
func (s *Server) Start() error {
	return s.Serve(context.Background(), nil)
}

// ShutdownTimeout bounds how long Serve waits for in-flight requests after
// its context is cancelled.
const ShutdownTimeout = 5 * time.Second

// Serve accepts connections on ln (or on the configured port when ln is nil)
// until ctx is cancelled, then shuts down gracefully, draining in-flight
// requests for up to ShutdownTimeout. It returns nil after a clean shutdown.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	if ln == nil {
		var err error
		ln, err = net.Listen("tcp", fmt.Sprintf(":%d", s.port))
		if err != nil {
			return err
		}
	}

	srv := &http.Server{Handler: s.Handler()}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		<-errCh
		return nil
	}
}

// Addr formats the address the server listens on when Serve is given no
// listener.
func (s *Server) Addr() string { return fmt.Sprintf(":%d", s.port) }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ingestResponse is the body returned for an accepted ingest request. The
// embedded message carries the identity (sessionId, messageId) and timestamp
// the server stored, so a client can later retrieve exactly what it sent.
type ingestResponse struct {
	Status  string                    `json:"status"`
	Message parser.AntigravityMessage `json:"message"`
	// Listener carries jargon and ambiguity metadata when the server has a
	// lexicon; it is omitted otherwise.
	Listener *listener.Summary `json:"listener,omitempty"`
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	var req IngestRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxIngestBodyBytes)).Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("body exceeds %d bytes", MaxIngestBodyBytes))
			return
		}
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if strings.TrimSpace(req.Message) == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	ts := s.now().UTC()
	if req.Timestamp != "" {
		parsed, err := time.Parse(time.RFC3339, req.Timestamp)
		if err != nil {
			writeError(w, http.StatusBadRequest, "timestamp must be RFC 3339")
			return
		}
		ts = parsed
	}

	msg := parser.AntigravityMessage{
		SessionID: strings.TrimSpace(req.SessionID),
		Type:      strings.TrimSpace(req.Type),
		User:      strings.TrimSpace(req.User),
		Message:   req.Message,
		Timestamp: ts,
		Project:   strings.TrimSpace(req.Project),
	}
	if msg.SessionID == "" {
		msg.SessionID = DefaultSessionID
	}
	if msg.Type == "" {
		msg.Type = "user"
	}

	status := http.StatusAccepted
	if s.store != nil {
		stored, err := s.store.AppendMessage(msg)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to persist message")
			return
		}
		msg = stored
		status = http.StatusCreated
	}

	// Answer the client as soon as the message is durable (or validated, when
	// persistence is off), then hand it to the worker pool. Dispatch can block
	// under backpressure; doing it after the response is flushed means a slow
	// swarm cannot make a client time out and retry an already-stored message.
	resp := ingestResponse{Status: "accepted", Message: msg}
	if s.lexicon != nil {
		sum := s.lexicon.Summarize(msg.Message)
		resp.Listener = &sum
	}
	writeJSON(w, status, resp)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	s.msgChan <- msg
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"persistence": s.store != nil,
	})
}

func (s *Server) requireStore(w http.ResponseWriter) bool {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "persistence is disabled on this server")
		return false
	}
	return true
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	sessions, err := s.store.ListSessions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

// queryInt parses an optional non-negative integer query parameter.
// ok is false when the value is present but invalid.
func queryInt(r *http.Request, name string, def int) (val int, ok bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// handleListMessages returns a session's messages as a JSON array in the
// Antigravity chat-log shape, so the response is directly re-ingestable by
// parser.ParseChatLog and the tail command. Optional query parameters:
// after=<messageId> returns only newer messages (for incremental polling)
// and limit=<n> caps the count (at most MaxListLimit).
func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	sessionID := r.PathValue("sessionId")

	after, ok := queryInt(r, "after", -1)
	if !ok {
		writeError(w, http.StatusBadRequest, "after must be a non-negative integer")
		return
	}
	limit, ok := queryInt(r, "limit", 0)
	if !ok {
		writeError(w, http.StatusBadRequest, "limit must be a non-negative integer")
		return
	}
	if limit == 0 || limit > MaxListLimit {
		limit = MaxListLimit
	}

	filter := storage.MessageFilter{User: r.URL.Query().Get("user"), Project: r.URL.Query().Get("project")}
	for _, p := range []struct {
		name string
		dst  *time.Time
	}{{"since", &filter.Since}, {"until", &filter.Until}} {
		raw := r.URL.Query().Get(p.name)
		if raw == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, p.name+" must be an RFC 3339 timestamp")
			return
		}
		*p.dst = t
	}
	if !filter.Since.IsZero() && !filter.Until.IsZero() && !filter.Until.After(filter.Since) {
		writeError(w, http.StatusBadRequest, "until must be after since")
		return
	}

	messages, err := s.store.ListMessagesFiltered(sessionID, after, limit, filter)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list messages")
		return
	}
	writeJSON(w, http.StatusOK, messages)
}

// handleAnalyze previews the listener's flags for a draft message without
// storing or dispatching it. Body: {"message": "..."}. 503 without a lexicon.
func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	if s.lexicon == nil {
		writeError(w, http.StatusServiceUnavailable, "listener is not configured (start the server with a lexicon)")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxIngestBodyBytes)
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "body must be JSON with a message field")
		return
	}
	writeJSON(w, http.StatusOK, s.lexicon.Summarize(req.Message))
}

// handleListMentions summarizes the #tags and @users referenced across all
// sessions. Optional query parameter kind=tag|user restricts the list.
func (s *Server) handleListMentions(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	var kind parser.MentionKind
	if raw := r.URL.Query().Get("kind"); raw != "" {
		k, ok := parser.ParseMentionKind(raw)
		if !ok {
			writeError(w, http.StatusBadRequest, "kind must be tag or user")
			return
		}
		kind = k
	}
	mentions, err := s.store.ListMentions(kind)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list mentions")
		return
	}
	writeJSON(w, http.StatusOK, mentions)
}

// handleMentionMessages returns the messages that reference one #tag or
// @user, oldest first, in the chat-log shape. {kind} is tag or user; the
// name is matched case-insensitively. limit=<n> caps the count (at most
// MaxListLimit). An entity nobody mentioned yields an empty array.
func (s *Server) handleMentionMessages(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	kind, ok := parser.ParseMentionKind(r.PathValue("kind"))
	if !ok {
		writeError(w, http.StatusBadRequest, "kind must be tag or user")
		return
	}
	limit, ok := queryInt(r, "limit", 0)
	if !ok {
		writeError(w, http.StatusBadRequest, "limit must be a non-negative integer")
		return
	}
	if limit == 0 || limit > MaxListLimit {
		limit = MaxListLimit
	}
	messages, err := s.store.MessagesMentioning(kind, r.PathValue("name"), limit)
	if errors.Is(err, storage.ErrInvalidMention) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list messages")
		return
	}
	writeJSON(w, http.StatusOK, messages)
}

// pathMessageID parses the {messageId} path value, writing a 400 on failure.
func pathMessageID(w http.ResponseWriter, r *http.Request) (int, bool) {
	messageID, err := strconv.Atoi(r.PathValue("messageId"))
	if err != nil || messageID < 0 {
		writeError(w, http.StatusBadRequest, "messageId must be a non-negative integer")
		return 0, false
	}
	return messageID, true
}

func (s *Server) handleGetMessage(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	sessionID := r.PathValue("sessionId")
	messageID, ok := pathMessageID(w, r)
	if !ok {
		return
	}
	msg, err := s.store.GetMessage(sessionID, messageID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "message not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get message")
		return
	}
	writeJSON(w, http.StatusOK, msg)
}

func (s *Server) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	sessionID := r.PathValue("sessionId")
	messageID, ok := pathMessageID(w, r)
	if !ok {
		return
	}
	err := s.store.DeleteMessage(sessionID, messageID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "message not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete message")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	err := s.store.DeleteSession(r.PathValue("sessionId"))
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete session")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
