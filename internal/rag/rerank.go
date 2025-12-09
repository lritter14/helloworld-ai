package rag

import (
	"strings"
	"unicode"
)

const (
	lexicalLengthScale = float32(10.0)
	maxLexicalScore    = float32(0.4)
	headingMatchBonus  = float32(0.1)
)

var lexicalStopwords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "at": {}, "be": {}, "but": {}, "by": {},
	"for": {}, "from": {}, "has": {}, "have": {}, "in": {}, "is": {}, "it": {}, "of": {}, "on": {},
	"or": {}, "the": {}, "to": {}, "was": {}, "were": {}, "with": {},
}

// lexicalScore computes a lightweight lexical relevance score for a chunk relative to a query.
// The score is normalized to remain in a predictable range so it can be blended with vector scores.
func lexicalScore(query, chunkText, headingPath string) float32 {
	queryTokens := filterStopwords(tokenize(query))
	if len(queryTokens) == 0 {
		return 0
	}

	chunkTokens := tokenize(chunkText)
	if len(chunkTokens) == 0 {
		return 0
	}

	chunkFreq := make(map[string]int, len(chunkTokens))
	for _, token := range chunkTokens {
		chunkFreq[token]++
	}

	var rawMatches int
	for _, token := range queryTokens {
		rawMatches += chunkFreq[token]
	}

	score := (float32(rawMatches) / (1 + float32(len(chunkTokens)))) * lexicalLengthScale

	if headingPath != "" {
		headingTokens := tokenize(headingPath)
		if len(headingTokens) > 0 {
			headingSet := make(map[string]struct{}, len(headingTokens))
			for _, token := range headingTokens {
				headingSet[token] = struct{}{}
			}
			var headingMatches int
			for _, token := range queryTokens {
				if _, ok := headingSet[token]; ok {
					headingMatches++
				}
			}
			score += float32(headingMatches) * headingMatchBonus
		}
	}

	if score > maxLexicalScore {
		return maxLexicalScore
	}
	if score < 0 {
		return 0
	}
	return score
}

func tokenize(text string) []string {
	if text == "" {
		return nil
	}

	var builder strings.Builder
	builder.Grow(len(text))
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		} else {
			builder.WriteRune(' ')
		}
	}
	clean := builder.String()
	tokens := strings.Fields(clean)
	if len(tokens) == 0 {
		return nil
	}
	return tokens
}

func filterStopwords(tokens []string) []string {
	if len(tokens) == 0 {
		return nil
	}

	result := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if _, isStop := lexicalStopwords[token]; isStop {
			continue
		}
		result = append(result, token)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
