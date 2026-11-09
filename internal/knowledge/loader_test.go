package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const cvFR = "---\r\n" +
	"kind: cv\r\n" +
	"lang: fr\r\n" +
	"# comment lines and blank lines are skipped\r\n" +
	"\r\n" +
	"title: \"CV : Alex Abena\"\r\n" +
	"url: /about\r\n" +
	"reviewed: false\r\n" +
	"---\r\n" +
	"\r\n" +
	"## Formation\r\n\r\nÉlève-ingénieur à Saint Jean Ingénieur, Yaoundé.\r\n"

const paper = `---
kind: publication
slug: cerc2026-tonal
title: LinguoMT: Cascade versus End-to-End
lang: en
url: /publications
pdf: /papers/tonal.pdf
venue: CERC 2026, Galway
venue_fr: CERC 2026, Galway (Irlande)
date: 2026-10
summary_en: Compares a cascade with an end-to-end model.
summary_fr: Compare une cascade et un modèle de bout en bout, pour l'igbo.
reviewed: true
---
## Abstract

Body text.
`

func TestParseFileFrontMatterAndAccents(t *testing.T) {
	sources, err := ParseFile([]byte(cvFR))
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("got %d sources", len(sources))
	}
	s := sources[0]
	if s.Kind != domain.SourceCV || s.Ref != "cv" || s.Lang != "fr" || s.URL != "/about" {
		t.Fatalf("source = %+v", s)
	}
	if s.Title != "CV : Alex Abena" {
		t.Fatalf("title %q (quotes should be removed, inner colon kept)", s.Title)
	}
	if s.Text != "## Formation\n\nÉlève-ingénieur à Saint Jean Ingénieur, Yaoundé." {
		t.Fatalf("body %q", s.Text)
	}
	if s.Metadata["reviewed"] != "false" {
		t.Fatalf("metadata %v", s.Metadata)
	}
	if withBOM, err := ParseFile([]byte(byteOrderMark + cvFR)); err != nil || withBOM[0].Text != s.Text {
		t.Fatalf("a leading byte order mark broke parsing: %v", err)
	}
}

func TestParseFilePublicationAddsSummariesAndFrenchSource(t *testing.T) {
	sources, err := ParseFile([]byte(paper))
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 {
		t.Fatalf("got %d sources", len(sources))
	}
	en, fr := sources[0], sources[1]
	if en.Ref != "cerc2026-tonal" || en.Title != "LinguoMT: Cascade versus End-to-End" {
		t.Fatalf("en = %+v", en)
	}
	if !strings.HasPrefix(en.Text, "Summary: Compares a cascade with an end-to-end model.\nVenue: CERC 2026, Galway\nDate: 2026-10\nPDF: /papers/tonal.pdf\n\n## Abstract") {
		t.Fatalf("en text %q", en.Text)
	}
	if fr.Lang != "fr" || fr.Ref != en.Ref || fr.URL != en.URL {
		t.Fatalf("fr = %+v", fr)
	}
	if !strings.Contains(fr.Text, "Résumé : Compare une cascade et un modèle de bout en bout, pour l'igbo.") ||
		!strings.Contains(fr.Text, "Lieu de publication : CERC 2026, Galway (Irlande)") {
		t.Fatalf("fr text %q", fr.Text)
	}
}

func TestParseFileErrors(t *testing.T) {
	for name, content := range map[string]string{
		"no front matter":  "# Title\n",
		"unclosed":         "---\nkind: cv\n",
		"missing title":    "---\nkind: cv\nlang: en\nurl: /about\n---\nbody",
		"bad line":         "---\nkind cv\n---\n",
		"repeated key":     "---\nkind: cv\nkind: about\n---\n",
		"invalid encoding": "---\nkind: cv\ntitle: \xff\n---\n",
	} {
		if _, err := ParseFile([]byte(content)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestDirLoad(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "README.md", "# Not a source\n")
	write(t, dir, "cv.fr.md", cvFR)
	write(t, dir, "papers/cerc2026-tonal.md", paper)
	write(t, dir, "eval.yaml", "questions: []\n")

	sources, err := NewDir(dir).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, s := range sources {
		keys = append(keys, s.Key().String())
	}
	want := "cv/cv/fr publication/cerc2026-tonal/en publication/cerc2026-tonal/fr"
	if strings.Join(keys, " ") != want {
		t.Fatalf("keys = %v, want %s", keys, want)
	}

	// A real French file replaces the derived French summary.
	write(t, dir, "papers/cerc2026-tonal.fr.md", strings.Replace(strings.Replace(paper, "lang: en", "lang: fr", 1), "Body text.", "Texte.", 1))
	sources, err = NewDir(dir).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 3 {
		t.Fatalf("got %d sources", len(sources))
	}
	for _, s := range sources {
		if s.Metadata["derived"] != "" {
			t.Fatalf("derived source kept next to a real one: %+v", s)
		}
	}
}

func TestDirLoadFailsWithoutFiles(t *testing.T) {
	if _, err := NewDir(filepath.Join(t.TempDir(), "missing")).Load(context.Background()); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}

// TestRepositoryCorpusLoads parses the committed knowledge/ directory, so a
// broken front matter fails CI before it fails a deploy.
func TestRepositoryCorpusLoads(t *testing.T) {
	sources, err := NewDir(filepath.Join("..", "..", "knowledge")).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	seen := map[domain.SourceKey]bool{}
	for _, s := range sources {
		if seen[s.Key()] {
			t.Fatalf("duplicate source %s", s.Key())
		}
		seen[s.Key()] = true
		if !s.Kind.Valid() || s.Kind == domain.SourceProject || !domain.ValidLang(s.Lang) || !strings.HasPrefix(s.URL, "/") || strings.TrimSpace(s.Text) == "" {
			t.Errorf("invalid source %+v", s.Key())
		}
	}
	for _, want := range []string{"cv/cv/en", "cv/cv/fr", "about/about/en", "about/about/fr"} {
		found := false
		for k := range seen {
			found = found || k.String() == want
		}
		if !found {
			t.Errorf("corpus lacks %s", want)
		}
	}
}
