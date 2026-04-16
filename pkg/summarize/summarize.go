package summarize

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gnomatix/enkente/pkg/graph"
	"github.com/gnomatix/enkente/pkg/sentiment"
)

// BranchSummary holds the automated summary for a conceptual branch.
type BranchSummary struct {
	RootConceptID string            `json:"root_concept_id"`
	RootLabel     string            `json:"root_label"`
	Depth         int               `json:"depth"`
	NumConcepts   int               `json:"num_concepts"`
	NumEdges      int               `json:"num_edges"`
	Participants  []string          `json:"participants"`       // users involved
	TopConcepts   []ConceptRank     `json:"top_concepts"`       // most connected concepts
	Timeline      []TimelineEntry   `json:"timeline"`           // key events in order
	Sentiment     *SentimentSummary `json:"sentiment,omitempty"`
	Narrative     string            `json:"narrative"`          // generated text summary
	Generated     time.Time         `json:"generated"`
}

// ConceptRank represents a concept sorted by relevance (edge count).
type ConceptRank struct {
	ConceptID  string      `json:"concept_id"`
	Label      string      `json:"label"`
	Type       string      `json:"type"`
	EdgeCount  int         `json:"edge_count"`
	Provenance string      `json:"provenance_user"`
}

// TimelineEntry represents a significant event in the branch's history.
type TimelineEntry struct {
	Time      time.Time `json:"time"`
	Event     string    `json:"event"`
	User      string    `json:"user"`
	ConceptID string    `json:"concept_id"`
}

// SentimentSummary aggregates sentiment across a branch.
type SentimentSummary struct {
	AvgScore     float64                     `json:"avg_score"`
	Dominant     sentiment.Sentiment         `json:"dominant_sentiment"`
	ToneProfile  map[sentiment.ToneTag]int   `json:"tone_profile"`
	Observations int                         `json:"observations"`
}

// SessionSummary holds summary for an entire session.
type SessionSummary struct {
	SessionID       string            `json:"session_id"`
	Participants    []string          `json:"participants"`
	TopicCount      int               `json:"topic_count"`
	ConceptCount    int               `json:"concept_count"`
	EdgeCount       int               `json:"edge_count"`
	TopTopics       []ConceptRank     `json:"top_topics"`
	KeyTerms        []ConceptRank     `json:"key_terms"`
	BranchSummaries []*BranchSummary  `json:"branch_summaries"`
	Narrative       string            `json:"narrative"`
	Generated       time.Time         `json:"generated"`
}

// Summarizer generates automated summaries of conceptual branches and sessions.
type Summarizer struct {
	sentimentTracker *sentiment.Tracker
}

// NewSummarizer creates a summarizer. sentimentTracker may be nil.
func NewSummarizer(st *sentiment.Tracker) *Summarizer {
	return &Summarizer{sentimentTracker: st}
}

