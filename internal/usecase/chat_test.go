package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/ports"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
)

// chatDay is the fixed clock every chat test runs at.
var chatDay = time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)

var chatAnonymousSubject = domain.ChatSubject{Key: "ip:test", Tier: domain.ChatTierAnonymous}

// chatUsageSnapshot copies the store's usage rows so a test can prove that
// nothing was written.
func (s *knowledgeStore) chatUsageSnapshot() map[string]domain.ChatUsageCounts {
	s.mu.Lock()
	defer s.mu.Unlock()
	return maps.Clone(s.chatUsage)
}

// chatStep is one scripted model turn.
type chatStep struct {
	deltas []string
	text   string
	calls  []domain.ToolCall
	usage  domain.ChatTokenUsage
	err    error
}

// scriptedChatModel records every request it receives and replays the steps in
// order. With repeat, the last step answers every extra call.
type scriptedChatModel struct {
	mu       sync.Mutex
	requests []domain.ChatTurnRequest
	steps    []chatStep
	repeat   bool
}

func (m *scriptedChatModel) Stream(_ context.Context, req domain.ChatTurnRequest, onText func(string) error) (domain.ChatTurnResult, error) {
	m.mu.Lock()
	req.Messages = slices.Clone(req.Messages)
	m.requests = append(m.requests, req)
	n := len(m.requests) - 1
	m.mu.Unlock()

	if len(m.steps) == 0 {
		return domain.ChatTurnResult{}, errors.New("scripted model has no steps")
	}
	if n >= len(m.steps) && !m.repeat {
		return domain.ChatTurnResult{}, fmt.Errorf("unscripted model call %d", n+1)
	}
	step := m.steps[min(n, len(m.steps)-1)]
	for _, delta := range step.deltas {
		if err := onText(delta); err != nil {
			return domain.ChatTurnResult{}, err
		}
	}
	text := step.text
	if text == "" {
		text = strings.Join(step.deltas, "")
	}
	return domain.ChatTurnResult{Text: text, ToolCalls: step.calls, Usage: step.usage}, step.err
}

func (m *scriptedChatModel) requestList() []domain.ChatTurnRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.requests)
}

func (m *scriptedChatModel) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.requests)
}

// chatEventLog captures the events Chat streams.
type chatEventLog struct {
	events []domain.ChatEvent
}

func (l *chatEventLog) emit(e domain.ChatEvent) error {
	l.events = append(l.events, e)
	return nil
}

