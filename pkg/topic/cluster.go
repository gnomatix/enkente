package topic

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gnomatix/enkente/pkg/graph"
)

// Cluster represents a group of related concepts that form an emerging theme.
type Cluster struct {
	ID          string            `json:"id"`
	Label       string            `json:"label"`       // auto-generated or curated label
	ConceptIDs  []string          `json:"concept_ids"` // member concepts
	Centroid    map[string]float64 `json:"centroid"`    // term frequency vector (TF centroid)
	Score       float64           `json:"score"`       // cluster cohesion score
	MessageCount int              `json:"message_count"`
	Created     time.Time         `json:"created"`
	Modified    time.Time         `json:"modified"`
	Session     string            `json:"session,omitempty"`
	Properties  map[string]any    `json:"properties,omitempty"`
}

// Engine manages topic extraction and clustering from the concept graph.
type Engine struct {
	mu       sync.RWMutex
	clusters map[string]*Cluster

	// Term frequency tracking: term -> count of messages containing it
	termDocFreq map[string]int
	totalDocs   int

	// Message-to-terms mapping for clustering
	messageTerms map[string]map[string]float64 // messageKey -> term -> tf
	messageToConceptIDs map[string][]string      // messageKey -> concept IDs from that message
}

// NewEngine creates a topic clustering engine.
func NewEngine() *Engine {
	return &Engine{
		clusters:            make(map[string]*Cluster),
		termDocFreq:         make(map[string]int),
		messageTerms:        make(map[string]map[string]float64),
		messageToConceptIDs: make(map[string][]string),
	}
}

// IngestMessage processes a message's text and associated concept IDs
// to update term frequency vectors and trigger re-clustering.
func (e *Engine) IngestMessage(messageKey, text, session string, conceptIDs []string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Tokenize and compute term frequencies
	terms := tokenize(text)
	tf := make(map[string]float64)
	for _, t := range terms {
		tf[t]++
	}
	// Normalize by message length
	for t := range tf {
		tf[t] /= float64(len(terms))
	}

	// Track document frequency
	seen := make(map[string]bool)
	for _, t := range terms {
		if !seen[t] {
			seen[t] = true
			e.termDocFreq[t]++
		}
	}
	e.totalDocs++

	e.messageTerms[messageKey] = tf
	e.messageToConceptIDs[messageKey] = conceptIDs
}

// ReclusterAll performs a full re-clustering of all ingested messages.
// Uses a simple agglomerative approach based on cosine similarity of TF-IDF vectors.
func (e *Engine) ReclusterAll(minSimilarity float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.messageTerms) == 0 {
		return
	}

	// Compute TF-IDF vectors for each message
	type docVec struct {
		key       string
		tfidf     map[string]float64
		conceptIDs []string
	}

	var docs []docVec
	for key, tf := range e.messageTerms {
		tfidf := make(map[string]float64)
		for term, freq := range tf {
			df := e.termDocFreq[term]
			if df > 0 {
				idf := math.Log(float64(e.totalDocs)/float64(df)) + 1
				tfidf[term] = freq * idf
			}
		}
		docs = append(docs, docVec{
			key:       key,
			tfidf:     tfidf,
			conceptIDs: e.messageToConceptIDs[key],
		})
	}

	// Single-link agglomerative clustering
	// Start with each document in its own cluster
	type protoCluster struct {
		members []int              // indices into docs
		centroid map[string]float64
	}

	clusters := make([]*protoCluster, len(docs))
	for i, doc := range docs {
		centroid := make(map[string]float64)
		for k, v := range doc.tfidf {
			centroid[k] = v
		}
		clusters[i] = &protoCluster{
			members:  []int{i},
			centroid: centroid,
		}
	}

	// Iteratively merge most similar clusters
	for {
		if len(clusters) <= 1 {
			break
		}

		bestSim := -1.0
		bestI, bestJ := -1, -1

		for i := 0; i < len(clusters); i++ {
			for j := i + 1; j < len(clusters); j++ {
				sim := cosineSimilarity(clusters[i].centroid, clusters[j].centroid)
				if sim > bestSim {
					bestSim = sim
					bestI = i
					bestJ = j
				}
			}
		}

		if bestSim < minSimilarity {
			break
		}

		// Merge bestJ into bestI
		merged := &protoCluster{
			members:  append(clusters[bestI].members, clusters[bestJ].members...),
			centroid: mergeCentroids(clusters[bestI].centroid, len(clusters[bestI].members),
				clusters[bestJ].centroid, len(clusters[bestJ].members)),
		}
		clusters[bestI] = merged
		clusters = append(clusters[:bestJ], clusters[bestJ+1:]...)
	}

	// Convert proto-clusters to Cluster objects
	now := time.Now()
	newClusters := make(map[string]*Cluster)

	for i, pc := range clusters {
		if len(pc.members) < 2 {
			continue // skip singleton clusters
		}

		// Collect all concept IDs
		conceptSet := make(map[string]bool)
		for _, mi := range pc.members {
			for _, cid := range docs[mi].conceptIDs {
				conceptSet[cid] = true
			}
		}
		var conceptIDs []string
		for cid := range conceptSet {
			conceptIDs = append(conceptIDs, cid)
		}
		sort.Strings(conceptIDs)

		// Generate label from top terms
		label := topTermsLabel(pc.centroid, 3)

		clusterID := fmt.Sprintf("cluster:%d", i)
		newClusters[clusterID] = &Cluster{
			ID:           clusterID,
			Label:        label,
			ConceptIDs:   conceptIDs,
			Centroid:     pc.centroid,
			Score:        clusterCohesion(pc, docs),
			MessageCount: len(pc.members),
			Created:      now,
			Modified:     now,
		}
	}

	e.clusters = newClusters
}