// SummarizeBranch generates a summary for a conceptual branch rooted at the
// given concept, traversing up to maxDepth hops in the graph.
func (s *Summarizer) SummarizeBranch(g *graph.ConceptGraph, rootID string, maxDepth int) *BranchSummary {
	concepts, edges := g.SubgraphFor(rootID, maxDepth)

	if len(concepts) == 0 {
		return nil
	}

	root := g.GetConcept(rootID)
	rootLabel := rootID
	if root != nil {
		rootLabel = root.Label
	}

	// Collect participants
	participantSet := make(map[string]bool)
	for _, c := range concepts {
		if c.Type == graph.PersonType {
			participantSet[c.Label] = true
		}
		if c.Provenance.User != "" {
			participantSet[c.Provenance.User] = true
		}
	}
	var participants []string
	for p := range participantSet {
		participants = append(participants, p)
	}
	sort.Strings(participants)

	// Rank concepts by connectivity
	edgeCounts := make(map[string]int)
	for _, e := range edges {
		edgeCounts[e.Source]++
		edgeCounts[e.Target]++
	}

	var ranked []ConceptRank
	for _, c := range concepts {
		ranked = append(ranked, ConceptRank{
			ConceptID:  c.ID,
			Label:      c.Label,
			Type:       string(c.Type),
			EdgeCount:  edgeCounts[c.ID],
			Provenance: c.Provenance.User,
		})
	}
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].EdgeCount > ranked[j].EdgeCount
	})

	// Top concepts (limit to 10)
	topN := 10
	if len(ranked) < topN {
		topN = len(ranked)
	}
	topConcepts := ranked[:topN]

	// Build timeline from concept creation times
	var timeline []TimelineEntry
	for _, c := range concepts {
		if !c.Provenance.Created.IsZero() {
			timeline = append(timeline, TimelineEntry{
				Time:      c.Provenance.Created,
				Event:     fmt.Sprintf("%s introduced %s \"%s\"", c.Provenance.User, c.Type, c.Label),
				User:      c.Provenance.User,
				ConceptID: c.ID,
			})
		}
	}
	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i].Time.Before(timeline[j].Time)
	})

	// Limit timeline to most significant entries
	if len(timeline) > 20 {
		timeline = timeline[:20]
	}

	// Sentiment summary
	var sentSummary *SentimentSummary
	if s.sentimentTracker != nil {
		var totalScore float64
		var totalObs int
		toneProfile := make(map[sentiment.ToneTag]int)

		for _, c := range concepts {
			cs := s.sentimentTracker.GetConceptSentiment(c.ID)
			if cs != nil {
				totalScore += cs.TotalScore
				totalObs += cs.Observations
				for tone, count := range cs.ToneHist {
					toneProfile[tone] += count
				}
			}
		}

		if totalObs > 0 {
			avgScore := totalScore / float64(totalObs)
			dominant := sentiment.Neutral
			if avgScore > 0.15 {
				dominant = sentiment.Positive
			} else if avgScore < -0.15 {
				dominant = sentiment.Negative
			}

			sentSummary = &SentimentSummary{
				AvgScore:     avgScore,
				Dominant:     dominant,
				ToneProfile:  toneProfile,
				Observations: totalObs,
			}
		}
	}

	// Generate narrative text
	narrative := generateBranchNarrative(rootLabel, participants, topConcepts, sentSummary)

	return &BranchSummary{
		RootConceptID: rootID,
		RootLabel:     rootLabel,
		Depth:         maxDepth,
		NumConcepts:   len(concepts),
		NumEdges:      len(edges),
		Participants:  participants,
		TopConcepts:   topConcepts,
		Timeline:      timeline,
		Sentiment:     sentSummary,
		Narrative:     narrative,
		Generated:     time.Now(),
	}
}

