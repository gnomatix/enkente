// Package listener is the passive jargon and ambiguity listener: it scans
// message text against a lexicon of domain terms and reports which terms are
// overloaded (more than one recorded sense) and which synonyms were used in
// place of a canonical term. It flags; it never rewrites or interrupts.
package listener

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode"
)

// Entry is one canonical term in the lexicon.
type Entry struct {
	// Term is the canonical spelling, e.g. "database".
	Term string `json:"term"`
	// Senses lists the distinct meanings in use. Two or more marks the term
	// as overloaded.
	Senses []string `json:"senses,omitempty"`
	// Synonyms are alternative spellings or phrasings that refer to Term,
	// e.g. "db", "datastore". Multi-word synonyms are allowed.
	Synonyms []string `json:"synonyms,omitempty"`
}

// Lexicon is the listener's vocabulary, as loaded from a JSON array of
// Entry objects.
type Lexicon struct {
	entries []Entry
	// phrases maps a normalized phrase (space-joined lower-case words) to
	// the index of its entry.
	phrases  map[string]int
	maxWords int
}

// Flag reports one lexicon term found in a message.
type Flag struct {
	Term string `json:"term"`
	// Matched is the phrase as it appeared (lower-cased), which differs from
	// Term when a synonym was used.
	Matched string `json:"matched"`
	// Overloaded is true when the term has more than one recorded sense; the
	// reader should confirm which one is meant.
	Overloaded bool     `json:"overloaded"`
	Senses     []string `json:"senses,omitempty"`
	// Synonym is true when Matched is a synonym rather than the term itself.
	Synonym bool `json:"synonym"`
	// Count is how many times the phrase occurs in the message.
	Count int `json:"count"`
}

// Uncertain reports whether this flag should be surfaced as an ambiguity.
func (f Flag) Uncertain() bool { return f.Overloaded }

func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-')
	})
}

// NewLexicon validates entries and builds the lookup table. Terms and
// synonyms are matched on whole words, case-insensitively. A phrase may
// belong to only one entry.
func NewLexicon(entries []Entry) (*Lexicon, error) {
	l := &Lexicon{entries: entries, phrases: map[string]int{}}
	add := func(phrase string, idx int) error {
		w := words(phrase)
		if len(w) == 0 {
			return fmt.Errorf("entry %q: empty term or synonym", entries[idx].Term)
		}
		key := strings.Join(w, " ")
		if prev, ok := l.phrases[key]; ok && prev != idx {
			return fmt.Errorf("phrase %q belongs to both %q and %q", key, entries[prev].Term, entries[idx].Term)
		}
		l.phrases[key] = idx
		if len(w) > l.maxWords {
			l.maxWords = len(w)
		}
		return nil
	}
	for i, e := range entries {
		if err := add(e.Term, i); err != nil {
			return nil, err
		}
		for _, s := range e.Synonyms {
			if err := add(s, i); err != nil {
				return nil, err
			}
		}
	}
	return l, nil
}

// LoadLexicon reads a JSON array of Entry objects from path.
func LoadLexicon(path string) (*Lexicon, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []Entry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("lexicon %s: %w", path, err)
	}
	return NewLexicon(entries)
}

// Len returns the number of canonical terms.
func (l *Lexicon) Len() int { return len(l.entries) }

// Analyze returns a flag per distinct phrase from the lexicon found in text,
// ordered by first occurrence. The longest phrase wins at each position, so
// "graph database" is matched before "database" when both are listed.
func (l *Lexicon) Analyze(text string) []Flag {
	if l == nil || len(l.phrases) == 0 {
		return []Flag{}
	}
	w := words(text)
	byPhrase := map[string]*Flag{}
	var order []string
	for i := 0; i < len(w); {
		matched := 0
		for n := min(l.maxWords, len(w)-i); n >= 1; n-- {
			key := strings.Join(w[i:i+n], " ")
			idx, ok := l.phrases[key]
			if !ok {
				continue
			}
			if f := byPhrase[key]; f != nil {
				f.Count++
			} else {
				e := l.entries[idx]
				byPhrase[key] = &Flag{
					Term:       e.Term,
					Matched:    key,
					Overloaded: len(e.Senses) > 1,
					Senses:     e.Senses,
					Synonym:    key != strings.Join(words(e.Term), " "),
					Count:      1,
				}
				order = append(order, key)
			}
			matched = n
			break
		}
		if matched == 0 {
			matched = 1
		}
		i += matched
	}
	out := make([]Flag, 0, len(order))
	for _, k := range order {
		out = append(out, *byPhrase[k])
	}
	return out
}

// Summary counts flags by category for response metadata.
type Summary struct {
	Flags      []Flag   `json:"flags"`
	Overloaded []string `json:"overloaded"`
}

// Summarize wraps Analyze with the sorted list of overloaded terms, which is
// the metadata attached to a processed message.
func (l *Lexicon) Summarize(text string) Summary {
	flags := l.Analyze(text)
	seen := map[string]bool{}
	over := []string{}
	for _, f := range flags {
		if f.Overloaded && !seen[f.Term] {
			seen[f.Term] = true
			over = append(over, f.Term)
		}
	}
	sort.Strings(over)
	return Summary{Flags: flags, Overloaded: over}
}

// PromptUser is the author of messages the active listener posts. The
// server never prompts on messages from this user, so prompts cannot loop.
const PromptUser = "listener"

// Prompt returns the clarification question for the overloaded terms in a
// summary, addressed to author when known, or "" when nothing is ambiguous.
// Each term lists its senses as lettered options so a reply can be short:
//
//	@alice Listener here: which sense of "database" did you mean? (a) storage engine (b) dataset
func Prompt(sum Summary, author string) string {
	var terms, parts []string
	var lastOpts string
	seen := map[string]bool{}
	for _, f := range sum.Flags {
		if !f.Overloaded || seen[f.Term] {
			continue
		}
		seen[f.Term] = true
		var opts []string
		for i, sense := range f.Senses {
			opts = append(opts, fmt.Sprintf("(%c) %s", 'a'+i, sense))
		}
		lastOpts = strings.Join(opts, " ")
		terms = append(terms, f.Term)
		parts = append(parts, fmt.Sprintf("%q %s", f.Term, lastOpts))
	}
	if len(parts) == 0 {
		return ""
	}
	prefix := "Listener here: "
	if author = strings.TrimSpace(author); author != "" && !strings.ContainsAny(author, " \t") {
		prefix = "@" + author + " " + prefix
	}
	if len(parts) == 1 {
		return fmt.Sprintf("%swhich sense of %q did you mean? %s", prefix, terms[0], lastOpts)
	}
	return prefix + "which senses did you mean? " + strings.Join(parts, "; ")
}
