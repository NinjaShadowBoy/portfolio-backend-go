package usecase

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

// The assistant's tools. All are read-only: they read the projects table,
// the knowledge index and the corpus files, never visitor-written text, and
// fetch no URLs. Each returns JSON text plus the links it surfaced; only
// those links can appear in the links event.

const (
	searchDefaultResults = 4
	searchMaxResults     = 6
	searchMaxQueryRunes  = 200
	searchSnippetRunes   = 1000
	projectSummaryRunes  = 200
	projectFieldRunes    = 2500
	technologyMaxRunes   = 100
)

// Public contact details the assistant may give. They match what the site
// already publishes (portfolio-app core/constants/contact.ts and
// social-links.service.ts); the phone number is left out on purpose.
const contactEmail = "alex.nelson.bryan@gmail.com"

type chatProfile struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

var publicProfiles = []chatProfile{
	{Name: "LinkedIn", URL: "https://www.linkedin.com/in/alex-nelson-ryan-abena-439068290/"},
	{Name: "GitHub", URL: "https://github.com/NinjaShadowBoy/NinjaShadowBoy"},
}

type chatTool struct {
	spec domain.ToolSpec
	run  func(ctx context.Context, r *chatRun, input json.RawMessage) (string, []domain.ChatLink, error)
	// effectOnly marks a tool whose result adds nothing to the answer (it
	// only animates the page): when the model already wrote its answer in
	// the same turn, the chat ends there instead of asking it again.
	effectOnly bool
}

// chatToolOrder is the order tools are given to the model. It never changes
// between requests, so the tool definitions stay a cacheable prefix.
var chatToolOrder = []string{"search_site", "read_page", "list_projects", "get_project", "list_publications", "get_contact_options", "get_document", "ninja_move"}

var chatTools = map[string]chatTool{
	"search_site": {
		spec: domain.ToolSpec{
			Name: "search_site",
			Description: "Full-text and semantic search over Alex's projects, publications, CV and About page. " +
				"Returns the best matching passages with their page URL. Use it for questions about skills, experience, studies, research results or anything not covered by the other tools.",
			InputSchema: objectSchema(map[string]any{
				"query": map[string]any{"type": "string", "description": "What to look for: keywords or a short question, at most 200 characters."},
				"kinds": map[string]any{
					"type":        "array",
					"description": "Only search these kinds of source. Omit to search everything.",
					"items":       map[string]any{"type": "string", "enum": []string{"project", "publication", "cv", "about"}},
				},
				"limit": map[string]any{"type": "integer", "description": "Number of passages to return, 1 to 6. Default 4."},
			}, "query"),
		},
		run: toolSearchSite,
	},
	"read_page": {
		spec: domain.ToolSpec{
			Name: "read_page",
			Description: "Returns the full text of a page of the site in the visitor's language: about (education, experience, research, what Alex is looking for, " +
				"technologies, hobbies) or cv (profile, education, experience, research, projects, skills, languages). Use it for any question about Alex himself.",
			InputSchema: objectSchema(map[string]any{
				"page": map[string]any{"type": "string", "enum": []string{"about", "cv"}, "description": "Which page."},
			}, "page"),
		},
		run: toolReadPage,
	},
	"list_projects": {
		spec: domain.ToolSpec{
			Name:        "list_projects",
			Description: "Lists Alex's projects in display order with id, name, a one-line summary, technologies and page URL. Optionally filter by a technology or by featured projects.",
			InputSchema: objectSchema(map[string]any{
				"technology": map[string]any{"type": "string", "description": "Only projects using this technology, e.g. Angular (case-insensitive, at most 100 characters)."},
				"featured":   map[string]any{"type": "boolean", "description": "true for featured projects only."},
			}),
		},
		run: toolListProjects,
	},
	"get_project": {
		spec: domain.ToolSpec{
			Name:        "get_project",
			Description: "Returns one project in full: description, challenges, what Alex learned, technologies, source code and demo links, and page URL. Take the id from list_projects or search_site.",
			InputSchema: objectSchema(map[string]any{
				"id": map[string]any{"type": "integer", "description": "The project id."},
			}, "id"),
		},
		run: toolGetProject,
	},
	"list_publications": {
		spec: domain.ToolSpec{
			Name:        "list_publications",
			Description: "Lists Alex's research publications with title, venue, date, authors, his role, a summary, the publications page URL and the PDF path.",
			InputSchema: objectSchema(map[string]any{}),
		},
		run: toolListPublications,
	},
	"get_contact_options": {
		spec: domain.ToolSpec{
			Name:        "get_contact_options",
			Description: "Returns how to contact Alex: the contact page, his email, public profiles and his CV as PDF.",
			InputSchema: objectSchema(map[string]any{}),
		},
		run: toolContactOptions,
	},
	"get_document": {
		spec: domain.ToolSpec{
			Name: "get_document",
			Description: "Gives the visitor a file to download: Alex's CV (English or French PDF) or one of his papers (PDF). " +
				"The chat shows a download button for it under the answer.",
			InputSchema: objectSchema(map[string]any{
				"document": map[string]any{"type": "string", "description": "\"cv\", or a paper slug from list_publications."},
				"lang":     map[string]any{"type": "string", "enum": []string{"en", "fr"}, "description": "CV language. Omit to use the visitor's language. Papers ignore it."},
			}, "document"),
		},
		run: toolGetDocument,
	},
	"ninja_move": {
		// The model learns nothing about the animation it triggers: no move
		// names, no description of what happens. What it does not know, it
		// cannot narrate to the visitor.
		spec: domain.ToolSpec{
			Name:        "ninja_move",
			Description: "Plays a short surprise animation of you on the visitor's screen. Takes no arguments and returns nothing to tell the visitor.",
			InputSchema: objectSchema(map[string]any{}),
		},
		run:        toolNinjaMove,
		effectOnly: true,
	},
}

