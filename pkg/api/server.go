package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gnomatix/enkente/pkg/parser"
)

// IngestRequest represents a message submitted via the REST API.
type IngestRequest struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Server manages the HTTP ingestion endpoint and dispatches messages to a handler.
type Server struct {
	port    int
	handler func(workerID int, msg parser.AntigravityMessage)
	msgChan chan parser.AntigravityMessage
}

// NewServer creates a new ingestion server on the given port.
// numWorkers goroutines will concurrently consume messages using the handler.
func NewServer(port int, numWorkers int, handler func(workerID int, msg parser.AntigravityMessage)) *Server {
	s := &Server{
		port:    port,
		handler: handler,
		msgChan: make(chan parser.AntigravityMessage, 100),
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

// Start begins listening for HTTP requests.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ingest", s.handleIngest)
	mux.HandleFunc("/health", s.handleHealth)

	addr := fmt.Sprintf(":%d", s.port)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req IngestRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	msg := parser.AntigravityMessage{
		SessionID: "live",
		MessageID: 0,
		Type:      req.Type,
		Message:   req.Message,
		Timestamp: time.Now(),
	}

	s.msgChan <- msg

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
