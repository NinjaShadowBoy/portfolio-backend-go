package usecase

import (
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

func words(n int, word string) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = word
	}
	return strings.Join(parts, " ")
}

func TestChunkTextPacksSmallSectionsWithTitlePrefix(t *testing.T) {
	text := "Intro line.\n\n## Education\n\nGrenoble INP - Ensimag.\n\n## Experience\n\nTWMicronics."
	chunks := chunkText("CV of Alex", text, 1200, 150)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1: %q", len(chunks), chunks)
	}
	want := "CV of Alex\n\nIntro line.\n\n## Education\n\nGrenoble INP - Ensimag.\n\n## Experience\n\nTWMicronics."
	if chunks[0] != want {
		t.Fatalf("chunk = %q\nwant  %q", chunks[0], want)
	}
}

func TestChunkTextStartsNewChunkAtHeadingWhenFull(t *testing.T) {
	a := "## A\n\n" + words(100, "alpha") // 6 + 599 runes
	b := "## B\n\n" + words(100, "betaa")
	chunks := chunkText("T", a+"\n\n"+b, 800, 100)
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(chunks))
	}
	if !strings.HasPrefix(chunks[0], "T\n\n## A") || !strings.HasPrefix(chunks[1], "T\n\n## B") {
		t.Fatalf("chunks do not start at their headings: %q / %q", chunks[0][:12], chunks[1][:12])
	}
}

func TestChunkTextSplitsLongSectionWithOverlap(t *testing.T) {
	var sentences []string
	for i := range 60 {
		sentences = append(sentences, "Sentence number "+strings.Repeat("x", i%7)+" talks about Yaoundé and speech translation.")
	}
	body := strings.Join(sentences, " ")
	chunks := chunkText("Paper", "## Results\n\n"+body, 1200, 150)
	if len(chunks) < 3 {
		t.Fatalf("got %d chunks for %d runes", len(chunks), runeLen(body))
	}
	for i, c := range chunks {
		if !strings.HasPrefix(c, "Paper\n\n") {
			t.Fatalf("chunk %d has no title prefix: %q", i, c[:20])
		}
		if i > 0 && !strings.HasPrefix(c, "Paper\n\n## Results\n\n") {
			t.Fatalf("continued chunk %d lacks its heading: %q", i, c[:30])
		}
		bodyPart := strings.TrimPrefix(strings.TrimPrefix(c, "Paper\n\n"), "## Results\n\n")
		if i > 0 && runeLen(bodyPart) > 1200 {
			t.Fatalf("chunk %d body is %d runes, over 1200", i, runeLen(bodyPart))
		}
		if !strings.HasSuffix(bodyPart, ".") {
			t.Fatalf("chunk %d does not end at a sentence boundary: %q", i, bodyPart[len(bodyPart)-20:])
		}
	}
}

