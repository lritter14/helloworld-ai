package rag

import (
	"math"
	"strings"
	"testing"
)

func TestLexicalScoreBasicMatch(t *testing.T) {
	query := "Project updates"
	chunk := "The project timeline lists recent updates for the project. These updates cover scope."
	score := lexicalScore(query, chunk, "# Planning > ## Updates")

	if score <= 0 {
		t.Fatalf("expected score to be positive, got %f", score)
	}
	if score > maxLexicalScore {
		t.Fatalf("score should be clamped to maxLexicalScore, got %f", score)
	}
}

func TestLexicalScoreHeadingBonus(t *testing.T) {
	query := "database"
	chunk := "General context without the keyword."
	score := lexicalScore(query, chunk, "# Architecture > ## Database Layer")

	if math.Abs(float64(score-headingMatchBonus)) > 0.0001 {
		t.Fatalf("expected heading bonus only (%f), got %f", headingMatchBonus, score)
	}
}

func TestLexicalScoreStopwordsRemoved(t *testing.T) {
	query := "the and of"
	chunk := "the and of"
	score := lexicalScore(query, chunk, "")

	if score != 0 {
		t.Fatalf("expected score 0 when query tokens are only stopwords, got %f", score)
	}
}

func TestLexicalScoreNormalization(t *testing.T) {
	query := "project"
	// Repeat keyword many times to ensure normalization kicks in
	chunk := "project " + strings.Repeat(" filler", 200)
	score := lexicalScore(query, chunk, "")

	if score <= 0 {
		t.Fatalf("expected normalized score to stay positive, got %f", score)
	}
	if score > maxLexicalScore {
		t.Fatalf("expected score to be clamped to %f, got %f", maxLexicalScore, score)
	}
}