func (l *chatEventLog) of(kind domain.ChatEventKind) []domain.ChatEvent {
	var out []domain.ChatEvent
	for _, e := range l.events {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func (l *chatEventLog) text() string {
	var b strings.Builder
	for _, e := range l.of(domain.ChatEventText) {
		b.WriteString(e.Text)
	}
	return b.String()
}

func newChatUseCases(store *knowledgeStore, model ports.ChatModel, settings usecase.ChatSettings) usecase.Service {
	settings.Model = model
	if settings.Now == nil {
		settings.Now = func() time.Time { return chatDay }
	}
	return usecase.New(usecase.Dependencies{
		Repositories:    store,
		UnitOfWork:      store,
		KnowledgeCorpus: staticCorpus(corpus()),
		Chat:            settings,
	})
}

func chatRequest(subject domain.ChatSubject, locale, pagePath string, texts ...string) domain.ChatInput {
	in := domain.ChatInput{Subject: subject, Locale: locale, PagePath: pagePath}
	for _, text := range texts {
		in.Messages = append(in.Messages, domain.ChatMessage{Role: domain.ChatRoleUser, Text: text})
	}
	return in
}

func chatToolCall(id, name, input string) domain.ToolCall {
	return domain.ToolCall{ID: id, Name: name, Input: json.RawMessage(input)}
}

// chatToolResults collects the tool results in a request's messages.
func chatToolResults(req domain.ChatTurnRequest) []domain.ToolResult {
	var out []domain.ToolResult
	for _, m := range req.Messages {
		out = append(out, m.ToolResults...)
	}
	return out
}

func chatProjects() []domain.Project {
	return []domain.Project{
		{ID: 1, Name: "Lora", Featured: true, Description: "Offline-first community platform.", DescriptionFr: strPtr("Une plateforme communautaire hors ligne.")},
		{ID: 7, Name: "AcadAi", Description: "A RAG assistant."},
	}
}

func TestChatToolRoundThenAnswer(t *testing.T) {
	store := newKnowledgeStore()
	store.projects = chatProjects()
	model := &scriptedChatModel{steps: []chatStep{
		{calls: []domain.ToolCall{chatToolCall("call-1", "list_projects", `{}`)}},
		{deltas: []string{"Lora is Alex's ", "offline-first platform. ", "See [Lora](/projects/1)."}},
	}}
	svc := newChatUseCases(store, model, usecase.ChatSettings{DailyMessagesAnonymous: 5})
	log := &chatEventLog{}

	if err := svc.Chat(context.Background(), chatRequest(chatAnonymousSubject, "en", "", "What has Alex built?"), log.emit); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	requests := model.requestList()
	if len(requests) != 2 {
		t.Fatalf("model calls = %d, want 2", len(requests))
	}
	if !strings.Contains(requests[0].System, "You are the Ninjassistant") {
		t.Errorf("English system prompt missing: %q", requests[0].System)
	}
	if !slices.ContainsFunc(requests[0].Tools, func(spec domain.ToolSpec) bool { return spec.Name == "list_projects" }) {
		t.Errorf("list_projects not offered to the model: %+v", requests[0].Tools)
	}

	tools := log.of(domain.ChatEventTool)
	if len(tools) != 1 || tools[0].Tool != "list_projects" {
		t.Fatalf("tool events = %+v, want one list_projects", tools)
	}
	// The leak guard holds a turn's first sentence until a second one ends;
	// this answer's second sentence ends with the turn, so all of it is one
	// event at the turn's end.
	text := log.of(domain.ChatEventText)
	if len(text) != 1 {
		t.Errorf("text events = %d, want 1", len(text))
	}
	if got := log.text(); !strings.Contains(got, "/projects/1") {
		t.Errorf("streamed answer %q lacks the project link", got)
	}
	links := log.of(domain.ChatEventLinks)
	if len(links) != 1 {
		t.Fatalf("links events = %d, want 1", len(links))
	}
	if len(links[0].Links) != 1 || links[0].Links[0].URL != "/projects/1" || links[0].Links[0].Title != "Lora" {
		t.Errorf("links = %+v, want the Lora project link", links[0].Links)
	}
	done := log.of(domain.ChatEventDone)
	if len(done) != 1 {
		t.Fatalf("done events = %d, want 1", len(done))
	}
	if done[0].Remaining == nil || *done[0].Remaining != 4 {
		t.Errorf("done remaining = %v, want 4 of 5 for the anonymous subject", done[0].Remaining)
	}
	if last := log.events[len(log.events)-1]; last.Kind != domain.ChatEventDone {
		t.Errorf("last event = %s, want done", last.Kind)
	}

	// The second request carries the assistant's tool call and the tool result.
	results := chatToolResults(requests[1])
	if len(results) != 1 {
		t.Fatalf("tool results in the second request = %+v, want 1", results)
	}
	if results[0].CallID != "call-1" || results[0].IsError {
		t.Errorf("tool result = %+v", results[0])
	}
	if !strings.Contains(results[0].Content, "Lora") || !strings.Contains(results[0].Content, "/projects/1") {
		t.Errorf("tool result content = %q, want the project", results[0].Content)
	}
	var sawCall bool
	for _, m := range requests[1].Messages {
		if m.Role == domain.ChatRoleAssistant && len(m.ToolCalls) == 1 && m.ToolCalls[0].Name == "list_projects" && m.ToolCalls[0].ID == "call-1" {
			sawCall = true
		}
	}
	if !sawCall {
		t.Errorf("second request lacks the assistant tool-call turn: %+v", requests[1].Messages)
	}
}

func TestChatToolLoopCapsAtFourModelCalls(t *testing.T) {
	store := newKnowledgeStore()
	store.projects = chatProjects()
	model := &scriptedChatModel{
		steps:  []chatStep{{calls: []domain.ToolCall{chatToolCall("call-1", "list_projects", `{}`)}}},
		repeat: true,
	}
	svc := newChatUseCases(store, model, usecase.ChatSettings{})
	log := &chatEventLog{}

	if err := svc.Chat(context.Background(), chatRequest(chatAnonymousSubject, "en", "", "What has Alex built?"), log.emit); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	requests := model.requestList()
	if len(requests) != 4 {
		t.Fatalf("model calls = %d, want the 4-call cap", len(requests))
	}
	for i, req := range requests {
		if want := i == len(requests)-1; req.NoToolUse != want {
			t.Errorf("call %d NoToolUse = %v, want %v", i+1, req.NoToolUse, want)
		}
	}
	if got := len(requests[3].Messages); got != 7 {
		t.Errorf("last request has %d messages, want 7 (the user turn plus 3 tool rounds)", got)
	}
	if got := len(log.of(domain.ChatEventTool)); got != 3 {
		t.Errorf("tool rounds = %d, want 3", got)
	}
	if got := log.text(); !strings.Contains(got, "could not put an answer together") {
		t.Errorf("streamed text %q, want the fallback answer", got)
	}
	if got := len(log.of(domain.ChatEventLinks)); got != 1 {
		t.Errorf("links events = %d, want 1", got)
	}
	if got := len(log.of(domain.ChatEventDone)); got != 1 {
		t.Errorf("done events = %d, want 1", got)
	}
}

func TestChatToolFailuresBecomeErrorResults(t *testing.T) {
	store := newKnowledgeStore()
	store.projects = chatProjects()
	model := &scriptedChatModel{steps: []chatStep{
		{calls: []domain.ToolCall{
			chatToolCall("call-unknown", "no_such_tool", `{}`),
			chatToolCall("call-bad-args", "get_project", `{"id":`),
		}},
		{deltas: []string{"I could not look that up."}},
	}}
	svc := newChatUseCases(store, model, usecase.ChatSettings{})
	log := &chatEventLog{}

	if err := svc.Chat(context.Background(), chatRequest(chatAnonymousSubject, "en", "", "Tell me about a project"), log.emit); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	requests := model.requestList()
	if len(requests) != 2 {
		t.Fatalf("model calls = %d, want 2", len(requests))
	}
	results := chatToolResults(requests[1])
	if len(results) != 2 {
		t.Fatalf("tool results = %+v, want 2", results)
	}
	byCall := make(map[string]domain.ToolResult, len(results))
	for _, result := range results {
		byCall[result.CallID] = result
	}
	if result, ok := byCall["call-unknown"]; !ok || !result.IsError || !strings.Contains(result.Content, "unknown tool") {
		t.Errorf("unknown tool result = %+v, want an error naming the unknown tool", result)
	}
	if result, ok := byCall["call-bad-args"]; !ok || !result.IsError || !strings.Contains(result.Content, "invalid arguments") {
		t.Errorf("bad arguments result = %+v, want an invalid arguments error", result)
	}
	// Only real tools are announced.
	tools := log.of(domain.ChatEventTool)
	if len(tools) != 1 || tools[0].Tool != "get_project" {
		t.Errorf("tool events = %+v, want one get_project", tools)
	}
	if got := len(log.of(domain.ChatEventDone)); got != 1 {
		t.Errorf("done events = %d, want 1", got)
	}
}

func TestChatDailyLimitStopsBeforeTheModel(t *testing.T) {
	for _, used := range []int64{5, 6} {
		t.Run(fmt.Sprintf("%d_requests", used), func(t *testing.T) {
			store := newKnowledgeStore()
			subject := chatAnonymousSubject
			store.setUsage(chatDay, subject.Key, domain.ChatUsageCounts{Requests: used, InputTokens: 100})
			model := &scriptedChatModel{steps: []chatStep{{deltas: []string{"never sent"}}}}
			svc := newChatUseCases(store, model, usecase.ChatSettings{DailyMessagesAnonymous: 5})
			before := store.chatUsageSnapshot()
			log := &chatEventLog{}

			err := svc.Chat(context.Background(), chatRequest(subject, "en", "", "Hello"), log.emit)

			var limited *domain.ErrChatLimited
			if !errors.As(err, &limited) {
				t.Fatalf("Chat error = %v, want *domain.ErrChatLimited", err)
			}
			if limited.Reason != domain.ChatLimitDaily {
				t.Errorf("reason = %q, want %q", limited.Reason, domain.ChatLimitDaily)
			}
			if limited.RetryAfter <= 0 || limited.RetryAfter > 24*time.Hour {
				t.Errorf("RetryAfter = %v, want a positive duration up to one day", limited.RetryAfter)
			}
			if got := model.callCount(); got != 0 {
				t.Errorf("model calls = %d, want 0", got)
			}
			if got := len(log.events); got != 0 {
				t.Errorf("events = %+v, want none", log.events)
			}
			if after := store.chatUsageSnapshot(); !maps.Equal(before, after) {
				t.Errorf("chat usage changed from %v to %v", before, after)
			}
		})
	}
}

func TestChatGlobalTokenBudgetStopsBeforeTheModel(t *testing.T) {
	store := newKnowledgeStore()
	store.setUsage(chatDay, domain.ChatGlobalSubject, domain.ChatUsageCounts{InputTokens: 900, OutputTokens: 100})
	model := &scriptedChatModel{steps: []chatStep{{deltas: []string{"never sent"}}}}
	svc := newChatUseCases(store, model, usecase.ChatSettings{DailyTokenBudget: 1000})
	log := &chatEventLog{}

	err := svc.Chat(context.Background(), chatRequest(chatAnonymousSubject, "en", "", "Hello"), log.emit)

	var limited *domain.ErrChatLimited
	if !errors.As(err, &limited) {
		t.Fatalf("Chat error = %v, want *domain.ErrChatLimited", err)
	}
	if limited.Reason != domain.ChatLimitBudget {
		t.Errorf("reason = %q, want %q", limited.Reason, domain.ChatLimitBudget)
	}
	if limited.RetryAfter <= 0 {
		t.Errorf("RetryAfter = %v, want a positive duration", limited.RetryAfter)
	}
	if got := model.callCount(); got != 0 {
		t.Errorf("model calls = %d, want 0", got)
	}
	if got := len(log.events); got != 0 {
		t.Errorf("events = %+v, want none", log.events)
	}
}

func TestChatRecordsUsage(t *testing.T) {
	store := newKnowledgeStore()
	subject := chatAnonymousSubject
	model := &scriptedChatModel{steps: []chatStep{{
		deltas: []string{"Hello!"},
		usage:  domain.ChatTokenUsage{InputTokens: 100, OutputTokens: 20, CacheReadTokens: 30, CacheWriteTokens: 10},
	}}}
	svc := newChatUseCases(store, model, usecase.ChatSettings{DailyMessagesAnonymous: 5})

	if err := svc.Chat(context.Background(), chatRequest(subject, "en", "", "Hello"), (&chatEventLog{}).emit); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	want := domain.ChatUsageCounts{Requests: 1, InputTokens: 140, OutputTokens: 20}
	for _, key := range []string{subject.Key, domain.ChatGlobalSubject} {
		got, err := store.GetUsage(context.Background(), chatDay, key)
		if err != nil {
			t.Fatalf("GetUsage(%q): %v", key, err)
		}
		if got != want {
			t.Errorf("usage for %q = %+v, want %+v (input counts cache tokens)", key, got, want)
		}
	}
}

func TestChatRecordsUsageWhenTheModelFails(t *testing.T) {
	store := newKnowledgeStore()
	subject := chatAnonymousSubject
	boom := errors.New("stream aborted")
	model := &scriptedChatModel{steps: []chatStep{{
		deltas: []string{"Partial "},
		usage:  domain.ChatTokenUsage{InputTokens: 7, OutputTokens: 2},
		err:    boom,
	}}}
	svc := newChatUseCases(store, model, usecase.ChatSettings{})
	log := &chatEventLog{}

	err := svc.Chat(context.Background(), chatRequest(subject, "en", "", "Hello"), log.emit)

	if !errors.Is(err, boom) {
		t.Fatalf("Chat error = %v, want %v", err, boom)
	}
	if got := log.text(); got != "Partial " {
		t.Errorf("streamed text = %q, want the deltas sent before the failure", got)
	}
	want := domain.ChatUsageCounts{Requests: 1, InputTokens: 7, OutputTokens: 2}
	for _, key := range []string{subject.Key, domain.ChatGlobalSubject} {
		got, err := store.GetUsage(context.Background(), chatDay, key)
		if err != nil {
			t.Fatalf("GetUsage(%q): %v", key, err)
		}
		if got != want {
			t.Errorf("usage for %q = %+v, want %+v despite the model error", key, got, want)
		}
	}
}

// A run the visitor never saw and that spent no tokens costs nothing: the
// counters stay untouched.
func TestChatFailureWithoutOutputCostsNothing(t *testing.T) {
	store := newKnowledgeStore()
	boom := errors.New("stream aborted")
	model := &scriptedChatModel{steps: []chatStep{{err: boom}}}
	svc := newChatUseCases(store, model, usecase.ChatSettings{})

	if err := svc.Chat(context.Background(), chatRequest(chatAnonymousSubject, "en", "", "Hello"), (&chatEventLog{}).emit); !errors.Is(err, boom) {
		t.Fatalf("Chat error = %v, want %v", err, boom)
	}
	if after := store.chatUsageSnapshot(); len(after) != 0 {
		t.Errorf("usage recorded %+v, want none", after)
	}
}

// Tokens were spent but no text delta reached the visitor: tokens are counted
// against the day, but the request is free.
func TestChatTokensWithoutDeliveryCostNoMessage(t *testing.T) {
	store := newKnowledgeStore()
	boom := errors.New("stream aborted")
	model := &scriptedChatModel{steps: []chatStep{{
		usage: domain.ChatTokenUsage{InputTokens: 50, OutputTokens: 5},
		err:   boom,
	}}}
	svc := newChatUseCases(store, model, usecase.ChatSettings{})

	if err := svc.Chat(context.Background(), chatRequest(chatAnonymousSubject, "en", "", "Hello"), (&chatEventLog{}).emit); !errors.Is(err, boom) {
		t.Fatalf("Chat error = %v, want %v", err, boom)
	}
	want := domain.ChatUsageCounts{Requests: 0, InputTokens: 50, OutputTokens: 5}
	for _, key := range []string{chatAnonymousSubject.Key, domain.ChatGlobalSubject} {
		got, err := store.GetUsage(context.Background(), chatDay, key)
		if err != nil {
			t.Fatalf("GetUsage(%q): %v", key, err)
		}
		if got != want {
			t.Errorf("usage for %q = %+v, want tokens only", key, got)
		}
	}
}

// The client vanished on the very first text event: nothing was written, so
// nothing is recorded.
func TestChatClientGoneWithoutOutputCostsNothing(t *testing.T) {
	store := newKnowledgeStore()
	model := &scriptedChatModel{steps: []chatStep{{deltas: []string{"Half "}}}}
	svc := newChatUseCases(store, model, usecase.ChatSettings{})
	emitter := func(domain.ChatEvent) error { return errors.New("client gone") }

	if err := svc.Chat(context.Background(), chatRequest(chatAnonymousSubject, "en", "", "Hello"), emitter); err == nil {
		t.Fatal("Chat error = nil, want the emit error")
	}
	if after := store.chatUsageSnapshot(); len(after) != 0 {
		t.Errorf("usage recorded %+v, want none", after)
	}
}

func TestChatFrenchLocale(t *testing.T) {
	store := newKnowledgeStore()
	store.projects = chatProjects()
	model := &scriptedChatModel{steps: []chatStep{
		{calls: []domain.ToolCall{chatToolCall("call-1", "list_projects", `{}`)}},
		{deltas: []string{"Le projet [Lora](/projects/1) est une plateforme communautaire hors ligne."}},
	}}
	svc := newChatUseCases(store, model, usecase.ChatSettings{})
	log := &chatEventLog{}

	in := chatRequest(chatAnonymousSubject, "fr", "/fr/projects/1", "Parle-moi de ce projet")
	if err := svc.Chat(context.Background(), in, log.emit); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	requests := model.requestList()
	if len(requests) != 2 {
		t.Fatalf("model calls = %d, want 2", len(requests))
	}
	req := requests[0]
	if !strings.Contains(req.System, "Vous êtes le Ninjassistant") {
		t.Errorf("French system prompt missing: %q", req.System)
	}
	if strings.Contains(req.System, "You are the Ninjassistant") {
		t.Error("English system prompt sent to a French visitor")
	}
	for _, want := range []string{"Le visiteur est sur la page /projects/1", "d'id 1", "get_project"} {
		if !strings.Contains(req.SystemContext, want) {
			t.Errorf("SystemContext %q lacks %q", req.SystemContext, want)
		}
	}

	results := chatToolResults(requests[1])
	if len(results) != 1 {
		t.Fatalf("tool results = %+v, want 1", results)
	}
	if !strings.Contains(results[0].Content, "Une plateforme communautaire hors ligne.") {
		t.Errorf("French tool result = %q, want the French description", results[0].Content)
	}
	if strings.Contains(results[0].Content, "Offline-first") {
		t.Errorf("English description sent to the French tool: %q", results[0].Content)
	}
	links := log.of(domain.ChatEventLinks)
	if len(links) != 1 || len(links[0].Links) != 1 || links[0].Links[0].URL != "/projects/1" {
		t.Errorf("links = %+v, want the Lora project link", links)
	}
}

func TestChatLinksOnlyComeFromTools(t *testing.T) {
	t.Run("made up URL", func(t *testing.T) {
		store := newKnowledgeStore()
		model := &scriptedChatModel{steps: []chatStep{{deltas: []string{"See [a project](/projects/999)."}}}}
		svc := newChatUseCases(store, model, usecase.ChatSettings{})
		log := &chatEventLog{}

		if err := svc.Chat(context.Background(), chatRequest(chatAnonymousSubject, "en", "", "Show me a project"), log.emit); err != nil {
			t.Fatalf("Chat: %v", err)
		}
		links := log.of(domain.ChatEventLinks)
		if len(links) != 1 {
			t.Fatalf("links events = %d, want 1", len(links))
		}
		if links[0].Links == nil {
			t.Error("links = null, want an empty array")
		}
		if len(links[0].Links) != 0 {
			t.Errorf("links = %+v, want none: no tool returned that URL", links[0].Links)
		}
	})

	t.Run("tool returned URL", func(t *testing.T) {
		store := newKnowledgeStore()
		store.projects = chatProjects()
		model := &scriptedChatModel{steps: []chatStep{
			{calls: []domain.ToolCall{chatToolCall("call-1", "list_projects", `{}`)}},
			{deltas: []string{"[Lora](/projects/1) is real; [this](/projects/999) is not."}},
		}}
		svc := newChatUseCases(store, model, usecase.ChatSettings{})
		log := &chatEventLog{}

		if err := svc.Chat(context.Background(), chatRequest(chatAnonymousSubject, "en", "", "Show me a project"), log.emit); err != nil {
			t.Fatalf("Chat: %v", err)
		}
		links := log.of(domain.ChatEventLinks)
		if len(links) != 1 {
			t.Fatalf("links events = %d, want 1", len(links))
		}
		if len(links[0].Links) != 1 || links[0].Links[0].URL != "/projects/1" {
			t.Errorf("links = %+v, want only the tool-returned /projects/1", links[0].Links)
		}
	})
}

func TestChatSearchUnavailableWithoutEmbedder(t *testing.T) {
	store := newKnowledgeStore()
	model := &scriptedChatModel{steps: []chatStep{
		{calls: []domain.ToolCall{chatToolCall("call-1", "search_site", `{"query":"speech"}`)}},
		{deltas: []string{"Search is unavailable; see the projects page."}},
	}}
	svc := newChatUseCases(store, model, usecase.ChatSettings{})
	if svc.KnowledgeEnabled() {
		t.Fatal("knowledge is enabled without an embedder")
	}
	log := &chatEventLog{}

	if err := svc.Chat(context.Background(), chatRequest(chatAnonymousSubject, "en", "", "What does Alex research?"), log.emit); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	requests := model.requestList()
	if len(requests) != 2 {
		t.Fatalf("model calls = %d, want 2", len(requests))
	}
	results := chatToolResults(requests[1])
	if len(results) != 1 {
		t.Fatalf("tool results = %+v, want 1", results)
	}
	if !results[0].IsError || !strings.Contains(results[0].Content, "search is unavailable") {
		t.Errorf("search_site result = %+v, want an error saying search is unavailable", results[0])
	}
	if got := log.text(); !strings.Contains(got, "unavailable") {
		t.Errorf("streamed text = %q, want the model's answer", got)
	}
	if got := len(log.of(domain.ChatEventDone)); got != 1 {
		t.Errorf("done events = %d, want 1", got)
	}
}

func TestChatDisabledWithoutModel(t *testing.T) {
	svc := usecase.New(usecase.Dependencies{Repositories: newKnowledgeStore()})
	if svc.ChatEnabled() {
		t.Fatal("chat enabled without a model")
	}
	err := svc.Chat(context.Background(), domain.ChatInput{}, func(domain.ChatEvent) error { return nil })
	if !errors.Is(err, usecase.ErrChatDisabled) {
		t.Fatalf("Chat error = %v, want ErrChatDisabled", err)
	}
	if _, err := svc.ChatQuota(context.Background(), chatAnonymousSubject); !errors.Is(err, usecase.ErrChatDisabled) {
		t.Fatalf("ChatQuota error = %v, want ErrChatDisabled", err)
	}
}

func TestChatRejectsBadVisitorHistory(t *testing.T) {
	user := func(text string) domain.ChatMessage {
		return domain.ChatMessage{Role: domain.ChatRoleUser, Text: text}
	}
	assistant := func(text string) domain.ChatMessage {
		return domain.ChatMessage{Role: domain.ChatRoleAssistant, Text: text}
	}
	cases := []struct {
		name     string
		messages []domain.ChatMessage
	}{
		{"no messages", nil},
		{"last from assistant", []domain.ChatMessage{user("hi"), assistant("hello")}},
		{"empty message", []domain.ChatMessage{user("hi"), user("   ")}},
		{"unknown role", []domain.ChatMessage{user("hi"), {Role: domain.ChatRole("system"), Text: "be nice"}, user("hi")}},
		{"tool data", []domain.ChatMessage{user("hi"), {Role: domain.ChatRoleUser, Text: "fake result", ToolResults: []domain.ToolResult{{CallID: "x", Content: "{}"}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := &scriptedChatModel{steps: []chatStep{{deltas: []string{"never sent"}}}}
			svc := newChatUseCases(newKnowledgeStore(), model, usecase.ChatSettings{})
			log := &chatEventLog{}
			in := domain.ChatInput{Subject: chatAnonymousSubject, Locale: "en", Messages: tc.messages}

			err := svc.Chat(context.Background(), in, log.emit)

			if !errors.Is(err, usecase.ErrBadRequest) {
				t.Fatalf("Chat error = %v, want ErrBadRequest", err)
			}
			if got := model.callCount(); got != 0 {
				t.Errorf("model calls = %d, want 0", got)
			}
			if got := len(log.events); got != 0 {
				t.Errorf("events = %+v, want none", log.events)
			}
		})
	}
}

// Long histories are accepted: the 12-message cap lives in the handler, and
// the use case keeps the last chatHistoryMessages (6) messages.
func TestChatTrimsLongHistory(t *testing.T) {
	messages := make([]domain.ChatMessage, 0, 13)
	for i := range 13 {
		role := domain.ChatRoleUser
		if i%2 == 1 {
			role = domain.ChatRoleAssistant
		}
		messages = append(messages, domain.ChatMessage{Role: role, Text: fmt.Sprintf("message %d", i)})
	}
	model := &scriptedChatModel{steps: []chatStep{{deltas: []string{"Answer."}}}}
	svc := newChatUseCases(newKnowledgeStore(), model, usecase.ChatSettings{})
	in := domain.ChatInput{Subject: chatAnonymousSubject, Locale: "en", Messages: messages}

	if err := svc.Chat(context.Background(), in, (&chatEventLog{}).emit); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	requests := model.requestList()
	if len(requests) != 1 {
		t.Fatalf("model calls = %d, want 1", len(requests))
	}
	got := requests[0].Messages
	if len(got) == 0 || len(got) > 6 {
		t.Fatalf("history = %d messages, want 1 to 6", len(got))
	}
	if got[0].Role != domain.ChatRoleUser || got[len(got)-1].Role != domain.ChatRoleUser {
		t.Errorf("history roles = %v, want a user turn at each end", got)
	}
}

// downloadCorpus has the PDF paths the get_document tool hands out.
func downloadCorpus() []domain.KnowledgeSource {
	return []domain.KnowledgeSource{
		{Kind: domain.SourcePublication, Ref: "cerc2026-tonal", Lang: "en", Title: "Tonal paper", URL: "/publications", Text: "Paper.",
			Metadata: map[string]string{"pdf": "/papers/tonal.pdf", "date": "2026-03"}},
		{Kind: domain.SourceCV, Ref: "cv", Lang: "en", Title: "CV", URL: "/about", Text: "CV.",
			Metadata: map[string]string{"pdf": "/assets/docs/Alex_Abena_cv-en.pdf"}},
		{Kind: domain.SourceCV, Ref: "cv", Lang: "fr", Title: "CV", URL: "/about", Text: "CV.",
			Metadata: map[string]string{"pdf": "/assets/docs/Alex_Abena_cv-fr.pdf"}},
	}
}

func runDocumentChat(t *testing.T, locale string, steps ...chatStep) (*chatEventLog, *scriptedChatModel) {
	t.Helper()
	model := &scriptedChatModel{steps: steps}
	svc := usecase.New(usecase.Dependencies{
		Repositories:    newKnowledgeStore(),
		UnitOfWork:      newKnowledgeStore(),
		KnowledgeCorpus: staticCorpus(downloadCorpus()),
		Chat:            usecase.ChatSettings{Model: model, Now: func() time.Time { return chatDay }},
	})
	log := &chatEventLog{}
	if err := svc.Chat(context.Background(), chatRequest(chatAnonymousSubject, locale, "", "CV please"), log.emit); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	return log, model
}

func TestChatGetDocumentAlwaysSendsTheDownload(t *testing.T) {
	// No lang: the visitor's locale picks the French CV, and the answer never
	// names the path, yet the download reaches the links event.
	log, _ := runDocumentChat(t, "fr",
		chatStep{calls: []domain.ToolCall{chatToolCall("c1", "get_document", `{"document":"cv"}`)}},
		chatStep{deltas: []string{"Le bouton est juste en dessous."}},
	)
	links := log.of(domain.ChatEventLinks)
	if len(links) != 1 || len(links[0].Links) != 1 {
		t.Fatalf("links = %+v, want the one download", links)
	}
	got := links[0].Links[0]
	if got.URL != "/assets/docs/Alex_Abena_cv-fr.pdf" || got.Kind != domain.ChatLinkDownload {
		t.Fatalf("link = %+v, want the French CV as a download", got)
	}

	// An explicit lang wins over the locale.
	log, _ = runDocumentChat(t, "fr",
		chatStep{calls: []domain.ToolCall{chatToolCall("c1", "get_document", `{"document":"cv","lang":"en"}`)}},
		chatStep{deltas: []string{"Voilà."}},
	)
	if got := log.of(domain.ChatEventLinks)[0].Links; len(got) != 1 || got[0].URL != "/assets/docs/Alex_Abena_cv-en.pdf" {
		t.Fatalf("links = %+v, want the English CV", got)
	}
}

func TestChatGetDocumentUpgradesAListedPaperPDF(t *testing.T) {
	log, _ := runDocumentChat(t, "en",
		chatStep{calls: []domain.ToolCall{chatToolCall("c1", "list_publications", `{}`)}},
		chatStep{calls: []domain.ToolCall{chatToolCall("c2", "get_document", `{"document":"cerc2026-tonal"}`)}},
		chatStep{deltas: []string{"Here it is."}},
	)
	links := log.of(domain.ChatEventLinks)[0].Links
	if len(links) != 1 || links[0].URL != "/papers/tonal.pdf" || links[0].Kind != domain.ChatLinkDownload {
		t.Fatalf("links = %+v, want only the paper PDF as a download", links)
	}
}

func TestChatGetDocumentRejectsUnknownDocuments(t *testing.T) {
	_, model := runDocumentChat(t, "en",
		chatStep{calls: []domain.ToolCall{chatToolCall("c1", "get_document", `{"document":"../../etc/passwd"}`)}},
		chatStep{deltas: []string{"Sorry."}},
	)
	results := chatToolResults(model.requestList()[1])
	if len(results) != 1 || !results[0].IsError || !strings.Contains(results[0].Content, "cerc2026-tonal") {
		t.Fatalf("tool result = %+v, want an error listing the valid documents", results)
	}
}

func TestChatNinjaMoveSendsTheMoveEvent(t *testing.T) {
	// Whatever the model passes, the server picks a known move.
	log, model := runDocumentChat(t, "en",
		chatStep{calls: []domain.ToolCall{chatToolCall("c1", "ninja_move", `{"move":"moonwalk"}`)}},
		chatStep{deltas: []string{"Ninjas have no names."}},
	)
	moves := log.of(domain.ChatEventMove)
	if len(moves) != 1 || !slices.Contains(domain.NinjaMoves, moves[0].Move) {
		t.Fatalf("move events = %+v, want one known move", moves)
	}
	if !strings.Contains(model.requestList()[0].System, "Ninjassistant") {
		t.Error("the system prompt does not introduce the Ninjassistant")
	}

	// Nothing the model sees names or describes a move, so it cannot narrate one.
	results := chatToolResults(model.requestList()[1])
	if len(results) != 1 || results[0].IsError || results[0].Content != `{"ok":true}` {
		t.Fatalf("tool result = %+v, want a bare ok", results)
	}
	for _, req := range model.requestList() {
		for _, spec := range req.Tools {
			if spec.Name != "ninja_move" {
				continue
			}
			text := spec.Description + fmt.Sprint(spec.InputSchema)
			for _, move := range domain.NinjaMoves {
				if strings.Contains(text, move) {
					t.Errorf("ninja_move spec leaks the move %q: %s", move, text)
				}
			}
		}
	}
}

func TestChatEndsWhenTheAnswerCameWithOnlyAMove(t *testing.T) {
	// The model answered and asked for a move in the same turn: no second
	// turn, so the answer is never written twice.
	log, model := runDocumentChat(t, "en",
		chatStep{deltas: []string{"Ninjas have no names."}, calls: []domain.ToolCall{chatToolCall("c1", "ninja_move", `{}`)}},
		chatStep{deltas: []string{"Ninjas have no names."}},
	)
	if got := model.callCount(); got != 1 {
		t.Fatalf("model calls = %d, want 1", got)
	}
	if got := log.text(); got != "Ninjas have no names." {
		t.Fatalf("answer = %q, want it once", got)
	}
	if len(log.of(domain.ChatEventMove)) != 1 {
		t.Fatal("the move was not sent")
	}
}

func TestChatDropsSentencesThatNameATool(t *testing.T) {
	log, _ := runDocumentChat(t, "fr",
		chatStep{deltas: []string{"Le visiteur demande une figure : j'appelle ", "ninja_move. ", "Voilà !"}, calls: []domain.ToolCall{chatToolCall("c1", "ninja_move", `{}`)}},
	)
	if got := log.text(); got != "Voilà !" {
		t.Fatalf("answer = %q, want only the sentence without the tool name", got)
	}

	// Nothing left after the filter: a playful line instead of an empty answer.
	log, _ = runDocumentChat(t, "en",
		chatStep{deltas: []string{"Calling ninja_move now."}, calls: []domain.ToolCall{chatToolCall("c1", "ninja_move", `{}`)}},
	)
	if got := log.text(); !strings.HasPrefix(got, "Blink and you'll miss it.") {
		t.Fatalf("answer = %q, want the move fallback", got)
	}
}

func TestChatReadPageReturnsTheCorpusPage(t *testing.T) {
	// No search index needed: the page comes straight from the corpus, in
	// the visitor's language.
	log, model := runDocumentChat(t, "fr",
		chatStep{calls: []domain.ToolCall{chatToolCall("c1", "read_page", `{"page":"cv"}`)}},
		chatStep{deltas: []string{"Voici son parcours."}},
	)
	results := chatToolResults(model.requestList()[1])
	if len(results) != 1 || results[0].IsError || !strings.Contains(results[0].Content, `"lang":"fr"`) || !strings.Contains(results[0].Content, `"url":"/about"`) {
		t.Fatalf("tool result = %+v, want the French CV page", results)
	}
	if !strings.Contains(model.requestList()[0].System, "read_page") {
		t.Error("the prompt does not point to read_page")
	}
	_ = log

	// A page the corpus lacks is an error result, not a failed chat.
	_, model = runDocumentChat(t, "en",
		chatStep{calls: []domain.ToolCall{chatToolCall("c1", "read_page", `{"page":"about"}`)}},
		chatStep{deltas: []string{"Sorry."}},
	)
	if results := chatToolResults(model.requestList()[1]); len(results) != 1 || !results[0].IsError {
		t.Fatalf("tool result = %+v, want an error", results)
	}
}

func TestChatDropsALoneAnnouncementBeforeALookup(t *testing.T) {
	log, _ := runDocumentChat(t, "fr",
		chatStep{deltas: []string{"Je vais consulter le CV ", "d'Alex."}, calls: []domain.ToolCall{chatToolCall("c1", "read_page", `{"page":"cv"}`)}},
		chatStep{deltas: []string{"Alex parle français ", "et anglais. ", "Il code en Go."}},
	)
	if got := log.text(); got != "Alex parle français et anglais. Il code en Go." {
		t.Fatalf("answer = %q, want it without the announcement", got)
	}

	// Longer text before a lookup is part of the answer and stays.
	log, _ = runDocumentChat(t, "en",
		chatStep{deltas: []string{"Alex studies in Grenoble. He also speaks French. "}, calls: []domain.ToolCall{chatToolCall("c1", "read_page", `{"page":"cv"}`)}},
		chatStep{deltas: []string{"He codes in Go."}},
	)
	if got := log.text(); got != "Alex studies in Grenoble. He also speaks French. \n\nHe codes in Go." {
		t.Fatalf("answer = %q, want both turns", got)
	}
}
