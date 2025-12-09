package rag

import "testing"

func TestDetermineAutoKDetailHints(t *testing.T) {
	if got := determineAutoK("Short question", nil, "brief"); got != minAutoK {
		t.Fatalf("expected brief detail to set minAutoK (%d), got %d", minAutoK, got)
	}
	if got := determineAutoK("Tell me everything about quarterly OKRs and action plans", nil, "detailed"); got != maxAutoK {
		t.Fatalf("expected detailed detail to set maxAutoK (%d), got %d", maxAutoK, got)
	}
}

func TestDetermineAutoKBroadVsNarrow(t *testing.T) {
	broad := determineAutoK("Give me an overview of all projects and everything in the roadmap?", nil, "")
	narrow := determineAutoK("Status of Q1 OKR synthesis note?", []string{"work/projects"}, "")

	if broad <= narrow {
		t.Fatalf("expected broad query (%d) to request >= narrow query (%d)", broad, narrow)
	}
	if broad < minAutoK || broad > maxAutoK {
		t.Fatalf("broad query result out of range: %d", broad)
	}
	if narrow < minAutoK || narrow > maxAutoK {
		t.Fatalf("narrow query result out of range: %d", narrow)
	}
}

func TestClampUserProvidedK(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{0, 0},
		{1, minAutoK},
		{9, maxAutoK},
		{5, 5},
	}

	for _, tt := range tests {
		if got := clampUserProvidedK(tt.input); got != tt.want {
			t.Fatalf("clampUserProvidedK(%d)=%d, want %d", tt.input, got, tt.want)
		}
	}
}
