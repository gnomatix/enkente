package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gnomatix/enkente/pkg/curation"
	"github.com/gnomatix/enkente/pkg/extract"
	"github.com/gnomatix/enkente/pkg/graph"
	"github.com/gnomatix/enkente/pkg/listener"
	"github.com/gnomatix/enkente/pkg/namespace"
	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/sentiment"
	"github.com/gnomatix/enkente/pkg/storage"
	"github.com/gnomatix/enkente/pkg/summarize"
	"github.com/gnomatix/enkente/pkg/topic"
)

// IngestRequest represents a message submitted via the REST API.
type IngestRequest struct {
	Type    string `json:"type"`
	User    string `json:"user,omitempty"`
	Message string `json:"message"`
	Session string `json:"session,omitempty"`
}

// Server manages the HTTP ingestion endpoint and dispatches messages to a handler.
type Server struct {
	port    int
	handler func(workerID int, msg parser.AntigravityMessage)
	msgChan chan parser.AntigravityMessage
	store   *storage.BoltStorage
	graph   *graph.ConceptGraph

	// Round 3 subsystems
	sentimentTracker *sentiment.Tracker
	topicEngine      *topic.Engine
	listener         *listener.Listener
	nsRegistry       *namespace.Registry
	summarizer       *summarize.Summarizer
	curator          *curation.Curator
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

	// Initialize round 3 subsystems
	if g != nil {
		s.sentimentTracker = sentiment.NewTracker()
		s.topicEngine = topic.NewEngine()
		s.listener = listener.NewListener(listener.Passive)
		s.nsRegistry = namespace.NewRegistry()
		s.summarizer = summarize.NewSummarizer(s.sentimentTracker)
		s.curator = curation.NewCurator(g, store)
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

// processMessage persists the message to BoltDB, runs concept extraction,
// sentiment analysis, listener checks, topic tracking, and namespace binding.
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
	var conceptIDs []string
	for _, c := range result.Concepts {
		_ = s.store.PutJSON(storage.ConceptBucket, c.ID, c)
		_ = s.store.AddToIndex(storage.ConceptsBySessionBucket, msg.SessionID, c.ID)
		conceptIDs = append(conceptIDs, c.ID)
	}
	for _, e := range result.Edges {
		_ = s.store.PutJSON(storage.EdgeBucket, e.ID, e)
		_ = s.store.AddToIndex(storage.EdgesBySourceBucket, e.Source, e.ID)
		_ = s.store.AddToIndex(storage.EdgesByTargetBucket, e.Target, e.ID)
	}

	// Sentiment analysis
	if s.sentimentTracker != nil {
		analysis := sentiment.Analyze(msg)
		s.sentimentTracker.RecordForConcepts(conceptIDs, analysis)
	}

	// Topic clustering
	if s.topicEngine != nil {
		s.topicEngine.IngestMessage(key, msg.Message, msg.SessionID, conceptIDs)
		// Re-cluster periodically (every 10 messages is a simple heuristic)
		stats := s.graph.Stats()
		if stats.NumConcepts > 0 && stats.NumConcepts%10 == 0 {
			s.topicEngine.ReclusterAll(0.3)
		}
	}

	// Listener analysis
	if s.listener != nil {
		s.listener.AnalyzeMessage(msg.Message, msg.User, msg.SessionID)
	}

	// Namespace binding
	if s.nsRegistry != nil {
		s.nsRegistry.AutoBindFromExtraction(result.Concepts)
	}
}

// Start begins listening for HTTP requests.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ingest", s.handleIngest)
	mux.HandleFunc("/health", s.handleHealth)

	// Graph query endpoints (round 2)
	mux.HandleFunc("GET /graph/concepts", s.handleGetConcepts)
	mux.HandleFunc("GET /graph/concepts/{id}", s.handleGetConcept)
	mux.HandleFunc("GET /graph/concepts/{id}/neighbors", s.handleGetNeighbors)
	mux.HandleFunc("GET /graph/edges", s.handleGetEdges)
	mux.HandleFunc("GET /graph/subgraph", s.handleSubgraph)
	mux.HandleFunc("GET /graph/stats", s.handleGraphStats)
	mux.HandleFunc("GET /sessions/{id}/graph", s.handleSessionGraph)

	// Sentiment endpoints (round 3)
	mux.HandleFunc("GET /sentiment/concepts", s.handleGetSentiments)
	mux.HandleFunc("GET /sentiment/concepts/{id}", s.handleGetConceptSentiment)

	// Topic cluster endpoints (round 3)
	mux.HandleFunc("GET /topics/clusters", s.handleGetClusters)
	mux.HandleFunc("GET /topics/clusters/{id}", s.handleGetCluster)
	mux.HandleFunc("POST /topics/recluster", s.handleRecluster)

	// Listener endpoints (round 3)
	mux.HandleFunc("GET /listener/flags", s.handleGetListenerFlags)
	mux.HandleFunc("POST /listener/flags/{id}/resolve", s.handleResolveFlag)
	mux.HandleFunc("GET /listener/terms", s.handleGetTermProfiles)
	mux.HandleFunc("GET /listener/terms/{term}", s.handleGetTermProfile)
	mux.HandleFunc("POST /listener/definitions", s.handleRegisterDefinition)
	mux.HandleFunc("POST /listener/synonyms", s.handleRegisterSynonym)
	mux.HandleFunc("GET /listener/mode", s.handleGetListenerMode)
	mux.HandleFunc("PUT /listener/mode", s.handleSetListenerMode)

	// Namespace endpoints (round 3)
	mux.HandleFunc("GET /namespaces", s.handleGetNamespaces)
	mux.HandleFunc("POST /namespaces", s.handleCreateNamespace)
	mux.HandleFunc("GET /namespaces/{id}/concepts", s.handleGetNamespaceConcepts)
	mux.HandleFunc("GET /namespaces/{id}/graph", s.handleGetNamespaceGraph)
	mux.HandleFunc("POST /namespaces/{id}/bind", s.handleBindToNamespace)

	// Summarization endpoints (round 3)
	mux.HandleFunc("GET /summarize/branch/{id}", s.handleSummarizeBranch)
	mux.HandleFunc("GET /summarize/session/{id}", s.handleSummarizeSession)

	// Curation endpoints -- two-way read-write (round 3)
	mux.HandleFunc("POST /curate/concepts", s.handleCreateConcept)
	mux.HandleFunc("PUT /curate/concepts/{id}", s.handleUpdateConcept)
	mux.HandleFunc("DELETE /curate/concepts/{id}", s.handleDeleteConcept)
	mux.HandleFunc("POST /curate/edges", s.handleCreateEdge)
	mux.HandleFunc("PUT /curate/edges/{id}", s.handleUpdateEdge)
	mux.HandleFunc("DELETE /curate/edges/{id}", s.handleDeleteEdge)
	mux.HandleFunc("POST /curate/merge", s.handleMergeConcepts)
	mux.HandleFunc("GET /curate/search", s.handleSearchConcepts)

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

	session := req.Session
	if session == "" {
		session = "live"
	}

	msg := parser.AntigravityMessage{
		SessionID: session,
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

// --- Graph query handlers (round 2) ---

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

// --- Sentiment handlers (round 3) ---

func (s *Server) handleGetSentiments(w http.ResponseWriter, r *http.Request) {
	if s.sentimentTracker == nil {
		http.Error(w, "Sentiment tracker not initialized", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.sentimentTracker.AllConceptSentiments())
}

func (s *Server) handleGetConceptSentiment(w http.ResponseWriter, r *http.Request) {
	if s.sentimentTracker == nil {
		http.Error(w, "Sentiment tracker not initialized", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	cs := s.sentimentTracker.GetConceptSentiment(id)
	if cs == nil {
		http.Error(w, "No sentiment data for concept", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cs)
}

// --- Topic cluster handlers (round 3) ---

func (s *Server) handleGetClusters(w http.ResponseWriter, r *http.Request) {
	if s.topicEngine == nil {
		http.Error(w, "Topic engine not initialized", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.topicEngine.GetClusters())
}

func (s *Server) handleGetCluster(w http.ResponseWriter, r *http.Request) {
	if s.topicEngine == nil {
		http.Error(w, "Topic engine not initialized", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	cluster := s.topicEngine.GetCluster(id)
	if cluster == nil {
		http.Error(w, "Cluster not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cluster)
}

func (s *Server) handleRecluster(w http.ResponseWriter, r *http.Request) {
	if s.topicEngine == nil {
		http.Error(w, "Topic engine not initialized", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		MinSimilarity float64 `json:"min_similarity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.MinSimilarity = 0.3
	}
	if req.MinSimilarity <= 0 {
		req.MinSimilarity = 0.3
	}

	s.topicEngine.ReclusterAll(req.MinSimilarity)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":   "reclustered",
		"clusters": len(s.topicEngine.GetClusters()),
	})
}

// --- Listener handlers (round 3) ---

func (s *Server) handleGetListenerFlags(w http.ResponseWriter, r *http.Request) {
	if s.listener == nil {
		http.Error(w, "Listener not initialized", http.StatusServiceUnavailable)
		return
	}
	includeResolved := r.URL.Query().Get("include_resolved") == "true"
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.listener.GetFlags(includeResolved))
}

func (s *Server) handleResolveFlag(w http.ResponseWriter, r *http.Request) {
	if s.listener == nil {
		http.Error(w, "Listener not initialized", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	var req struct {
		Resolution string `json:"resolution"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	flagID := fmt.Sprintf("flag:%s", id)
	if err := s.listener.ResolveFlag(flagID, req.Resolution); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "resolved"})
}

func (s *Server) handleGetTermProfiles(w http.ResponseWriter, r *http.Request) {
	if s.listener == nil {
		http.Error(w, "Listener not initialized", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.listener.AllTermProfiles())
}

func (s *Server) handleGetTermProfile(w http.ResponseWriter, r *http.Request) {
	if s.listener == nil {
		http.Error(w, "Listener not initialized", http.StatusServiceUnavailable)
		return
	}
	term := r.PathValue("term")
	profile := s.listener.GetTermProfile(term)
	if profile == nil {
		http.Error(w, "Term not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile)
}

func (s *Server) handleRegisterDefinition(w http.ResponseWriter, r *http.Request) {
	if s.listener == nil {
		http.Error(w, "Listener not initialized", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Term    string `json:"term"`
		Meaning string `json:"meaning"`
		Source  string `json:"source"`
		Context string `json:"context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	s.listener.RegisterDefinition(req.Term, req.Meaning, req.Source, req.Context)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
}

func (s *Server) handleRegisterSynonym(w http.ResponseWriter, r *http.Request) {
	if s.listener == nil {
		http.Error(w, "Listener not initialized", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Term1 string `json:"term1"`
		Term2 string `json:"term2"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	s.listener.RegisterSynonym(req.Term1, req.Term2)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
}

func (s *Server) handleGetListenerMode(w http.ResponseWriter, r *http.Request) {
	if s.listener == nil {
		http.Error(w, "Listener not initialized", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"mode": string(s.listener.GetMode())})
}

func (s *Server) handleSetListenerMode(w http.ResponseWriter, r *http.Request) {
	if s.listener == nil {
		http.Error(w, "Listener not initialized", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	switch listener.Mode(req.Mode) {
	case listener.Active, listener.Passive:
		s.listener.SetMode(listener.Mode(req.Mode))
	default:
		http.Error(w, "Invalid mode. Use 'active' or 'passive'", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated", "mode": req.Mode})
}

// --- Namespace handlers (round 3) ---

func (s *Server) handleGetNamespaces(w http.ResponseWriter, r *http.Request) {
	if s.nsRegistry == nil {
		http.Error(w, "Namespace registry not initialized", http.StatusServiceUnavailable)
		return
	}
	dim := namespace.Dimension(r.URL.Query().Get("dimension"))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.nsRegistry.ListNamespaces(dim))
}

func (s *Server) handleCreateNamespace(w http.ResponseWriter, r *http.Request) {
	if s.nsRegistry == nil {
		http.Error(w, "Namespace registry not initialized", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Dimension string `json:"dimension"`
		Label     string `json:"label"`
		Parent    string `json:"parent,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	ns := s.nsRegistry.CreateNamespace(namespace.Dimension(req.Dimension), req.Label, req.Parent)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ns)
}

func (s *Server) handleGetNamespaceConcepts(w http.ResponseWriter, r *http.Request) {
	if s.nsRegistry == nil {
		http.Error(w, "Namespace registry not initialized", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	conceptIDs := s.nsRegistry.NamespaceConcepts(id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(conceptIDs)
}

func (s *Server) handleGetNamespaceGraph(w http.ResponseWriter, r *http.Request) {
	if s.nsRegistry == nil || s.graph == nil {
		http.Error(w, "Not initialized", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	concepts, edges := s.nsRegistry.FilterByNamespace(s.graph, id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"namespace": id,
		"concepts":  concepts,
		"edges":     edges,
	})
}

func (s *Server) handleBindToNamespace(w http.ResponseWriter, r *http.Request) {
	if s.nsRegistry == nil {
		http.Error(w, "Namespace registry not initialized", http.StatusServiceUnavailable)
		return
	}
	nsID := r.PathValue("id")
	var req struct {
		ConceptID string  `json:"concept_id"`
		Strength  float64 `json:"strength"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Strength <= 0 {
		req.Strength = 1.0
	}
	if err := s.nsRegistry.Bind(req.ConceptID, nsID, req.Strength); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "bound"})
}

// --- Summarization handlers (round 3) ---

func (s *Server) handleSummarizeBranch(w http.ResponseWriter, r *http.Request) {
	if s.summarizer == nil || s.graph == nil {
		http.Error(w, "Summarizer not initialized", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	depth := 2
	if d := r.URL.Query().Get("depth"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			depth = parsed
		}
	}
	summary := s.summarizer.SummarizeBranch(s.graph, id, depth)
	if summary == nil {
		http.Error(w, "Concept not found or no data", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

func (s *Server) handleSummarizeSession(w http.ResponseWriter, r *http.Request) {
	if s.summarizer == nil || s.graph == nil || s.store == nil {
		http.Error(w, "Summarizer not initialized", http.StatusServiceUnavailable)
		return
	}
	sessionID := r.PathValue("id")
	conceptIDs, err := s.store.GetIndex(storage.ConceptsBySessionBucket, sessionID)
	if err != nil {
		http.Error(w, "Failed to read session index", http.StatusInternalServerError)
		return
	}
	summary := s.summarizer.SummarizeSession(s.graph, sessionID, conceptIDs)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// --- Curation handlers -- two-way read-write (round 3) ---

func (s *Server) handleCreateConcept(w http.ResponseWriter, r *http.Request) {
	if s.curator == nil {
		http.Error(w, "Curator not initialized", http.StatusServiceUnavailable)
		return
	}
	var req curation.NewConceptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	concept, err := s.curator.CreateConcept(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(concept)
}

func (s *Server) handleUpdateConcept(w http.ResponseWriter, r *http.Request) {
	if s.curator == nil {
		http.Error(w, "Curator not initialized", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	var update curation.ConceptUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	concept, err := s.curator.UpdateConcept(id, update)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(concept)
}

func (s *Server) handleDeleteConcept(w http.ResponseWriter, r *http.Request) {
	if s.curator == nil {
		http.Error(w, "Curator not initialized", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if err := s.curator.DeleteConcept(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

func (s *Server) handleCreateEdge(w http.ResponseWriter, r *http.Request) {
	if s.curator == nil {
		http.Error(w, "Curator not initialized", http.StatusServiceUnavailable)
		return
	}
	var req curation.NewEdgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	edge, err := s.curator.CreateEdge(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(edge)
}

func (s *Server) handleUpdateEdge(w http.ResponseWriter, r *http.Request) {
	if s.curator == nil {
		http.Error(w, "Curator not initialized", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	var update curation.EdgeUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	edge, err := s.curator.UpdateEdge(id, update)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(edge)
}

func (s *Server) handleDeleteEdge(w http.ResponseWriter, r *http.Request) {
	if s.curator == nil {
		http.Error(w, "Curator not initialized", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if err := s.curator.DeleteEdge(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

func (s *Server) handleMergeConcepts(w http.ResponseWriter, r *http.Request) {
	if s.curator == nil {
		http.Error(w, "Curator not initialized", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		ConceptA string `json:"concept_a"`
		ConceptB string `json:"concept_b"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	merged, err := s.curator.MergeConcepts(req.ConceptA, req.ConceptB)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(merged)
}

func (s *Server) handleSearchConcepts(w http.ResponseWriter, r *http.Request) {
	if s.curator == nil {
		http.Error(w, "Curator not initialized", http.StatusServiceUnavailable)
		return
	}
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "q parameter required", http.StatusBadRequest)
		return
	}
	results := s.curator.SearchConcepts(query)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}