func chatToolSpecs() []domain.ToolSpec {
	specs := make([]domain.ToolSpec, len(chatToolOrder))
	for i, name := range chatToolOrder {
		specs[i] = chatTools[name].spec
	}
	return specs
}

// objectSchema builds a strict object schema: no properties beyond those
// listed. Limits (lengths, ranges) are in the descriptions and checked by
// the tools, because strict tool schemas do not support them.
func objectSchema(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// errToolArgs marks arguments a tool rejects; its text is shown to the model.
type errToolArgs struct{ msg string }

func (e errToolArgs) Error() string { return "invalid arguments: " + e.msg }

func badArgs(format string, args ...any) error { return errToolArgs{msg: fmt.Sprintf(format, args...)} }

// errToolFailed carries a message safe to show the model.
type errToolFailed struct{ msg string }

func (e errToolFailed) Error() string { return e.msg }

func toolError(callID, message string) domain.ToolResult {
	content, _ := json.Marshal(map[string]string{"error": message})
	return domain.ToolResult{CallID: callID, Content: string(content), IsError: true}
}

// toolErrorMessage turns a tool failure into text for the model without
// leaking internal errors.
func toolErrorMessage(err error) string {
	var args errToolArgs
	var failed errToolFailed
	switch {
	case errors.As(err, &args):
		return args.Error()
	case errors.As(err, &failed):
		return failed.msg
	case errors.Is(err, domain.ErrKnowledgeDisabled):
		return "site search is unavailable right now; use list_projects, get_project or list_publications instead"
	case errors.Is(err, domain.ErrNotFound):
		return "not found"
	}
	return "the lookup failed; answer from what you already have or suggest the contact page"
}

// decodeArgs decodes the tool input strictly: one JSON object with no
// unknown fields. An empty input counts as {}.
func decodeArgs(input json.RawMessage, dst any) error {
	trimmed := bytes.TrimSpace(input)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		trimmed = []byte("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return badArgs("%v", err)
	}
	if dec.More() {
		return badArgs("trailing data after the arguments object")
	}
	return nil
}

func toolJSON(v any) (string, error) {
	data, err := json.Marshal(v)
	return string(data), err
}

type searchArgs struct {
	Query string   `json:"query"`
	Kinds []string `json:"kinds"`
	Limit *int     `json:"limit"`
}

type searchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Kind    string `json:"kind"`
	Lang    string `json:"lang"`
	Snippet string `json:"snippet"`
}

