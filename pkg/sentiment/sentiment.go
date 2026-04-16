package sentiment

import (
	"math"
	"strings"
	"time"

	"github.com/gnomatix/enkente/pkg/graph"
	"github.com/gnomatix/enkente/pkg/parser"
)

// Sentiment represents the emotional valence of a message or concept.
type Sentiment string

const (
	Positive Sentiment = "positive"
	Negative Sentiment = "negative"
	Neutral  Sentiment = "neutral"
	Mixed    Sentiment = "mixed"
)

// ToneTag represents a specific emotional tone detected in text.
type ToneTag string

const (
	Enthusiastic ToneTag = "enthusiastic"
	Concerned    ToneTag = "concerned"
	Questioning  ToneTag = "questioning"
	Assertive    ToneTag = "assertive"
	Tentative    ToneTag = "tentative"
	Agreeing     ToneTag = "agreeing"
	Disagreeing  ToneTag = "disagreeing"
	Frustrated   ToneTag = "frustrated"
	Supportive   ToneTag = "supportive"
)

// Analysis holds the sentiment and tone analysis result for a single message.
type Analysis struct {
	Score     float64   `json:"score"`     // -1.0 (negative) to +1.0 (positive)
	Sentiment Sentiment `json:"sentiment"` // classified sentiment
	Tones     []ToneTag `json:"tones"`     // detected tone tags
	Intensity float64   `json:"intensity"` // 0.0 (mild) to 1.0 (strong)
}

// ConceptSentiment tracks aggregated sentiment for a concept over time.
type ConceptSentiment struct {
	ConceptID    string    `json:"concept_id"`
	AvgScore     float64   `json:"avg_score"`
	Observations int       `json:"observations"`
	TotalScore   float64   `json:"total_score"`
	LastUpdated  time.Time `json:"last_updated"`
	ToneHist     map[ToneTag]int `json:"tone_histogram"` // frequency of each tone
}

// Tracker maintains per-concept sentiment state.
type Tracker struct {
	concepts map[string]*ConceptSentiment
}

// NewTracker creates a new sentiment tracker.
func NewTracker() *Tracker {
	return &Tracker{
		concepts: make(map[string]*ConceptSentiment),
	}
}

// Heuristic word lists for Go-side sentiment estimation.
// These provide baseline capability without requiring the Python NLP pipeline.
var (
	positiveWords = map[string]float64{
		"good": 0.5, "great": 0.7, "excellent": 0.9, "amazing": 0.9,
		"love": 0.8, "agree": 0.6, "yes": 0.4, "perfect": 0.8,
		"wonderful": 0.8, "fantastic": 0.9, "nice": 0.5, "helpful": 0.6,
		"awesome": 0.8, "brilliant": 0.8, "exciting": 0.7, "interesting": 0.5,
		"support": 0.5, "like": 0.4, "cool": 0.5, "thanks": 0.5,
		"exactly": 0.6, "absolutely": 0.7, "definitely": 0.6, "right": 0.3,
		"elegant": 0.6, "clean": 0.4, "powerful": 0.6, "efficient": 0.5,
	}

	negativeWords = map[string]float64{
		"bad": -0.5, "terrible": -0.9, "awful": -0.9, "hate": -0.8,
		"disagree": -0.6, "no": -0.3, "wrong": -0.5, "problem": -0.4,
		"issue": -0.3, "bug": -0.4, "broken": -0.6, "fail": -0.6,
		"concern": -0.4, "worried": -0.5, "unfortunately": -0.5, "confusing": -0.5,
		"unclear": -0.5, "difficult": -0.3, "complicated": -0.4, "messy": -0.5,
		"blocker": -0.6, "blocked": -0.5, "stuck": -0.4, "annoying": -0.6,
		"frustrating": -0.7, "slow": -0.3, "ugly": -0.5, "hacky": -0.5,
	}

	// Tone indicator patterns (word -> tone)
	toneIndicators = map[string]ToneTag{
		"!":            Enthusiastic,
		"?":            Questioning,
		"agree":        Agreeing,
		"disagree":     Disagreeing,
		"but":          Tentative,
		"however":      Tentative,
		"maybe":        Tentative,
		"perhaps":      Tentative,
		"might":        Tentative,
		"should":       Assertive,
		"must":         Assertive,
		"need":         Assertive,
		"definitely":   Assertive,
		"concern":      Concerned,
		"worried":      Concerned,
		"risk":         Concerned,
		"careful":      Concerned,
		"love":         Supportive,
		"support":      Supportive,
		"help":         Supportive,
		"thanks":       Supportive,
		"frustrating":  Frustrated,
		"annoying":     Frustrated,
		"stuck":        Frustrated,
	}

	// Negation words that flip sentiment
	negationWords = map[string]bool{
		"not": true, "no": true, "never": true, "neither": true,
		"nor": true, "nobody": true, "nothing": true, "nowhere": true,
		"don't": true, "doesn't": true, "didn't": true, "isn't": true,
		"aren't": true, "wasn't": true, "weren't": true, "won't": true,
		"wouldn't": true, "shouldn't": true, "couldn't": true, "can't": true,
	}

	// Intensifiers that amplify sentiment
	intensifiers = map[string]float64{
		"very": 1.5, "really": 1.5, "extremely": 2.0, "incredibly": 2.0,
		"so": 1.3, "quite": 1.2, "pretty": 1.1, "super": 1.5,
		"totally": 1.5, "absolutely": 1.7, "completely": 1.5,
	}
)

