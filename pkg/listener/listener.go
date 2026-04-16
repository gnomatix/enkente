package listener

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gnomatix/enkente/pkg/graph"
)

// Mode controls whether the listener is passive (metadata only) or active
// (generates real-time interruption prompts).
type Mode string

const (
	Passive Mode = "passive" // flag ambiguities in metadata
	Active  Mode = "active"  // generate interruption prompts
)

// AmbiguityType classifies the kind of ambiguity detected.
type AmbiguityType string

const (
	Jargon         AmbiguityType = "jargon"          // domain-specific term
	OverloadedTerm AmbiguityType = "overloaded_term"  // term with multiple meanings
	Synonym        AmbiguityType = "synonym"          // different words, same meaning
	Acronym        AmbiguityType = "acronym"          // unexpanded abbreviation
	Vague          AmbiguityType = "vague"            // imprecise language
)

// Flag represents a detected ambiguity or jargon usage.
type Flag struct {
	ID          string        `json:"id"`
	Type        AmbiguityType `json:"type"`
	Term        string        `json:"term"`
	Context     string        `json:"context"`      // surrounding text
	User        string        `json:"user"`          // who used it
	Session     string        `json:"session"`
	Definitions []Definition  `json:"definitions"`   // known definitions for this term
	Resolved    bool          `json:"resolved"`
	Resolution  string        `json:"resolution,omitempty"` // chosen meaning
	Prompt      string        `json:"prompt,omitempty"`     // active listener prompt
	Created     time.Time     `json:"created"`
}

// Definition represents one known meaning of a term.
type Definition struct {
	Meaning  string `json:"meaning"`
	Source   string `json:"source"`   // who defined it or where it came from
	Context  string `json:"context"`  // in what context this meaning applies
	UseCount int    `json:"use_count"`
}

// TermProfile tracks all known information about a term across sessions.
type TermProfile struct {
	Term        string            `json:"term"`
	Definitions []Definition      `json:"definitions"`
	Synonyms    []string          `json:"synonyms,omitempty"`
	Users       map[string]int    `json:"users"`       // user -> usage count
	Sessions    map[string]int    `json:"sessions"`    // session -> usage count
	FirstSeen   time.Time         `json:"first_seen"`
	LastSeen    time.Time         `json:"last_seen"`
	FlagCount   int               `json:"flag_count"`
}

// Listener implements the jargon and ambiguity resolution system.
// Per requirements: identifies domain-specific jargon, tracks synonyms,
// recognizes overloaded terms, flags ambiguities, and optionally interrupts
// the chat in real-time.
type Listener struct {
	mu          sync.RWMutex
	mode        Mode
	terms       map[string]*TermProfile  // normalized term -> profile
	flags       map[string]*Flag         // flag ID -> flag
	flagCounter int

	// Configurable thresholds
	MinDefinitions int // flag as overloaded when definitions >= this (default: 2)
}

// NewListener creates a new Listener in the given mode.
func NewListener(mode Mode) *Listener {
	return &Listener{
		mode:           mode,
		terms:          make(map[string]*TermProfile),
		flags:          make(map[string]*Flag),
		MinDefinitions: 2,
	}
}

// SetMode changes the listener mode.
func (l *Listener) SetMode(mode Mode) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.mode = mode
}

// GetMode returns the current listener mode.
func (l *Listener) GetMode() Mode {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.mode
}

// RegisterDefinition records a known definition for a term.
// This can be called by users (two-way curation) or by extraction.
func (l *Listener) RegisterDefinition(term, meaning, source, context string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	normalized := strings.ToLower(strings.TrimSpace(term))
	profile := l.getOrCreateProfile(normalized)

	// Check if this definition already exists
	for i, d := range profile.Definitions {
		if strings.EqualFold(d.Meaning, meaning) {
			profile.Definitions[i].UseCount++
			return
		}
	}

	profile.Definitions = append(profile.Definitions, Definition{
		Meaning:  meaning,
		Source:   source,
		Context:  context,
		UseCount: 1,
	})
}

// RegisterSynonym records that two terms are synonymous.
func (l *Listener) RegisterSynonym(term1, term2 string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	n1 := strings.ToLower(strings.TrimSpace(term1))
	n2 := strings.ToLower(strings.TrimSpace(term2))

	p1 := l.getOrCreateProfile(n1)
	p2 := l.getOrCreateProfile(n2)

	// Add each as synonym of the other (deduplicated)
	if !containsString(p1.Synonyms, n2) {
		p1.Synonyms = append(p1.Synonyms, n2)
	}
	if !containsString(p2.Synonyms, n1) {
		p2.Synonyms = append(p2.Synonyms, n1)
	}
}