func toolSearchSite(ctx context.Context, r *chatRun, input json.RawMessage) (string, []domain.ChatLink, error) {
	var args searchArgs
	if err := decodeArgs(input, &args); err != nil {
		return "", nil, err
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "", nil, badArgs("query is required")
	}
	if utf8.RuneCountInString(query) > searchMaxQueryRunes {
		return "", nil, badArgs("query is longer than %d characters", searchMaxQueryRunes)
	}
	if len(args.Kinds) > 4 {
		return "", nil, badArgs("at most 4 kinds")
	}
	var kinds []domain.SourceKind
	for _, k := range args.Kinds {
		kind := domain.SourceKind(k)
		if !kind.Valid() {
			return "", nil, badArgs("unknown kind %q; use project, publication, cv or about", k)
		}
		kinds = append(kinds, kind)
	}
	limit := searchDefaultResults
	if args.Limit != nil {
		if *args.Limit < 1 || *args.Limit > searchMaxResults {
			return "", nil, badArgs("limit must be between 1 and %d", searchMaxResults)
		}
		limit = *args.Limit
	}
	hits, err := r.s.SearchKnowledge(ctx, query, r.locale, kinds, limit)
	if err != nil {
		if errors.Is(err, ErrBadRequest) {
			return "", nil, badArgs("%v", err)
		}
		return "", nil, err
	}
	results := make([]searchResult, 0, len(hits))
	var links []domain.ChatLink
	for _, hit := range hits {
		c := hit.Chunk
		snippet := strings.TrimSpace(strings.TrimPrefix(c.Content, strings.TrimSpace(c.Title)))
		results = append(results, searchResult{
			Title: c.Title, URL: c.URL, Kind: string(c.Kind), Lang: c.Lang,
			Snippet: truncateRunes(snippet, searchSnippetRunes),
		})
		links = append(links, domain.ChatLink{Title: c.Title, URL: c.URL, Kind: linkKindForSource(c.Kind)})
	}
	out, err := toolJSON(map[string]any{"results": results})
	return out, links, err
}

func linkKindForSource(kind domain.SourceKind) string {
	switch kind {
	case domain.SourceProject:
		return domain.ChatLinkProject
	case domain.SourcePublication:
		return domain.ChatLinkPublication
	}
	return domain.ChatLinkPage
}

type listProjectsArgs struct {
	Technology *string `json:"technology"`
	Featured   *bool   `json:"featured"`
}

type projectSummary struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	Summary      string   `json:"summary"`
	Technologies []string `json:"technologies"`
	Featured     bool     `json:"featured"`
	URL          string   `json:"url"`
}

func toolListProjects(ctx context.Context, r *chatRun, input json.RawMessage) (string, []domain.ChatLink, error) {
	var args listProjectsArgs
	if err := decodeArgs(input, &args); err != nil {
		return "", nil, err
	}
	technology := ""
	if args.Technology != nil {
		technology = strings.ToLower(strings.TrimSpace(*args.Technology))
		if utf8.RuneCountInString(technology) > technologyMaxRunes {
			return "", nil, badArgs("technology is longer than %d characters", technologyMaxRunes)
		}
	}
	projects, err := r.s.repos.Projects().ListProjects(ctx)
	if err != nil {
		return "", nil, err
	}
	technologies := map[int64][]string{}
	if len(projects) > 0 {
		ids := make([]int64, len(projects))
		for i, p := range projects {
			ids[i] = p.ID
		}
		rows, err := r.s.repos.Projects().ListAllTechnologiesByProjectID(ctx, ids)
		if err != nil {
			return "", nil, err
		}
		for _, row := range rows {
			technologies[row.ProjectID] = append(technologies[row.ProjectID], row.Technology)
		}
	}
	out := []projectSummary{}
	var links []domain.ChatLink
	for _, p := range projects {
		techs := technologies[p.ID]
		if args.Featured != nil && *args.Featured && !p.Featured {
			continue
		}
		if technology != "" && !slices.ContainsFunc(techs, func(t string) bool { return strings.Contains(strings.ToLower(t), technology) }) {
			continue
		}
		url := projectURL(p.ID)
		out = append(out, projectSummary{
			ID: p.ID, Name: p.Name, Summary: projectOneLine(p, r.locale), Technologies: nonNil(techs), Featured: p.Featured, URL: url,
		})
		links = append(links, domain.ChatLink{Title: p.Name, URL: url, Kind: domain.ChatLinkProject})
	}
	text, err := toolJSON(map[string]any{"count": len(out), "projects": out})
	return text, links, err
}

type getProjectArgs struct {
	ID *json.Number `json:"id"`
}

type projectDetail struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	URL          string   `json:"url"`
	Featured     bool     `json:"featured"`
	Period       string   `json:"period,omitempty"`
	Technologies []string `json:"technologies"`
	Description  string   `json:"description"`
	Challenges   string   `json:"challenges,omitempty"`
	WhatILearned string   `json:"whatILearned,omitempty"`
	SourceCode   string   `json:"sourceCode,omitempty"`
	Demo         string   `json:"demo,omitempty"`
}

