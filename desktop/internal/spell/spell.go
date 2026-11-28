// Package spell checks spelling with the operating system's spell checker in several languages at once.
//
// WebView2's built-in checking uses one language only (see spellcheck.go in package main). People who
// write in two languages, often in one message, need each word accepted when it is right in any of
// their languages, and a way to mark a piece of text as one language. The Windows Spell Checking API
// does that with the dictionaries Windows already has, including the user's own words.
//
// Positions are UTF-16 code units, the same as JavaScript string indices, so the editor can use them as is.
package spell

import (
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Action is what the spell checker suggests for an error (CORRECTIVE_ACTION in Windows).
type Action int

const (
	ActionNone Action = iota
	// ActionSuggest: the word is unknown, ask Suggest for corrections.
	ActionSuggest
	// ActionReplace: replace the word with Replacement (a common misspelling with one fix).
	ActionReplace
	// ActionDelete: delete the text (a repeated word).
	ActionDelete
)

// Error is one misspelled range in the checked text.
type Error struct {
	// Start and Length are in UTF-16 code units.
	Start       int    `json:"start"`
	Length      int    `json:"length"`
	Action      Action `json:"action"`
	Replacement string `json:"replacement,omitempty"`
}

// Merge keeps the errors every language reports: a word is wrong only when no language accepts it.
// perLang holds the errors of each language for the same text.
func Merge(perLang [][]Error) []Error {
	if len(perLang) == 0 {
		return nil
	}
	type key struct{ start, length int }
	count := map[key]int{}
	for _, errs := range perLang {
		seen := map[key]bool{}
		for _, e := range errs {
			k := key{e.Start, e.Length}
			if !seen[k] {
				seen[k] = true
				count[k]++
			}
		}
	}
	var out []Error
	for _, e := range perLang[0] {
		if count[key{e.Start, e.Length}] == len(perLang) {
			out = append(out, e)
			count[key{e.Start, e.Length}] = 0 // report each range once
		}
	}
	return out
}

// MergeSuggestions interleaves the suggestions of several languages, best first, without duplicates.
func MergeSuggestions(perLang [][]string, limit int) []string {
	seen := map[string]bool{}
	var out []string
	for i := 0; len(out) < limit; i++ {
		more := false
		for _, list := range perLang {
			if i >= len(list) {
				continue
			}
			more = true
			s := list[i]
			if k := strings.ToLower(s); !seen[k] {
				seen[k] = true
				out = append(out, s)
				if len(out) == limit {
					break
				}
			}
		}
		if !more {
			break
		}
	}
	return out
}

// ByCloseness orders the suggestion lists so that the language whose best suggestions are closest
// to the word comes first. Each language's own order is kept. For an English typo ("speling") the
// English list ("spelling") then leads, not a Czech word that merely looks alike ("spalin").
func ByCloseness(word string, perLang [][]string) [][]string {
	best := make([]int, len(perLang))
	for i, list := range perLang {
		best[i] = -1
		for j, s := range list {
			if j == 3 {
				break
			}
			if d := distance(word, s); best[i] < 0 || d < best[i] {
				best[i] = d
			}
		}
	}
	idx := make([]int, len(perLang))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		da, db := best[idx[a]], best[idx[b]]
		if da < 0 || db < 0 {
			return db < 0 && da >= 0
		}
		return da < db
	})
	out := make([][]string, len(perLang))
	for i, k := range idx {
		out[i] = perLang[k]
	}
	return out
}

// distance is the edit distance in half steps: changing only the case or a diacritic mark costs 1,
// any other insertion, deletion or substitution 2. "kun" is closer to "kůň" than to "kino".
func distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = 2 * j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = 2 * i
		for j := 1; j <= len(rb); j++ {
			sub := prev[j-1]
			if ra[i-1] != rb[j-1] {
				if base(ra[i-1]) == base(rb[j-1]) {
					sub++
				} else {
					sub += 2
				}
			}
			cur[j] = min(sub, prev[j]+2, cur[j-1]+2)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

// base is the letter without case and diacritics.
func base(r rune) rune {
	for _, c := range norm.NFD.String(string(unicode.ToLower(r))) {
		return c
	}
	return r
}
