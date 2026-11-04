package domain

import "time"

type CommentWithAuthor struct {
	ID          int64
	Body        string
	UserID      int64
	ProjectID   *int64
	ArticleSlug *string
	Type        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	UserName    string
	UserImage   *string
}

type AnonToken struct {
	TokenHash string
	UserID    int64
	CreatedAt time.Time
}

type Comment struct {
	ID          int64
	Body        string
	UserID      int64
	ProjectID   *int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Type        string
	ArticleSlug *string
}

type ContactMessage struct {
	ID          int64
	Name        string
	Email       string
	Message     string
	SubmittedAt time.Time
	IsRead      bool
	IsReplied   bool
}

type Photo struct {
	ID        int64
	PhotoURL  string
	ProjectID *int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Project struct {
	ID           int64
	Name         string
	Description  string
	Featured     bool
	GithubLink   *string
	Challenges   *string
	WhatILearned *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ProjectTechnology struct {
	ProjectID  int64
	Technology string
}

type Rating struct {
	ID        int64
	Rating    int32
	UserID    int64
	ProjectID int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

type User struct {
	ID            int64
	Email         string
	PasswordHash  *string
	Name          string
	Role          string
	Provider      string
	ProviderID    *string
	ImageURL      *string
	EmailVerified bool
	LastLoginAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type RatingAggregate struct {
	ProjectID int64
	Count     int32
	Average   float64
}

type RatingDistribution struct {
	Rating int32
	Count  int32
}

// AdminUser is a user row plus the activity counts shown in the admin list.
type AdminUser struct {
	User
	CommentCount int32
	RatingCount  int32
}

// UserFilter narrows the admin user list. Search matches name or email.
type UserFilter struct {
	Search           *string
	IncludeAnonymous bool
	Limit            int32
	Offset           int32
}

// AdminComment is a comment with its author and, for project comments, the
// project name, so the moderation feed needs no extra lookups.
type AdminComment struct {
	CommentWithAuthor
	UserProvider string
	ProjectName  *string
}

// CommentFilter narrows the moderation feed. Target is "project" or "article".
type CommentFilter struct {
	Type   *string
	Target *string
	Limit  int32
	Offset int32
}

type AdminStats struct {
	Users          int32
	AnonymousUsers int32
	Admins         int32
	Projects       int32
	Comments       int32
	Ratings        int32
	UnreadMessages int32
}