// AnalyzeMessage scans a message for jargon, overloaded terms, and ambiguities.
// Returns any flags generated. In active mode, flags include prompt text.
func (l *Listener) AnalyzeMessage(text, user, session string) []*Flag {
	l.mu.Lock()
	defer l.mu.Unlock()

	words := strings.Fields(strings.ToLower(text))
	var flags []*Flag

	// Track usage
	for _, word := range words {
		clean := strings.Trim(word, ".,!?;:'\"()-[]{}@#")
		if len(clean) < 2 {
			continue
		}

		profile, exists := l.terms[clean]
		if !exists {
			continue
		}

		// Update usage
		profile.Users[user]++
		profile.Sessions[session]++
		profile.LastSeen = time.Now()
	}

	// Detect quoted terms as potential jargon introductions
	quotedTerms := extractQuotedTerms(text)
	for _, qt := range quotedTerms {
		normalized := strings.ToLower(qt)
		profile := l.getOrCreateProfile(normalized)
		profile.Users[user]++
		profile.Sessions[session]++

		// First time seeing this quoted term? Flag as potential jargon
		if profile.FlagCount == 0 && len(profile.Definitions) == 0 {
			flag := l.createFlag(Jargon, qt, text, user, session, profile)
			flags = append(flags, flag)
			profile.FlagCount++
		}
	}

	// Check for overloaded terms (multiple definitions from different users)
	for _, word := range words {
		clean := strings.Trim(word, ".,!?;:'\"()-[]{}@#")
		profile, exists := l.terms[clean]
		if !exists {
			continue
		}

		if len(profile.Definitions) >= l.MinDefinitions {
			// Check if different users gave different definitions
			sources := make(map[string]bool)
			for _, d := range profile.Definitions {
				sources[d.Source] = true
			}
			if len(sources) > 1 {
				flag := l.createFlag(OverloadedTerm, clean, text, user, session, profile)
				flags = append(flags, flag)
				profile.FlagCount++
			}
		}
	}

	// Check for vague language
	vaguePatterns := []string{
		"it", "this thing", "that stuff", "whatever",
		"something", "somehow", "somewhere", "the thing",
	}
	lowerText := strings.ToLower(text)
	for _, vp := range vaguePatterns {
		if strings.Contains(lowerText, vp) {
			// Only flag if the vague term is used in a context that lacks specificity
			if isVagueUsage(lowerText, vp) {
				profile := l.getOrCreateProfile(vp)
				flag := l.createFlag(Vague, vp, text, user, session, profile)
				flags = append(flags, flag)
			}
		}
	}

	// Store flags
	for _, f := range flags {
		l.flags[f.ID] = f
	}

	return flags
}

// ResolveFlag marks a flag as resolved with the chosen meaning.
func (l *Listener) ResolveFlag(flagID, resolution string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	flag, ok := l.flags[flagID]
	if !ok {
		return fmt.Errorf("flag %s not found", flagID)
	}

	flag.Resolved = true
	flag.Resolution = resolution

	// Register the resolution as a definition
	normalized := strings.ToLower(flag.Term)
	profile := l.getOrCreateProfile(normalized)
	for i, d := range profile.Definitions {
		if strings.EqualFold(d.Meaning, resolution) {
			profile.Definitions[i].UseCount++
			return nil
		}
	}
	profile.Definitions = append(profile.Definitions, Definition{
		Meaning:  resolution,
		Source:   "resolution:" + flag.User,
		UseCount: 1,
	})

	return nil
}

// GetFlags returns all flags, optionally filtered by resolved status.
func (l *Listener) GetFlags(includeResolved bool) []*Flag {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []*Flag
	for _, f := range l.flags {
		if includeResolved || !f.Resolved {
			result = append(result, f)
		}
	}
	return result
}

// GetTermProfile returns the profile for a specific term.
func (l *Listener) GetTermProfile(term string) *TermProfile {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.terms[strings.ToLower(strings.TrimSpace(term))]
}

// AllTermProfiles returns all tracked term profiles.
func (l *Listener) AllTermProfiles() []*TermProfile {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*TermProfile, 0, len(l.terms))
	for _, p := range l.terms {
		result = append(result, p)
	}
	return result
}