func toolGetProject(ctx context.Context, r *chatRun, input json.RawMessage) (string, []domain.ChatLink, error) {
	var args getProjectArgs
	if err := decodeArgs(input, &args); err != nil {
		return "", nil, err
	}
	if args.ID == nil {
		return "", nil, badArgs("id is required")
	}
	id, err := strconv.ParseInt(args.ID.String(), 10, 64)
	if err != nil || id <= 0 {
		return "", nil, badArgs("id must be a positive integer")
	}
	p, err := r.s.repos.Projects().GetProject(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return "", nil, errToolFailed{msg: fmt.Sprintf("there is no project with id %d; call list_projects for the ids", id)}
	}
	if err != nil {
		return "", nil, err
	}
	techs, err := r.s.repos.Projects().ListTechnologiesByProjectID(ctx, id)
	if err != nil {
		return "", nil, err
	}
	period, description := splitPeriod(p.Description)
	challenges, learned := deref(p.Challenges), deref(p.WhatILearned)
	if r.locale == domain.LangFrench {
		if fr := strings.TrimSpace(deref(p.DescriptionFr)); fr != "" {
			_, description = splitPeriod(fr)
		}
		challenges = pickText(challenges, deref(p.ChallengesFr))
		learned = pickText(learned, deref(p.WhatILearnedFr))
	}
	detail := projectDetail{
		ID: p.ID, Name: p.Name, URL: projectURL(p.ID), Featured: p.Featured, Period: period, Technologies: nonNil(techs),
		Description:  truncateRunes(description, projectFieldRunes),
		Challenges:   truncateRunes(strings.TrimSpace(challenges), projectFieldRunes),
		WhatILearned: truncateRunes(strings.TrimSpace(learned), projectFieldRunes),
	}
	links := []domain.ChatLink{{Title: p.Name, URL: detail.URL, Kind: domain.ChatLinkProject}}
	fr := r.locale == domain.LangFrench
	if link := httpURL(deref(p.GithubLink)); link != "" {
		detail.SourceCode = link
		links = append(links, domain.ChatLink{Title: localized(fr, p.Name+" source code", p.Name+" : code source"), URL: link, Kind: domain.ChatLinkExternal})
	}
	if link := httpURL(deref(p.DemoLink)); link != "" {
		detail.Demo = link
		links = append(links, domain.ChatLink{Title: localized(fr, p.Name+" demo", p.Name+" : démo"), URL: link, Kind: domain.ChatLinkExternal})
	}
	text, err := toolJSON(detail)
	return text, links, err
}

type publication struct {
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Venue   string `json:"venue,omitempty"`
	Date    string `json:"date,omitempty"`
	Authors string `json:"authors,omitempty"`
	Role    string `json:"role,omitempty"`
	Summary string `json:"summary,omitempty"`
	URL     string `json:"url"`
	PDF     string `json:"pdf,omitempty"`
}

// toolListPublications reads the publications from the corpus front matter,
// the same files the knowledge index is built from.
func toolListPublications(ctx context.Context, r *chatRun, input json.RawMessage) (string, []domain.ChatLink, error) {
	var args struct{}
	if err := decodeArgs(input, &args); err != nil {
		return "", nil, err
	}
	sources, err := r.s.loadCorpus(ctx)
	if err != nil {
		return "", nil, err
	}
	fr := r.locale == domain.LangFrench
	byRef := map[string]domain.KnowledgeSource{}
	for _, src := range sources {
		if src.Kind != domain.SourcePublication || src.Metadata["derived"] != "" {
			continue
		}
		// Papers usually have an English file only; a file in the visitor's
		// language wins when there is one.
		if prev, ok := byRef[src.Ref]; !ok || (prev.Lang != r.locale && src.Lang == r.locale) {
			byRef[src.Ref] = src
		}
	}
	out := make([]publication, 0, len(byRef))
	for _, src := range byRef {
		meta := src.Metadata
		pub := publication{
			Slug: src.Ref, Title: src.Title, Venue: meta["venue"], Date: meta["date"], Authors: meta["authors"],
			Role: meta["role"], Summary: meta["summary_en"], URL: "/publications", PDF: sitePath(meta["pdf"]),
		}
		if fr {
			pub.Venue = cmp.Or(meta["venue_fr"], pub.Venue)
			pub.Summary = cmp.Or(meta["summary_fr"], pub.Summary)
		}
		out = append(out, pub)
	}
	slices.SortFunc(out, func(a, b publication) int {
		if c := cmp.Compare(b.Date, a.Date); c != 0 {
			return c
		}
		return cmp.Compare(a.Slug, b.Slug)
	})
	var links []domain.ChatLink
	for _, pub := range out {
		links = append(links, domain.ChatLink{Title: localized(fr, "Publications", "Publications"), URL: pub.URL, Kind: domain.ChatLinkPublication})
		if pub.PDF != "" {
			links = append(links, domain.ChatLink{Title: pub.Title + " (PDF)", URL: pub.PDF, Kind: domain.ChatLinkPDF})
		}
	}
	text, err := toolJSON(map[string]any{"publications": out})
	return text, links, err
}

