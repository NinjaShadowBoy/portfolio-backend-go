// Package knowledge reads the assistant's corpus files (knowledge/*.md and
// knowledge/papers/*.md) into domain.KnowledgeSource values. The format is
// documented in knowledge/README.md.
package knowledge

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/ports"
)

// Dir loads the corpus from a directory.
type Dir struct {
	Path string

	warned sync.Map // file name -> struct{}; unreviewed files are logged once
}

var _ ports.KnowledgeCorpus = (*Dir)(nil)

func NewDir(path string) *Dir { return &Dir{Path: path} }

// Load reads every corpus file. It fails when the directory holds none, so
// a missing directory never looks like an empty corpus.
func (d *Dir) Load(ctx context.Context) ([]domain.KnowledgeSource, error) {
	var files []string
	for _, pattern := range []string{"*.md", filepath.Join("papers", "*.md")} {
		matches, err := filepath.Glob(filepath.Join(d.Path, pattern))
		if err != nil {
			return nil, err
		}
		files = append(files, matches...)
	}
	slices.Sort(files)

	var sources []domain.KnowledgeSource
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if strings.EqualFold(filepath.Base(file), "README.md") {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		parsed, err := ParseFile(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		if parsed[0].Metadata["reviewed"] != "true" {
			if _, seen := d.warned.LoadOrStore(file, struct{}{}); !seen {
				slog.Warn("knowledge file not reviewed yet", "file", file)
			}
		}
		sources = append(sources, parsed...)
	}
	sources = dropDerivedDuplicates(sources)
	if len(sources) == 0 {
		return nil, fmt.Errorf("no knowledge files in %s", d.Path)
	}
	return sources, nil
}

// ParseFile parses one corpus file. Most files give one source; a
// publication with a summary_fr and lang en also gives a French source made
// of the French summary, so French questions can match it by full text.
func ParseFile(data []byte) ([]domain.KnowledgeSource, error) {
	if !utf8.Valid(data) {
		return nil, errors.New("file is not valid UTF-8")
	}
	meta, body, err := splitFrontMatter(string(data))
	if err != nil {
		return nil, err
	}
	kind := domain.SourceKind(meta["kind"])
	ref := meta["ref"]
	if ref == "" {
		ref = meta["slug"]
	}
	if ref == "" {
		ref = string(kind)
	}
	src := domain.KnowledgeSource{
		Kind: kind, Ref: ref, Lang: meta["lang"], Title: meta["title"], URL: meta["url"],
		Text: body, Metadata: meta,
	}
	for _, field := range []string{"kind", "lang", "title", "url"} {
		if meta[field] == "" {
			return nil, fmt.Errorf("front matter has no %s", field)
		}
	}
	if kind != domain.SourcePublication {
		return []domain.KnowledgeSource{src}, nil
	}

	src.Text = publicationHeader(meta, domain.LangEnglish) + body
	out := []domain.KnowledgeSource{src}
	if src.Lang == domain.LangEnglish && meta["summary_fr"] != "" {
		out = append(out, domain.KnowledgeSource{
			Kind: kind, Ref: ref, Lang: domain.LangFrench, Title: src.Title, URL: src.URL,
			Text:     strings.TrimSpace(publicationHeader(meta, domain.LangFrench)),
			Metadata: map[string]string{"derived": "summary_fr"},
		})
	}
	return out, nil
}

func publicationHeader(meta map[string]string, lang string) string {
	labels := [][2]string{{"Summary", "summary_en"}, {"Venue", "venue"}, {"Date", "date"}, {"Authors", "authors"}, {"PDF", "pdf"}}
	if lang == domain.LangFrench {
		labels = [][2]string{{"Résumé", "summary_fr"}, {"Lieu de publication", "venue_fr"}, {"Date", "date"}, {"Auteurs", "authors"}, {"PDF", "pdf"}}
	}
	var b strings.Builder
	sep := ": "
	if lang == domain.LangFrench {
		sep = " : "
	}
	for _, l := range labels {
		if v := meta[l[1]]; v != "" {
			b.WriteString(l[0] + sep + v + "\n")
		}
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	return b.String()
}

// dropDerivedDuplicates removes a derived French summary when a real French
// file for the same publication exists.
func dropDerivedDuplicates(sources []domain.KnowledgeSource) []domain.KnowledgeSource {
	real := map[domain.SourceKey]bool{}
	for _, s := range sources {
		if s.Metadata["derived"] == "" {
			real[s.Key()] = true
		}
	}
	return slices.DeleteFunc(sources, func(s domain.KnowledgeSource) bool {
		return s.Metadata["derived"] != "" && real[s.Key()]
	})
}

// splitFrontMatter reads the block between the first two "---" lines. Each
// line in it is "key: value"; the value is the rest of the line, with one
// pair of surrounding quotes removed. Blank lines and "#" comments are
// skipped. There are no nested or multi-line values.
func splitFrontMatter(text string) (map[string]string, string, error) {
	// Checkouts on Windows may have CRLF endings; offsets below assume LF.
	text = strings.ReplaceAll(strings.TrimPrefix(text, byteOrderMark), "\r\n", "\n")
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return nil, "", errors.New("file does not start with a --- front matter line")
	}
	meta := map[string]string{}
	consumed := len(scanner.Text()) + 1
	closed := false
	for scanner.Scan() {
		line := scanner.Text()
		consumed += len(line) + 1
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			closed = true
			break
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		key = strings.TrimSpace(key)
		if !ok || key == "" || strings.ContainsAny(key, " \t") {
			return nil, "", fmt.Errorf("bad front matter line %q", trimmed)
		}
		if _, dup := meta[key]; dup {
			return nil, "", fmt.Errorf("front matter key %q repeated", key)
		}
		meta[key] = unquote(strings.TrimSpace(value))
	}
	if err := scanner.Err(); err != nil {
		return nil, "", err
	}
	if !closed {
		return nil, "", errors.New("front matter is not closed with ---")
	}
	body := ""
	if consumed < len(text) {
		body = text[consumed:]
	}
	return meta, strings.TrimSpace(body), nil
}

// byteOrderMark is U+FEFF, which some editors put at the start of a file.
const byteOrderMark = string(rune(0xFEFF))

func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
		return v[1 : len(v)-1]
	}
	return v
}
