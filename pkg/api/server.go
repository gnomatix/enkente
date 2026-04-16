package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gnomatix/enkente/pkg/extract"
	"github.com/gnomatix/enkente/pkg/graph"
	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/storage"
)

// IngestRequest represents a message submitted via the REST API.
type IngestRequest struct {
	Type    string `json:"type"`
	User    string `json:"user,omitempty"`
	Message string `json:"message"`
}

// Server manages the HTTP ingestion endpoint and dispatches messages to a handler.
type Server struct {
	port    int
	handler func(workerID int, msg parser.AntigravityMessage)
	msgChan chan parser.AntigravityMessage
	store   *storage.BoltStorage
	graph   *graph.ConceptGraph
}

// NewServer creates a new ingestion server on the given port.
// numWorkers goroutines will concurrently consume messages using the handler.
// store and g may be nil for backwards compatibility (no persistence/graph).
func NewServer(port int, numWorkers int, handler func(workerID int, msg parser.AntigravityMessage), store *storage.BoltStorage, g *graph.ConceptGraph) *Server {
	s := &Server{
		port:    port,
		handler: handler,
		msgChan: make(chan parser.AntigravityMessage, 100),
		store:   store,
		graph:   g,
	}

	// Spawn worker swarm
	for i := 0; i < numWorkers; i++ {
		workerID := i
		go func() {
			for msg := range s.msgChan {
				// Persist message and extract concepts
				s.processMessage(msg)
				handler(workerID, msg)
			}
		}()
	}

	return s
}

// processMessage persists the message to BoltDB and runs concept extraction.
func (s *Server) processMessage(msg parser.AntigravityMessage) {
	if s.store == nil {
		return
	}

	// Persist the raw message
	key := fmt.Sprintf("%s:%d:%d", msg.SessionID, msg.Timestamp.UnixNano(), msg.MessageID)
	_ = s.store.PutJSON(storage.ChatBucket, key, msg)

	if s.graph == nil {
		return
	}

	// Extract concepts and edges
	result := extract.FromMessage(msg)
	extract.ApplyResult(s.graph, result)

	// Persist concepts and edges to BoltDB
	for _, c := range result.Concepts {
		_ = s.store.PutJSON(storage.ConceptBucket, c.ID, c)
		_ = s.store.AddToIndex(storage.ConceptsBySessionBucket, msg.SessionID, c.ID)
	}
	for _, e := range result.Edges {
		_ = s.store.PutJSON(storage.EdgeBucket, e.ID, e)
		_ = s.store.AddToIndex(storage.EdgesBySourceBucket, e.Source, e.ID)
		_ = s.store.AddToIndex(storage.EdgesByTargetBucket, e.Target, e.ID)
	}
}

// Start begins listening for HTTP requests.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ingest", s.handleIngest)
	mux.HandleFunc("/health", s.handleHealth)

	// Graph query endpoints
	mux.HandleFunc("GET /graph/concepts", s.handleGetConcepts)
	mux.HandleFunc("GET /graph/concepts/{id}", s.handleGetConcept)
	mux.HandleFunc("GET /graph/concepts/{id}/neighbors", s.handleGetNeighbors)
	mux.HandleFunc("GET /graph/edges", s.handleGetEdges)
	mux.HandleFunc("GET /graph/subgraph", s.handleSubgraph)
	mux.HandleFunc("GET /graph/stats", s.handleGraphStats)
	mux.HandleFunc("GET /sessions/{id}/graph", s.handleSessionGraph)

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
		User:      req.User,
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

// --- Graph query handlers ---

func (s *Server) handleGetConcepts(w http.ResponseWriter, r *http.Request) {
	if s.graph == nil {
		http.Error(w, "Graph not initialized", http.StatusServiceUnavailable)
		return
	}

	typeFilter := r.URL.Query().Get("type")
	var concepts []*graph.Concept
	if typeFilter != "" {
		concepts = s.graph.GetByType(graph.ConceptType(typeFilter))
	} else {
		concepts = s.graph.AllConcepts()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(concepts)
}

func (s *Server) handleGetConcept(w http.ResponseWriter, r *http.Request) {
	if s.graph == nil {
		http.Error(w, "Graph not initialized", http.StatusServiceUnavailable)
		return
	}

	id := r.PathValue("id")
	c := s.graph.GetConcept(id)
	if c == nil {
		http.Error(w, "Concept not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(c)
}

func (s *Server) handleGetNeighbors(w http.ResponseWriter, r *http.Request) {
	if s.graph == nil {
		http.Error(w, "Graph not initialized", http.StatusServiceUnavailable)
		return
	}

	id := r.PathValue("id")
	neighbors := s.graph.GetNeighbors(id)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(neighbors)
}

func (s *Server) handleGetEdges(w http.ResponseWriter, r *http.Request) {
	if s.graph == nil {
		http.Error(w, "Graph not initialized", http.StatusServiceUnavailable)
		return
	}

	conceptID := r.URL.Query().Get("concept")
	var edges []*graph.Edge
	if conceptID != "" {
		edges = s.graph.EdgesFor(conceptID)
	} else {
		edges = s.graph.AllEdges()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(edges)
}

func (s *Server) handleSubgraph(w http.ResponseWriter, r *http.Request) {
	if s.graph == nil {
		http.Error(w, "Graph not initialized", http.StatusServiceUnavailable)
		return
	}

	root := r.URL.Query().Get("root")
	if root == "" {
		http.Error(w, "root parameter required", http.StatusBadRequest)
		return
	}

	depth := 2 // default
	if d := r.URL.Query().Get("depth"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			depth = parsed
		}
	}

	concepts, edges := s.graph.SubgraphFor(root, depth)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"concepts": concepts,
		"edges":    edges,
	})
}

func (s *Server) handleGraphStats(w http.ResponseWriter, r *http.Request) {
	if s.graph == nil {
		http.Error(w, "Graph not initialized", http.StatusServiceUnavailable)
		return
	}

	stats := s.graph.Stats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleSessionGraph(w http.ResponseWriter, r *http.Request) {
	if s.graph == nil || s.store == nil {
		http.Error(w, "Graph not initialized", http.StatusServiceUnavailable)
		return
	}

	sessionID := r.PathValue("id")

	// Get concept IDs for this session from the index
	conceptIDs, err := s.store.GetIndex(storage.ConceptsBySessionBucket, sessionID)
	if err != nil {
		http.Error(w, "Failed to read session index", http.StatusInternalServerError)
		return
	}

	var concepts []*graph.Concept
	edgeSet := make(map[string]bool)
	var edges []*graph.Edge

	for _, cid := range conceptIDs {
		if c := s.graph.GetConcept(cid); c != nil {
			concepts = append(concepts, c)
			// Collect edges for these concepts
			for _, e := range s.graph.EdgesFor(cid) {
				if !edgeSet[e.ID] {
					edgeSet[e.ID] = true
					edges = append(edges, e)
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"session":  sessionID,
		"concepts": concepts,
		"edges":    edges,
	})
}