type contactOptions struct {
	ContactPage string           `json:"contactPage"`
	Email       string           `json:"email"`
	Profiles    []chatProfile    `json:"profiles"`
	CV          []map[string]any `json:"cv,omitempty"`
}

func toolContactOptions(ctx context.Context, r *chatRun, input json.RawMessage) (string, []domain.ChatLink, error) {
	var args struct{}
	if err := decodeArgs(input, &args); err != nil {
		return "", nil, err
	}
	fr := r.locale == domain.LangFrench
	c := r.s.chat
	out := contactOptions{ContactPage: "/contact", Email: c.contactEmail, Profiles: c.publicProfile}
	links := []domain.ChatLink{{Title: localized(fr, "Contact", "Contact"), URL: "/contact", Kind: domain.ChatLinkPage}}
	for _, p := range c.publicProfile {
		links = append(links, domain.ChatLink{Title: p.Name, URL: p.URL, Kind: domain.ChatLinkExternal})
	}
	// The CV PDFs come from the corpus front matter; without the corpus the
	// rest of the answer still stands.
	if sources, err := r.s.loadCorpus(ctx); err == nil {
		for _, src := range sources {
			if pdf := sitePath(src.Metadata["pdf"]); src.Kind == domain.SourceCV && pdf != "" {
				out.CV = append(out.CV, map[string]any{"lang": src.Lang, "url": pdf})
				title := "CV (English, PDF)"
				if src.Lang == domain.LangFrench {
					title = "CV (français, PDF)"
				}
				links = append(links, domain.ChatLink{Title: title, URL: pdf, Kind: domain.ChatLinkPDF})
			}
		}
	} else if ctx.Err() != nil {
		return "", nil, ctx.Err()
	}
	text, err := toolJSON(out)
	return text, links, err
}

type documentResult struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Lang  string `json:"lang"`
	Note  string `json:"note"`
}

// toolGetDocument hands out one downloadable PDF named in the corpus front
// matter: a CV by language or a paper by slug. Only those paths can come out.
func toolGetDocument(ctx context.Context, r *chatRun, input json.RawMessage) (string, []domain.ChatLink, error) {
	var args struct {
		Document string `json:"document"`
		Lang     string `json:"lang"`
	}
	if err := decodeArgs(input, &args); err != nil {
		return "", nil, err
	}
	doc := strings.ToLower(strings.TrimSpace(args.Document))
	lang := strings.ToLower(strings.TrimSpace(args.Lang))
	if lang != "" && lang != domain.LangEnglish && lang != domain.LangFrench {
		return "", nil, badArgs("lang must be en or fr")
	}
	if lang == "" {
		lang = r.locale
	}
	sources, err := r.s.loadCorpus(ctx)
	if err != nil {
		return "", nil, err
	}
	fr := r.locale == domain.LangFrench

	var found *domain.KnowledgeSource
	var slugs []string
	for i := range sources {
		src := &sources[i]
		pdf := sitePath(src.Metadata["pdf"])
		if pdf == "" || src.Metadata["derived"] != "" {
			continue
		}
		switch src.Kind {
		case domain.SourceCV:
			if doc == "cv" && (found == nil || src.Lang == lang) {
				found = src
			}
		case domain.SourcePublication:
			slugs = append(slugs, src.Ref)
			if doc == src.Ref {
				found = src
			}
		}
	}
	if found == nil {
		return "", nil, badArgs("unknown document %q; use cv or one of: %s", args.Document, strings.Join(slugs, ", "))
	}

	pdf := sitePath(found.Metadata["pdf"])
	title := found.Title + " (PDF)"
	if found.Kind == domain.SourceCV {
		title = localized(fr, "CV, English (PDF)", "CV, anglais (PDF)")
		if found.Lang == domain.LangFrench {
			title = localized(fr, "CV, French (PDF)", "CV, français (PDF)")
		}
	}
	text, err := toolJSON(documentResult{
		Title: title, URL: pdf, Lang: found.Lang,
		Note: "A download button for this file is shown under your answer.",
	})
	return text, []domain.ChatLink{{Title: title, URL: pdf, Kind: domain.ChatLinkDownload}}, err
}