// GetClusters returns all current clusters.
func (e *Engine) GetClusters() []*Cluster {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*Cluster, 0, len(e.clusters))
	for _, c := range e.clusters {
		result = append(result, c)
	}
	return result
}

// GetCluster returns a specific cluster by ID.
func (e *Engine) GetCluster(id string) *Cluster {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.clusters[id]
}

// ClustersForConcept returns all clusters containing the given concept ID.
func (e *Engine) ClustersForConcept(conceptID string) []*Cluster {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var result []*Cluster
	for _, c := range e.clusters {
		for _, cid := range c.ConceptIDs {
			if cid == conceptID {
				result = append(result, c)
				break
			}
		}
	}
	return result
}

// ApplyToGraph creates cluster concepts and edges in the graph.
func (e *Engine) ApplyToGraph(g *graph.ConceptGraph) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	now := time.Now()
	for _, cluster := range e.clusters {
		// Add cluster as a topic concept
		g.AddConcept(&graph.Concept{
			ID:    cluster.ID,
			Type:  graph.TopicType,
			Label: cluster.Label,
			Provenance: graph.Provenance{
				Source:   "clustering",
				Created:  now,
				Modified: now,
				Count:    cluster.MessageCount,
			},
			Properties: map[string]any{
				"cluster_score":    cluster.Score,
				"message_count":    cluster.MessageCount,
				"member_count":     len(cluster.ConceptIDs),
			},
		})

		// Add edges from cluster to member concepts
		for _, cid := range cluster.ConceptIDs {
			edgeID := graph.EdgeKey(cluster.ID, cid, graph.RelatedTo)
			g.AddEdge(&graph.Edge{
				ID:       edgeID,
				Source:   cluster.ID,
				Target:   cid,
				Type:     graph.RelatedTo,
				Weight:   1,
				Created:  now,
				Modified: now,
			})
		}
	}
}

// --- Helper functions ---

// tokenize splits text into lowercase, cleaned tokens, removing stop words.
func tokenize(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	var tokens []string
	for _, w := range words {
		clean := strings.Trim(w, ".,!?;:'\"()-[]{}@#")
		if len(clean) < 2 || stopWords[clean] {
			continue
		}
		tokens = append(tokens, clean)
	}
	return tokens
}

// cosineSimilarity computes cosine similarity between two sparse vectors.
func cosineSimilarity(a, b map[string]float64) float64 {
	var dot, normA, normB float64
	for k, v := range a {
		dot += v * b[k]
		normA += v * v
	}
	for _, v := range b {
		normB += v * v
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// mergeCentroids computes a weighted average of two centroids.
func mergeCentroids(a map[string]float64, countA int, b map[string]float64, countB int) map[string]float64 {
	total := float64(countA + countB)
	result := make(map[string]float64)
	for k, v := range a {
		result[k] += v * float64(countA) / total
	}
	for k, v := range b {
		result[k] += v * float64(countB) / total
	}
	return result
}

// topTermsLabel generates a label from the top N terms in a centroid.
func topTermsLabel(centroid map[string]float64, n int) string {
	type termScore struct {
		term  string
		score float64
	}
	var ts []termScore
	for t, s := range centroid {
		ts = append(ts, termScore{t, s})
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i].score > ts[j].score })

	var parts []string
	for i := 0; i < n && i < len(ts); i++ {
		parts = append(parts, ts[i].term)
	}
	if len(parts) == 0 {
		return "unnamed"
	}
	return strings.Join(parts, ", ")
}

// clusterCohesion computes the average pairwise similarity within a cluster.
func clusterCohesion(pc *protoCluster, docs []docVec) float64 {
	if len(pc.members) < 2 {
		return 1.0
	}
	var totalSim float64
	var pairs int
	for i := 0; i < len(pc.members); i++ {
		for j := i + 1; j < len(pc.members); j++ {
			totalSim += cosineSimilarity(docs[pc.members[i]].tfidf, docs[pc.members[j]].tfidf)
			pairs++
		}
	}
	if pairs == 0 {
		return 0
	}
	return totalSim / float64(pairs)
}

// stopWords is a basic set of English stop words to exclude from term vectors.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "shall": true, "can": true, "to": true,
	"of": true, "in": true, "for": true, "on": true, "with": true,
	"at": true, "by": true, "from": true, "up": true, "about": true,
	"into": true, "through": true, "during": true, "before": true,
	"after": true, "above": true, "below": true, "between": true,
	"and": true, "but": true, "or": true, "nor": true, "not": true,
	"so": true, "yet": true, "both": true, "either": true, "neither": true,
	"each": true, "every": true, "all": true, "any": true, "few": true,
	"more": true, "most": true, "other": true, "some": true, "such": true,
	"than": true, "too": true, "very": true, "just": true, "if": true,
	"then": true, "also": true, "that": true, "this": true, "these": true,
	"those": true, "it": true, "its": true, "he": true, "she": true,
	"we": true, "they": true, "i": true, "me": true, "my": true,
	"you": true, "your": true, "his": true, "her": true, "our": true,
	"their": true, "what": true, "which": true, "who": true, "whom": true,
	"how": true, "when": true, "where": true, "why": true, "as": true,
}

// docVec is a package-level type used by clusterCohesion.
type docVec struct {
	key        string
	tfidf      map[string]float64
	conceptIDs []string
}