// ApplyToGraph creates jargon/ambiguity metadata in the concept graph.
// Quoted terms with ambiguity flags get Properties["listener_flags"] set.
func (l *Listener) ApplyToGraph(g *graph.ConceptGraph) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	now := time.Now()
	for term, profile := range l.terms {
		if len(profile.Definitions) == 0 && profile.FlagCount == 0 {
			continue
		}

		// Find matching concept in graph
		cid := fmt.Sprintf("quoted_term:%s", strings.ReplaceAll(term, " ", "_"))
		c := g.GetConcept(cid)
		if c == nil {
			// Also check topic concepts
			cid = fmt.Sprintf("topic:%s", strings.ReplaceAll(term, " ", "_"))
			c = g.GetConcept(cid)
		}

		if c != nil {
			if c.Properties == nil {
				c.Properties = make(map[string]any)
			}
			c.Properties["listener_definitions"] = profile.Definitions
			c.Properties["listener_flag_count"] = profile.FlagCount
			if len(profile.Synonyms) > 0 {
				c.Properties["listener_synonyms"] = profile.Synonyms
			}
		}

		// Create synonym edges
		for _, syn := range profile.Synonyms {
			synCID := fmt.Sprintf("quoted_term:%s", strings.ReplaceAll(syn, " ", "_"))
			if g.GetConcept(synCID) != nil && g.GetConcept(cid) != nil {
				edgeID := graph.EdgeKey(cid, synCID, graph.RelatedTo)
				g.AddEdge(&graph.Edge{
					ID:       edgeID,
					Source:   cid,
					Target:   synCID,
					Type:     graph.RelatedTo,
					Weight:   1,
					Created:  now,
					Modified: now,
					Properties: map[string]any{
						"relationship": "synonym",
					},
				})
			}
		}
	}
}

// --- Internal helpers ---

func (l *Listener) getOrCreateProfile(normalized string) *TermProfile {
	profile, ok := l.terms[normalized]
	if !ok {
		now := time.Now()
		profile = &TermProfile{
			Term:      normalized,
			Users:     make(map[string]int),
			Sessions:  make(map[string]int),
			FirstSeen: now,
			LastSeen:  now,
		}
		l.terms[normalized] = profile
	}
	return profile
}

func (l *Listener) createFlag(ambType AmbiguityType, term, context, user, session string, profile *TermProfile) *Flag {
	l.flagCounter++
	flagID := fmt.Sprintf("flag:%d", l.flagCounter)

	flag := &Flag{
		ID:          flagID,
		Type:        ambType,
		Term:        term,
		Context:     context,
		User:        user,
		Session:     session,
		Definitions: profile.Definitions,
		Created:     time.Now(),
	}

	// In active mode, generate a prompt
	if l.mode == Active {
		flag.Prompt = generatePrompt(ambType, term, profile)
	}

	return flag
}

func generatePrompt(ambType AmbiguityType, term string, profile *TermProfile) string {
	switch ambType {
	case Jargon:
		return fmt.Sprintf("Hey! Listener here -- what did you mean by \"%s\"? Could you define it for the group?", term)
	case OverloadedTerm:
		var defs []string
		for _, d := range profile.Definitions {
			defs = append(defs, fmt.Sprintf("\"%s\" (%s)", d.Meaning, d.Source))
		}
		return fmt.Sprintf("Hey! Listener here -- \"%s\" has been used with different meanings: %s. Which meaning do you intend?",
			term, strings.Join(defs, ", "))
	case Synonym:
		return fmt.Sprintf("Hey! Listener here -- are \"%s\" and \"%s\" the same thing in this context?",
			term, strings.Join(profile.Synonyms, "/"))
	case Vague:
		return fmt.Sprintf("Hey! Listener here -- could you be more specific about what you mean by \"%s\"?", term)
	default:
		return fmt.Sprintf("Hey! Listener here -- can you clarify what you mean by \"%s\"?", term)
	}
}

func extractQuotedTerms(text string) []string {
	var terms []string
	inQuote := false
	var current strings.Builder
	for _, ch := range text {
		if ch == '"' {
			if inQuote {
				term := current.String()
				if len(term) >= 2 {
					terms = append(terms, term)
				}
				current.Reset()
			}
			inQuote = !inQuote
			continue
		}
		if inQuote {
			current.WriteRune(ch)
		}
	}
	return terms
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// isVagueUsage checks if a vague pattern is used in a truly vague context
// (not as part of a longer, more specific phrase).
func isVagueUsage(text, pattern string) bool {
	idx := strings.Index(text, pattern)
	if idx < 0 {
		return false
	}
	// Check if it's at a word boundary
	if idx > 0 {
		prev := text[idx-1]
		if prev != ' ' && prev != ',' && prev != '.' {
			return false
		}
	}
	end := idx + len(pattern)
	if end < len(text) {
		next := text[end]
		if next != ' ' && next != ',' && next != '.' && next != '!' && next != '?' {
			return false
		}
	}
	return true
}
