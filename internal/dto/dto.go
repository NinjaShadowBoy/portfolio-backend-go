package dto

import (
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

type User struct {
	ID          int64      `json:"id"`
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	Role        string     `json:"role"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastLoginAt *time.Time `json:"lastLoginAt"`
}

type LoginRequest struct {
	Email    string `json:"email" sanitize:"email" binding:"required,email,max=254"`
	Password string `json:"password" binding:"required,max=1024"`
}

type RegisterRequest struct {
	Email    string `json:"email" sanitize:"email" binding:"required,email,max=254"`
	Password string `json:"password" binding:"required,min=8,maxbytes=72"`
	Name     string `json:"name" sanitize:"text" binding:"required,max=100"`
}

type LoginResponse struct {
	Token     string `json:"token"`
	User      User   `json:"user"`
	ExpiresIn int64  `json:"expiresIn"`
}

type MessageResponse struct {
	Message string `json:"message"`
}

// DeviceCodeResponse is returned when a device-flow login is started. The client
// shows userCode + verificationUri to the user and then polls with deviceCode.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"deviceCode"`
	UserCode        string `json:"userCode"`
	VerificationUri string `json:"verificationUri"`
	ExpiresIn       int    `json:"expiresIn"`
	Interval        int    `json:"interval"`
}

// DevicePollRequest is the body the client sends while polling a device-flow login.
type DevicePollRequest struct {
	DeviceCode string `json:"deviceCode" sanitize:"text" binding:"required,max=255"`
}

// DeviceStatusResponse is returned (HTTP 202) while the user has not yet
// authorized the device. Status is "pending" or "slow_down".
type DeviceStatusResponse struct {
	Status string `json:"status"`
}

type Project struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Technologies []string `json:"technologies"`
	GithubLink   *string  `json:"githubLink"`
	DemoLink     *string  `json:"demoLink"`
	Challenges   *string  `json:"challenges"`
	WhatILearned *string  `json:"whatILearned"`
	// French versions; null when not translated, and the client falls back to English.
	DescriptionFr  *string   `json:"descriptionFr"`
	ChallengesFr   *string   `json:"challengesFr"`
	WhatILearnedFr *string   `json:"whatILearnedFr"`
	Featured       bool      `json:"featured"`
	DisplayOrder   int       `json:"displayOrder"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	AverageRating  float64   `json:"averageRating"`
	TotalRatings   int       `json:"totalRatings"`
	PhotoUrls      []string  `json:"photoUrls"`
	Photos         []Photo   `json:"photos"`
}

type ProjectCreate struct {
	Name         string   `json:"name" sanitize:"text" binding:"required,max=500"`
	Description  string   `json:"description" sanitize:"multiline" binding:"required"`
	Technologies []string `json:"technologies" sanitize:"text" binding:"max=50,dive,required,max=255"`
	GithubLink   *string  `json:"githubLink" sanitize:"text" binding:"omitempty,max=2000"`
	DemoLink     *string  `json:"demoLink" sanitize:"text" binding:"omitempty,http_url_or_empty,max=2000"`
	Challenges   *string  `json:"challenges" sanitize:"multiline"`
	WhatILearned *string  `json:"whatILearned" sanitize:"multiline"`
	// French versions; empty or absent means not translated.
	DescriptionFr  *string `json:"descriptionFr" sanitize:"multiline"`
	ChallengesFr   *string `json:"challengesFr" sanitize:"multiline"`
	WhatILearnedFr *string `json:"whatILearnedFr" sanitize:"multiline"`
	Featured       bool    `json:"featured"`
}

type ProjectUpdate struct {
	Name         *string   `json:"name" sanitize:"text" binding:"omitempty,max=500"`
	Description  *string   `json:"description" sanitize:"multiline"`
	Technologies *[]string `json:"technologies" sanitize:"text" binding:"omitempty,max=50,dive,required,max=255"`
	GithubLink   *string   `json:"githubLink" sanitize:"text" binding:"omitempty,max=2000"`
	DemoLink     *string   `json:"demoLink" sanitize:"text" binding:"omitempty,http_url_or_empty,max=2000"`
	Challenges   *string   `json:"challenges" sanitize:"multiline"`
	WhatILearned *string   `json:"whatILearned" sanitize:"multiline"`
	// French versions; an empty string removes the translation.
	DescriptionFr  *string `json:"descriptionFr" sanitize:"multiline"`
	ChallengesFr   *string `json:"challengesFr" sanitize:"multiline"`
	WhatILearnedFr *string `json:"whatILearnedFr" sanitize:"multiline"`
	Featured       *bool   `json:"featured"`
}

// ProjectOrder is the body of PUT /projects/order. IDs lists project ids in
// the order they should appear; each id's position becomes its displayOrder.
type ProjectOrder struct {
	IDs []int64 `json:"ids" binding:"required,min=1,dive,gt=0"`
}

type Rating struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"userId"`
	UserName  string    `json:"userName"`
	ProjectID int64     `json:"projectId"`
	Rating    int       `json:"rating"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type RatingCreate struct {
	ProjectID int64 `json:"projectId"`
	Rating    int   `json:"rating"`
}

type RatingUpdate struct {
	Rating *int `json:"rating"`
}

type Comment struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"userId"`
	UserName    string    `json:"userName"`
	UserImage   *string   `json:"userImage"`
	ProjectID   *int64    `json:"projectId"`
	ArticleSlug *string   `json:"articleSlug"`
	Type        string    `json:"type"`
	Body        string    `json:"body"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type CommentCreate struct {
	ProjectID   *int64  `json:"projectId"`
	ArticleSlug *string `json:"articleSlug" sanitize:"text" binding:"omitempty,max=255"`
	Type        string  `json:"type" binding:"required,oneof=critique suggestion feature_request"`
	Body        string  `json:"body" sanitize:"multiline" binding:"required,max=2000"`
}

type CommentUpdate struct {
	Body string `json:"body" sanitize:"multiline" binding:"required,max=2000"`
}

// Identity is the current caller as resolved by EnsureIdentity: a real account,
// an auto-provisioned anonymous user, or nobody yet.
type Identity struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	Role      string  `json:"role"`
	Provider  string  `json:"provider"`
	ImageURL  *string `json:"imageUrl"`
	Anonymous bool    `json:"anonymous"`
	Email     string  `json:"email,omitempty"`
}

type ProfileUpdate struct {
	Name     *string `json:"name" sanitize:"text" binding:"omitempty,max=100"`
	ImageURL *string `json:"imageUrl" binding:"omitempty,max=2000"`
}

type PasswordChange struct {
	CurrentPassword string `json:"currentPassword" binding:"required,max=1024"`
	NewPassword     string `json:"newPassword" binding:"required,min=8,maxbytes=72"`
}

type Photo struct {
	ID        int64  `json:"id"`
	PhotoURL  string `json:"photoUrl"`
	ProjectID int64  `json:"projectId"`
	// DisplayOrder is the photo's position within its project, from 0.
	DisplayOrder int `json:"displayOrder"`
}

// PhotoOrder is the body of PUT /projects/:id/photos/order. IDs lists every
// photo of the project in the order they should appear; the first is the cover.
type PhotoOrder struct {
	IDs []int64 `json:"ids" binding:"required,min=1,dive,gt=0"`
}

type PhotoCreate struct {
	PhotoURL  string `json:"photoUrl" binding:"required,max=2000"`
	ProjectID *int64 `json:"projectId"`
}

type Contact struct {
	Name    string `json:"name" sanitize:"text" binding:"required,max=100"`
	Email   string `json:"email" sanitize:"email" binding:"required,email,max=254"`
	Message string `json:"message" sanitize:"multiline" binding:"required,max=2000"`
}

type ContactMessage struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	Message     string    `json:"message"`
	SubmittedAt time.Time `json:"submittedAt"`
	IsRead      bool      `json:"isRead"`
	IsReplied   bool      `json:"isReplied"`
}

type ContactResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    *ContactMessage `json:"data,omitempty"`
}

// ChatMessageInput is one turn of the visitor's conversation. The API takes
// text only: tool calls and results never round-trip, so a client cannot forge
// them.
type ChatMessageInput struct {
	Role string `json:"role" binding:"required,oneof=user assistant"`
	Text string `json:"text" sanitize:"multiline" binding:"required,max=1000"`
}

// ChatRequest is the body of POST /chat. Messages are oldest first; the last
// one must be from the user (checked by the use case).
type ChatRequest struct {
	Locale   string             `json:"locale" binding:"required,oneof=en fr"`
	PagePath string             `json:"pagePath" sanitize:"text" binding:"max=200"`
	Messages []ChatMessageInput `json:"messages" binding:"required,min=1,max=12,dive"`
}

// ChatConfigResponse is the body of GET /chat/config. RemainingToday is null
// when the subject has no daily cap.
type ChatConfigResponse struct {
	Enabled        bool `json:"enabled"`
	RemainingToday *int `json:"remainingToday"`
}

// ChatUsageDay is one day's global assistant usage ('*' subject). Day is UTC
// as yyyy-mm-dd. Days in the range without traffic are reported with zeros.
type ChatUsageDay struct {
	Day          string `json:"day"`
	Requests     int64  `json:"requests"`
	InputTokens  int64  `json:"inputTokens"`
	OutputTokens int64  `json:"outputTokens"`
}

// ChatUsageSubject is one visitor's usage over the range. Subject is already
// hashed by the API ('ip:<hex>' or 'u:<userId>'), never a raw IP or email.
type ChatUsageSubject struct {
	Subject      string `json:"subject"`
	Requests     int64  `json:"requests"`
	InputTokens  int64  `json:"inputTokens"`
	OutputTokens int64  `json:"outputTokens"`
}

// ChatBudget is the assistant's standing today. TokenBudget of 0 means no
// global budget is configured (BudgetRemaining is then also 0).
type ChatBudget struct {
	Requests        int64 `json:"requests"`
	Tokens          int64 `json:"tokens"`
	TokenBudget     int64 `json:"tokenBudget"`
	BudgetRemaining int64 `json:"budgetRemaining"`
}

// ChatUsageReport is the body of GET /admin/chat/usage.
type ChatUsageReport struct {
	Days        []ChatUsageDay     `json:"days"`
	TopSubjects []ChatUsageSubject `json:"topSubjects"`
	Today       ChatBudget         `json:"today"`
}

// KnowledgeSourceStatus is one indexed source as GET /admin/knowledge lists
// it. UpdatedAt is the last change of any of its chunks.
type KnowledgeSourceStatus struct {
	Kind      string    `json:"kind"`
	Ref       string    `json:"ref"`
	Lang      string    `json:"lang"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	Chunks    int       `json:"chunks"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// KnowledgeSyncStatus summarises the last finished knowledge index sync,
// whether it came from the background loop or an admin reindex. Added and
// Updated count sources; Deleted counts chunk rows.
type KnowledgeSyncStatus struct {
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	Added      int       `json:"added"`
	Updated    int       `json:"updated"`
	Deleted    int64     `json:"deleted"`
	Unchanged  int       `json:"unchanged"`
	// Error is the failure reason, or null when the sync succeeded.
	Error *string `json:"error"`
}

// KnowledgeStatus is the body of GET /admin/knowledge. With enabled false,
// Syncing is false and LastSync nil; Sources still lists what is stored.
type KnowledgeStatus struct {
	Enabled  bool                    `json:"enabled"`
	Sources  []KnowledgeSourceStatus `json:"sources"`
	LastSync *KnowledgeSyncStatus    `json:"lastSync"`
	Syncing  bool                    `json:"syncing"`
}

type ErrorResponse struct {
	Timestamp time.Time `json:"timestamp"`
	Status    int       `json:"status"`
	Error     string    `json:"error"`
	Code      string    `json:"code"`
	Message   string    `json:"message"`
}

func FromUser(user domain.User) User {
	return User{
		ID:          user.ID,
		Email:       user.Email,
		Name:        user.Name,
		Role:        user.Role,
		CreatedAt:   user.CreatedAt,
		LastLoginAt: user.LastLoginAt,
	}
}

// FromComment maps a persisted comment plus its author's display fields to the
// wire DTO. Exactly one of projectId / articleSlug is populated (the DB CHECK
// enforces the XOR) — the polymorphic-target contract the frontend
// CommentService is coded against.
func FromComment(comment domain.Comment, userName string, userImage *string) Comment {
	return Comment{
		ID:          comment.ID,
		UserID:      comment.UserID,
		UserName:    userName,
		UserImage:   userImage,
		ProjectID:   comment.ProjectID,
		ArticleSlug: comment.ArticleSlug,
		Type:        comment.Type,
		Body:        comment.Body,
		CreatedAt:   comment.CreatedAt,
		UpdatedAt:   comment.UpdatedAt,
	}
}

func Ptr[T any](value T) *T {
	return &value
}
