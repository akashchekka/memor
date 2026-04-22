package index

import (
	"strings"
	"unicode"
)

// Trigrams extracts all 3-character shingles from a lowercased, punctuation-stripped string.
func Trigrams(s string) []string {
	s = normalize(s)
	if len(s) < 3 {
		return nil
	}

	runes := []rune(s)
	seen := make(map[string]struct{})
	var result []string

	for i := 0; i <= len(runes)-3; i++ {
		tri := string(runes[i : i+3])
		if _, ok := seen[tri]; !ok {
			seen[tri] = struct{}{}
			result = append(result, tri)
		}
	}
	return result
}

// normalize lowercases and strips non-alphanumeric characters (keeps spaces).
func normalize(s string) string {
	s = strings.ToLower(s)
	var sb strings.Builder
	sb.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// TrigramIndex maps trigrams to sets of entry indices.
type TrigramIndex struct {
	Postings map[string]map[int]struct{}
}

// NewTrigramIndex creates an empty trigram index.
func NewTrigramIndex() *TrigramIndex {
	return &TrigramIndex{
		Postings: make(map[string]map[int]struct{}),
	}
}

// Add indexes a document at the given position.
func (idx *TrigramIndex) Add(docID int, text string) {
	for _, tri := range Trigrams(text) {
		if idx.Postings[tri] == nil {
			idx.Postings[tri] = make(map[int]struct{})
		}
		idx.Postings[tri][docID] = struct{}{}
	}
}

// Search returns candidate document IDs matching the query by intersecting trigram postings.
func (idx *TrigramIndex) Search(query string) []int {
	trigrams := Trigrams(query)
	if len(trigrams) == 0 {
		// Return all docs if query is too short for trigrams
		return idx.AllDocs()
	}

	// Start with the posting list of the first trigram
	var candidates map[int]struct{}
	for i, tri := range trigrams {
		postings := idx.Postings[tri]
		if postings == nil {
			return nil // no matches
		}
		if i == 0 {
			candidates = make(map[int]struct{}, len(postings))
			for id := range postings {
				candidates[id] = struct{}{}
			}
		} else {
			// Intersect
			for id := range candidates {
				if _, ok := postings[id]; !ok {
					delete(candidates, id)
				}
			}
			if len(candidates) == 0 {
				return nil
			}
		}
	}

	result := make([]int, 0, len(candidates))
	for id := range candidates {
		result = append(result, id)
	}
	return result
}

// AllDocs returns all indexed document IDs.
func (idx *TrigramIndex) AllDocs() []int {
	seen := make(map[int]struct{})
	for _, postings := range idx.Postings {
		for id := range postings {
			seen[id] = struct{}{}
		}
	}
	result := make([]int, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	return result
}
