package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// Chat types for the site assistant. They are provider-neutral: the
// OpenRouter adapter maps them to the OpenAI Chat Completions wire format,
// and nothing here names a provider.

// ChatRole is who wrote a message.
type ChatRole string

const (
	ChatRoleUser      ChatRole = "user"
	ChatRoleAssistant ChatRole = "assistant"
)

// ChatMessage is one turn sent to the model. Visitor turns carry Text only;
// ToolCalls (assistant) and ToolResults (user) appear only in the turns the
// tool loop adds, so a client can never forge a tool result.
type ChatMessage struct {
	Role        ChatRole
	Text        string
	ToolCalls   []ToolCall
	ToolResults []ToolResult
}

// ToolSpec describes one tool to the model. InputSchema is a JSON schema
// object (type, properties, required, ...).
type ToolSpec struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// ToolCall is the model asking to run a tool.
type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// ToolResult answers one ToolCall. Content is JSON text; IsError marks a
// failed call (unknown tool, bad arguments, lookup failure).
type ToolResult struct {
	CallID  string
	Content string
	IsError bool
}

// ChatTurnRequest is one model call.
type ChatTurnRequest struct {
	// System is the stable part of the system prompt; the adapter marks it
	// for prompt caching.
	System string
	// SystemContext is appended after System and not cached (the page the
	// visitor is on). It may be empty.
	SystemContext string
	Messages      []ChatMessage
	Tools         []ToolSpec
	MaxTokens     int
	// NoToolUse keeps Tools defined (so earlier tool turns stay valid and the
	// cached prefix is unchanged) but tells the model to answer in text.
	NoToolUse bool
}

// ChatTokenUsage is what one or more model calls consumed.
type ChatTokenUsage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
}

// Add returns the sum of u and o.
func (u ChatTokenUsage) Add(o ChatTokenUsage) ChatTokenUsage {
	return ChatTokenUsage{
		InputTokens:      u.InputTokens + o.InputTokens,
		OutputTokens:     u.OutputTokens + o.OutputTokens,
		CacheReadTokens:  u.CacheReadTokens + o.CacheReadTokens,
		CacheWriteTokens: u.CacheWriteTokens + o.CacheWriteTokens,
	}
}

// TotalInput counts every prompt token, cached or not. The daily budget uses
// it, so a cache hit still counts against the budget (conservatively).
func (u ChatTokenUsage) TotalInput() int64 {
	return u.InputTokens + u.CacheReadTokens + u.CacheWriteTokens
}

// Stop reasons a model turn may end with.
const (
	ChatStopEndTurn   = "end_turn"
	ChatStopToolUse   = "tool_use"
	ChatStopMaxTokens = "max_tokens"
)

// ChatTurnResult is the outcome of one model call.
type ChatTurnResult struct {
	Text       string
	ToolCalls  []ToolCall
	StopReason string
	Usage      ChatTokenUsage
}

// ChatTier decides the daily message cap of a subject.
type ChatTier string

const (
	// ChatTierAnonymous is a visitor without an account: an anon_token
	// cookie or only an IP address.
	ChatTierAnonymous ChatTier = "anonymous"
	ChatTierUser      ChatTier = "user"
	// ChatTierAdmin has no daily message cap; the global token budget still
	// applies.
	ChatTierAdmin ChatTier = "admin"
)

// ChatSubject is who is chatting, for the caps. Key is 'u:<userId>' or
// 'ip:<hash>'; it is never the raw IP.
type ChatSubject struct {
	Key  string
	Tier ChatTier
}

// ChatGlobalSubject is the chat_usage row holding the daily total.
const ChatGlobalSubject = "*"

// ChatUsageCounts is one chat_usage row.
type ChatUsageCounts struct {
	Requests     int64
	InputTokens  int64
	OutputTokens int64
}

// ChatUsageDay is the global ('*') row of one day.
type ChatUsageDay struct {
	Day time.Time
	ChatUsageCounts
}

// ChatUsageSubject is one visitor's (hashed subject) usage total.
type ChatUsageSubject struct {
	Subject string
	ChatUsageCounts
}

// ChatInput is one visitor request.
type ChatInput struct {
	Subject ChatSubject
	// Locale is the page locale, en or fr: the default answer language and
	// the language tools return text in.
	Locale string
	// PagePath is the site path the visitor is on, without locale prefix.
	PagePath string
	// Messages are the visitor's conversation, text only, oldest first. The
	// last one is from the user.
	Messages []ChatMessage
}

// ChatEventKind names a streamed event.
type ChatEventKind string

const (
	ChatEventText  ChatEventKind = "text"
	ChatEventTool  ChatEventKind = "tool"
	ChatEventLinks ChatEventKind = "links"
	ChatEventDone  ChatEventKind = "done"
	// ChatEventMove asks the site's mascot to perform a ninja move.
	ChatEventMove ChatEventKind = "move"
)

// Ninja moves the mascot knows. The frontend animates them.
var NinjaMoves = []string{"flip", "wall_run", "sneak_dash", "vanish"}

// ChatLink is a page the answer refers to. URL is a site path without
// locale prefix (/projects/3) or an external profile URL.
type ChatLink struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Kind  string `json:"kind"`
}

// Link kinds.
const (
	ChatLinkProject     = "project"
	ChatLinkPublication = "publication"
	ChatLinkPDF         = "pdf"
	ChatLinkPage        = "page"
	ChatLinkExternal    = "external"
	// ChatLinkDownload is a file the visitor asked for (a CV or a paper PDF).
	// The links event always carries it, cited in the answer or not, so the
	// chat can show its download button.
	ChatLinkDownload = "download"
)

// ChatEvent is one streamed event. Only the fields of its Kind are set.
type ChatEvent struct {
	Kind ChatEventKind
	// Text is a text delta (text).
	Text string
	// Tool is the tool name (tool).
	Tool string
	// Links are the cited pages (links).
	Links []ChatLink
	// Move is the ninja move to perform (move).
	Move string
	// Remaining is the subject's messages left today (done); nil when the
	// subject has no cap.
	Remaining *int
}

// ChatQuota is what GET /chat/config reports.
type ChatQuota struct {
	// Remaining is the messages left today; nil when uncapped.
	Remaining *int
}

// Chat limit reasons.
const (
	ChatLimitDaily  = "daily_limit"
	ChatLimitBudget = "budget"
)

// ErrChatLimited is returned before any model call when the subject's daily
// cap or the global token budget is used up.
type ErrChatLimited struct {
	Reason     string
	RetryAfter time.Duration
}

func (e *ErrChatLimited) Error() string {
	return fmt.Sprintf("chat limited (%s), retry after %s", e.Reason, e.RetryAfter)
}
