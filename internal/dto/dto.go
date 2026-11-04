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
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Technologies  []string  `json:"technologies"`
	GithubLink    *string   `json:"githubLink"`
	Challenges    *string   `json:"challenges"`
	WhatILearned  *string   `json:"whatILearned"`
	Featured      bool      `json:"featured"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	AverageRating float64   `json:"averageRating"`
	TotalRatings  int       `json:"totalRatings"`
	PhotoUrls     []string  `json:"photoUrls"`
	Photos        []Photo   `json:"photos"`
}

type ProjectCreate struct {
	Name         string   `json:"name" sanitize:"text" binding:"required,max=500"`
	Description  string   `json:"description" sanitize:"multiline" binding:"required"`
	Technologies []string `json:"technologies" sanitize:"text" binding:"max=50,dive,required,max=255"`
	GithubLink   *string  `json:"githubLink" sanitize:"text" binding:"omitempty,max=2000"`
	Challenges   *string  `json:"challenges" sanitize:"multiline"`
	WhatILearned *string  `json:"whatILearned" sanitize:"multiline"`
	Featured     bool     `json:"featured"`
}

type ProjectUpdate struct {
	Name         *string   `json:"name" sanitize:"text" binding:"omitempty,max=500"`
	Description  *string   `json:"description" sanitize:"multiline"`
	Technologies *[]string `json:"technologies" sanitize:"text" binding:"omitempty,max=50,dive,required,max=255"`
	GithubLink   *string   `json:"githubLink" sanitize:"text" binding:"omitempty,max=2000"`
	Challenges   *string   `json:"challenges" sanitize:"multiline"`
	WhatILearned *string   `json:"whatILearned" sanitize:"multiline"`
	Featured     *bool     `json:"featured"`
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