// pageTextRunes caps a read_page result; the pages are a few KB each.
const pageTextRunes = 8000

type pageResult struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Lang  string `json:"lang"`
	Text  string `json:"text"`
}

// toolReadPage returns the About page or the CV from the corpus files, so the
// assistant knows them whether or not the search index (Voyage) is set up.
func toolReadPage(ctx context.Context, r *chatRun, input json.RawMessage) (string, []domain.ChatLink, error) {
	var args struct {
		Page string `json:"page"`
	}
	if err := decodeArgs(input, &args); err != nil {
		return "", nil, err
	}
	var kind domain.SourceKind
	switch strings.ToLower(strings.TrimSpace(args.Page)) {
	case "about":
		kind = domain.SourceAbout
	case "cv":
		kind = domain.SourceCV
	default:
		return "", nil, badArgs("page must be about or cv")
	}
	sources, err := r.s.loadCorpus(ctx)
	if err != nil {
		return "", nil, err
	}
	var found *domain.KnowledgeSource
	for i := range sources {
		src := &sources[i]
		if src.Kind != kind || src.Metadata["derived"] != "" {
			continue
		}
		// The visitor's language wins; any other language is the fallback.
		if found == nil || src.Lang == r.locale {
			found = src
		}
	}
	if found == nil {
		return "", nil, errToolFailed{msg: "that page is unavailable right now"}
	}
	url := sitePath(found.URL)
	if url == "" {
		url = "/about"
	}
	text, err := toolJSON(pageResult{Title: found.Title, URL: url, Lang: found.Lang, Text: truncateRunes(found.Text, pageTextRunes)})
	return text, []domain.ChatLink{{Title: found.Title, URL: url, Kind: domain.ChatLinkPage}}, err
}

// toolNinjaMove sends the mascot a random move to animate. Arguments are
// ignored, and the result carries nothing the model could repeat.
func toolNinjaMove(_ context.Context, r *chatRun, _ json.RawMessage) (string, []domain.ChatLink, error) {
	move := domain.NinjaMoves[rand.IntN(len(domain.NinjaMoves))]
	if err := r.emit(domain.ChatEvent{Kind: domain.ChatEventMove, Move: move}); err != nil {
		return "", nil, err
	}
	r.moved = true
	return `{"ok":true}`, nil, nil
}

// loadCorpus reads the corpus files for the publication and CV tools.
func (s *UseCases) loadCorpus(ctx context.Context) ([]domain.KnowledgeSource, error) {
	if s.corpus == nil {
		return nil, errToolFailed{msg: "the publication list is unavailable right now"}
	}
	return s.corpus.Load(ctx)
}

func projectURL(id int64) string { return "/projects/" + strconv.FormatInt(id, 10) }

// projectOneLine is the first sentence or line of the localized description,
// at most projectSummaryRunes long.
func projectOneLine(p domain.Project, locale string) string {
	_, description := splitPeriod(p.Description)
	if locale == domain.LangFrench {
		if fr := strings.TrimSpace(deref(p.DescriptionFr)); fr != "" {
			_, description = splitPeriod(fr)
		}
	}
	line, _, _ := strings.Cut(strings.TrimSpace(description), "\n")
	if i := strings.Index(line, ". "); i >= 0 {
		line = line[:i+1]
	}
	return truncateRunes(strings.TrimSpace(line), projectSummaryRunes)
}

// sitePath keeps a front matter path only when it is a site path.
func sitePath(p string) string {
	p = strings.TrimSpace(p)
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") {
		return ""
	}
	return p
}

// httpURL keeps a stored link only when it is an absolute http(s) URL.
func httpURL(link string) string {
	link = strings.TrimSpace(link)
	if strings.HasPrefix(link, "https://") || strings.HasPrefix(link, "http://") {
		return link
	}
	return ""
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:n])) + "…"
}

func localized(fr bool, en, frText string) string {
	if fr {
		return frText
	}
	return en
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