// Analyze performs heuristic sentiment analysis on a message.
// This is a Go-side estimation -- the Python NLP pipeline provides
// more accurate analysis when available.
func Analyze(msg parser.AntigravityMessage) Analysis {
	text := strings.ToLower(msg.Message)
	words := strings.Fields(text)

	var totalScore float64
	var wordCount int
	toneSet := make(map[ToneTag]bool)

	negated := false
	intensifier := 1.0

	for _, word := range words {
		// Clean punctuation for word lookup
		clean := strings.Trim(word, ".,!?;:'\"()-")

		// Check for negation
		if negationWords[clean] {
			negated = true
			continue
		}

		// Check for intensifiers
		if mult, ok := intensifiers[clean]; ok {
			intensifier = mult
			continue
		}

		// Score positive words
		if score, ok := positiveWords[clean]; ok {
			if negated {
				totalScore -= score * intensifier
			} else {
				totalScore += score * intensifier
			}
			wordCount++
			negated = false
			intensifier = 1.0
		}

		// Score negative words
		if score, ok := negativeWords[clean]; ok {
			if negated {
				totalScore -= score * intensifier // double negative = positive
			} else {
				totalScore += score * intensifier
			}
			wordCount++
			negated = false
			intensifier = 1.0
		}

		// Detect tones from word
		if tone, ok := toneIndicators[clean]; ok {
			toneSet[tone] = true
		}
	}

	// Check punctuation-based tones
	if strings.Contains(text, "!") {
		toneSet[Enthusiastic] = true
	}
	if strings.Contains(text, "?") {
		toneSet[Questioning] = true
	}

	// Calculate normalized score
	var score float64
	if wordCount > 0 {
		score = totalScore / float64(wordCount)
		// Clamp to [-1, 1]
		score = math.Max(-1, math.Min(1, score))
	}

	// Classify sentiment
	sentiment := Neutral
	if score > 0.15 {
		sentiment = Positive
	} else if score < -0.15 {
		sentiment = Negative
	}
	// Mixed: if we have both strong positive and negative tones
	if toneSet[Agreeing] && toneSet[Disagreeing] {
		sentiment = Mixed
	}
	if toneSet[Supportive] && toneSet[Concerned] {
		sentiment = Mixed
	}

	// Collect tones
	var tones []ToneTag
	for t := range toneSet {
		tones = append(tones, t)
	}

	// Intensity: magnitude of the score
	intensity := math.Abs(score)

	return Analysis{
		Score:     score,
		Sentiment: sentiment,
		Tones:     tones,
		Intensity: intensity,
	}
}

// RecordForConcepts updates the tracker with sentiment data for each concept
// mentioned in or associated with the analyzed message.
func (t *Tracker) RecordForConcepts(conceptIDs []string, analysis Analysis) {
	now := time.Now()
	for _, cid := range conceptIDs {
		cs, ok := t.concepts[cid]
		if !ok {
			cs = &ConceptSentiment{
				ConceptID: cid,
				ToneHist:  make(map[ToneTag]int),
			}
			t.concepts[cid] = cs
		}
		cs.Observations++
		cs.TotalScore += analysis.Score
		cs.AvgScore = cs.TotalScore / float64(cs.Observations)
		cs.LastUpdated = now

		for _, tone := range analysis.Tones {
			cs.ToneHist[tone]++
		}
	}
}

// GetConceptSentiment returns the aggregated sentiment for a concept.
// Returns nil if no sentiment data exists for the concept.
func (t *Tracker) GetConceptSentiment(conceptID string) *ConceptSentiment {
	return t.concepts[conceptID]
}

// AllConceptSentiments returns all tracked concept sentiments.
func (t *Tracker) AllConceptSentiments() []*ConceptSentiment {
	result := make([]*ConceptSentiment, 0, len(t.concepts))
	for _, cs := range t.concepts {
		result = append(result, cs)
	}
	return result
}

// ApplyToGraph attaches sentiment metadata to concept Properties in the graph.
func (t *Tracker) ApplyToGraph(g *graph.ConceptGraph) {
	for cid, cs := range t.concepts {
		c := g.GetConcept(cid)
		if c == nil {
			continue
		}
		if c.Properties == nil {
			c.Properties = make(map[string]any)
		}
		c.Properties["sentiment_avg"] = cs.AvgScore
		c.Properties["sentiment_observations"] = cs.Observations
		c.Properties["tone_histogram"] = cs.ToneHist
	}
}