// SummarizeSession generates a summary for all concepts in a session.
func (s *Summarizer) SummarizeSession(g *graph.ConceptGraph, sessionID string, sessionConceptIDs []string) *SessionSummary {
	// Collect concepts for this session
	conceptSet := make(map[string]bool)
	var concepts []*graph.Concept
	for _, cid := range sessionConceptIDs {
		c := g.GetConcept(cid)
		if c != nil {
			concepts = append(concepts, c)
			conceptSet[cid] = true
		}
	}

	// Participants
	participantSet := make(map[string]bool)
	for _, c := range concepts {
		if c.Type == graph.PersonType {
			participantSet[c.Label] = true
		}
	}
	var participants []string
	for p := range participantSet {
		participants = append(participants, p)
	}
	sort.Strings(participants)

	// Collect edges within this session's concepts
	var edges []*graph.Edge
	edgeCounts := make(map[string]int)
	allEdges := g.AllEdges()
	for _, e := range allEdges {
		if conceptSet[e.Source] || conceptSet[e.Target] {
			edges = append(edges, e)
			edgeCounts[e.Source]++
			edgeCounts[e.Target]++
		}
	}

	// Rank topics
	var topicRanks []ConceptRank
	var termRanks []ConceptRank
	for _, c := range concepts {
		rank := ConceptRank{
			ConceptID:  c.ID,
			Label:      c.Label,
			Type:       string(c.Type),
			EdgeCount:  edgeCounts[c.ID],
			Provenance: c.Provenance.User,
		}
		switch c.Type {
		case graph.TopicType:
			topicRanks = append(topicRanks, rank)
		case graph.QuotedTermType:
			termRanks = append(termRanks, rank)
		}
	}
	sort.Slice(topicRanks, func(i, j int) bool { return topicRanks[i].EdgeCount > topicRanks[j].EdgeCount })
	sort.Slice(termRanks, func(i, j int) bool { return termRanks[i].EdgeCount > termRanks[j].EdgeCount })

	if len(topicRanks) > 10 {
		topicRanks = topicRanks[:10]
	}
	if len(termRanks) > 10 {
		termRanks = termRanks[:10]
	}

	// Generate branch summaries for top topics
	var branchSummaries []*BranchSummary
	for _, tr := range topicRanks {
		if len(branchSummaries) >= 5 {
			break
		}
		bs := s.SummarizeBranch(g, tr.ConceptID, 2)
		if bs != nil {
			branchSummaries = append(branchSummaries, bs)
		}
	}

	// Session narrative
	narrative := generateSessionNarrative(sessionID, participants, topicRanks, termRanks)

	return &SessionSummary{
		SessionID:       sessionID,
		Participants:    participants,
		TopicCount:      len(topicRanks),
		ConceptCount:    len(concepts),
		EdgeCount:       len(edges),
		TopTopics:       topicRanks,
		KeyTerms:        termRanks,
		BranchSummaries: branchSummaries,
		Narrative:       narrative,
		Generated:       time.Now(),
	}
}

// --- Narrative generation helpers ---

func generateBranchNarrative(rootLabel string, participants []string, topConcepts []ConceptRank, sent *SentimentSummary) string {
	var parts []string

	parts = append(parts, fmt.Sprintf("Discussion branch rooted at \"%s\".", rootLabel))

	if len(participants) > 0 {
		parts = append(parts, fmt.Sprintf("Participants: %s.", strings.Join(participants, ", ")))
	}

	if len(topConcepts) > 0 {
		var labels []string
		for i, tc := range topConcepts {
			if i >= 5 {
				break
			}
			labels = append(labels, tc.Label)
		}
		parts = append(parts, fmt.Sprintf("Key concepts: %s.", strings.Join(labels, ", ")))
	}

	if sent != nil {
		parts = append(parts, fmt.Sprintf("Overall tone: %s (avg score: %.2f, %d observations).",
			sent.Dominant, sent.AvgScore, sent.Observations))

		if len(sent.ToneProfile) > 0 {
			var tones []string
			for tone, count := range sent.ToneProfile {
				tones = append(tones, fmt.Sprintf("%s(%d)", tone, count))
			}
			sort.Strings(tones)
			parts = append(parts, fmt.Sprintf("Tone profile: %s.", strings.Join(tones, ", ")))
		}
	}

	return strings.Join(parts, " ")
}

func generateSessionNarrative(sessionID string, participants []string, topics, terms []ConceptRank) string {
	var parts []string

	parts = append(parts, fmt.Sprintf("Session \"%s\" summary.", sessionID))

	if len(participants) > 0 {
		parts = append(parts, fmt.Sprintf("%d participants: %s.",
			len(participants), strings.Join(participants, ", ")))
	}

	if len(topics) > 0 {
		var labels []string
		for i, t := range topics {
			if i >= 5 {
				break
			}
			labels = append(labels, t.Label)
		}
		parts = append(parts, fmt.Sprintf("Main topics discussed: %s.", strings.Join(labels, ", ")))
	}

	if len(terms) > 0 {
		var labels []string
		for i, t := range terms {
			if i >= 5 {
				break
			}
			labels = append(labels, fmt.Sprintf("\"%s\"", t.Label))
		}
		parts = append(parts, fmt.Sprintf("Key terms introduced: %s.", strings.Join(labels, ", ")))
	}

	return strings.Join(parts, " ")
}