func TestSplitWindowOverlapAndBoundaries(t *testing.T) {
	numbered := make([]string, 300)
	for i := range numbered {
		numbered[i] = "mot" + strings.Repeat("é", i%3) + string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	text := strings.Join(numbered, " ") + " é" // distinct words, multi-byte runes
	pieces := splitWindow(text, 500, 150)
	if len(pieces) < 4 {
		t.Fatalf("got %d pieces", len(pieces))
	}
	for i, p := range pieces {
		if runeLen(p) > 500 {
			t.Fatalf("piece %d is %d runes", i, runeLen(p))
		}
		if strings.HasPrefix(p, " ") || strings.HasSuffix(p, " ") {
			t.Fatalf("piece %d is not trimmed", i)
		}
		if i == 0 {
			continue
		}
		prev := pieces[i-1]
		// The new piece begins with text the previous one ended with.
		overlap := 0
		for n := min(len(prev), len(p)); n > 0; n-- {
			if strings.HasSuffix(prev, p[:n]) {
				overlap = runeLen(p[:n])
				break
			}
		}
		if overlap < 100 || overlap > 150 {
			t.Fatalf("piece %d overlaps the previous one by %d runes, want about 150", i, overlap)
		}
		if !strings.HasPrefix(p, "mot") && !strings.HasPrefix(p, "é") {
			t.Fatalf("piece %d starts mid-word: %q", i, p[:5])
		}
	}
	if !strings.HasSuffix(pieces[len(pieces)-1], "é") {
		t.Fatal("the multi-byte tail was lost")
	}
}

func TestSplitWindowHardCutsTextWithoutSpaces(t *testing.T) {
	pieces := splitWindow(strings.Repeat("é", 1000), 300, 50)
	if len(pieces) < 4 || runeLen(pieces[0]) != 300 {
		t.Fatalf("pieces: %d, first %d runes", len(pieces), runeLen(pieces[0]))
	}
}

func TestFuseRanksOrder(t *testing.T) {
	// 2 is first in text and second in vectors: it wins. 1 is top of vectors
	// only; 3 is only in text at rank 2; 4 is in both lists, lower down.
	got := fuseRanks([]int64{1, 2, 4}, []int64{2, 3, 4}, 60)
	ids := make([]int64, len(got))
	for i, h := range got {
		ids[i] = h.id
	}
	if want := []int64{2, 4, 1, 3}; !slices.Equal(ids, want) {
		t.Fatalf("order = %v, want %v", ids, want)
	}
	if got[0].vectorRank != 2 || got[0].textRank != 1 {
		t.Fatalf("ranks of 2 = %+v", got[0])
	}
	if want := 1.0/62 + 1.0/61; math.Abs(got[0].score-want) > 1e-12 {
		t.Fatalf("score = %v, want %v", got[0].score, want)
	}
	// Equal scores: the better single rank, then the lower id, comes first.
	tie := fuseRanks([]int64{9}, []int64{5}, 60)
	if tie[0].id != 5 || tie[1].id != 9 {
		t.Fatalf("tie order = %+v", tie)
	}
	// A duplicate in one list counts once.
	dup := fuseRanks([]int64{7, 7}, nil, 60)
	if len(dup) != 1 || math.Abs(dup[0].score-1.0/61) > 1e-12 {
		t.Fatalf("duplicate = %+v", dup)
	}
}

func TestCosine(t *testing.T) {
	for _, tc := range []struct {
		a, b []float32
		want float64
	}{
		{[]float32{1, 0}, []float32{2, 0}, 1},
		{[]float32{1, 0}, []float32{0, 3}, 0},
		{[]float32{1, 1}, []float32{-1, -1}, -1},
		{[]float32{1, 2, 3}, []float32{2, 4, 6}, 1},
		{[]float32{3, 4}, []float32{4, 3}, 24.0 / 25},
		{[]float32{0, 0}, []float32{1, 1}, 0},
		{[]float32{1}, []float32{1, 1}, 0},
	} {
		if got := cosine(tc.a, tc.b); math.Abs(got-tc.want) > 1e-6 {
			t.Errorf("cosine(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSplitPeriodMatchesFrontend(t *testing.T) {
	period, body := splitPeriod("Built an app.\nPeriod: Feb 2024 - Jun 2024.\n\n\nMore text.")
	if period != "Feb 2024 - Jun 2024" || body != "Built an app.\n\nMore text." {
		t.Fatalf("period %q body %q", period, body)
	}
	if period, body := splitPeriod("  No period here. "); period != "" || body != "No period here." {
		t.Fatalf("period %q body %q", period, body)
	}
}

func TestSourceHashCoversModelAndText(t *testing.T) {
	src := domain.KnowledgeSource{Kind: domain.SourceCV, Ref: "cv", Lang: "en", Title: "CV", URL: "/about", Text: "text"}
	base := sourceHash(src, "voyage-4-large")
	if base != sourceHash(src, "voyage-4-large") {
		t.Fatal("hash is not stable")
	}
	if base == sourceHash(src, "voyage-4") {
		t.Fatal("the model is not part of the hash")
	}
	changed := src
	changed.Text = "text!"
	if base == sourceHash(changed, "voyage-4-large") {
		t.Fatal("the text is not part of the hash")
	}
}
