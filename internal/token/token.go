package token

import "strings"

// Count estimates the number of tokens in a string.
// Uses a heuristic approximation targeting cl100k_base-style tokenization:
// roughly 1 token per 4 characters for mixed English/code text.
// This avoids external dependencies while being accurate enough for budget enforcement.
func Count(s string) int {
	if len(s) == 0 {
		return 0
	}

	// Heuristic: count words + punctuation clusters.
	// Average token is ~4 chars for cl100k_base on English+code.
	// We use a blend: word-based count adjusted by character length.
	words := len(strings.Fields(s))
	chars := len(s)

	// Weighted average of two estimators for better accuracy
	wordEstimate := float64(words) * 1.3  // words tend to be ~1.3 tokens on average
	charEstimate := float64(chars) / 3.8  // ~3.8 chars per token for mixed content

	estimate := (wordEstimate + charEstimate) / 2.0

	if estimate < 1 {
		return 1
	}
	return int(estimate + 0.5) // round
}
