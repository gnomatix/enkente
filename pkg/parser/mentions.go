package parser

import (
	"strings"
	"unicode"
)

// MentionKind distinguishes topic tags (#topic) from user references (@user).
type MentionKind string

const (
	MentionTag  MentionKind = "tag"
	MentionUser MentionKind = "user"
)

// Mention is one explicit contextual reference found in a message.
// Name is normalized to lower case so #GraphDB and #graphdb are one entity.
type Mention struct {
	Kind MentionKind `json:"kind"`
	Name string      `json:"name"`
}

// ParseMentionKind maps the URL/CLI spelling ("tag", "user", "#", "@") to a
// MentionKind. ok is false for anything else.
func ParseMentionKind(s string) (kind MentionKind, ok bool) {
	switch strings.ToLower(s) {
	case "tag", "tags", "#":
		return MentionTag, true
	case "user", "users", "@":
		return MentionUser, true
	}
	return "", false
}

func isMentionChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.'
}

// ExtractMentions returns the distinct #tag and @user references in text, in
// order of first appearance.
//
// A sigil counts only at the start of the text or after a character that
// cannot be part of a name, so e-mail addresses (a@b.com) and URL fragments
// (page#section) are not mentions. A name is letters, digits, '_', '-' and
// '.'; trailing '.' and '-' are dropped as sentence punctuation. Names made
// only of digits (#42, issue numbers) are ignored.
func ExtractMentions(text string) []Mention {
	runes := []rune(text)
	seen := map[Mention]bool{}
	var out []Mention
	for i := 0; i < len(runes); i++ {
		var kind MentionKind
		switch runes[i] {
		case '#':
			kind = MentionTag
		case '@':
			kind = MentionUser
		default:
			continue
		}
		if i > 0 && (isMentionChar(runes[i-1]) || runes[i-1] == '#' || runes[i-1] == '@' || runes[i-1] == '/') {
			continue
		}
		j := i + 1
		for j < len(runes) && isMentionChar(runes[j]) {
			j++
		}
		name := strings.TrimRight(string(runes[i+1:j]), ".-")
		i = j - 1
		if name == "" || !strings.ContainsFunc(name, unicode.IsLetter) {
			continue
		}
		m := Mention{Kind: kind, Name: strings.ToLower(name)}
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}
