package dto

import (
	"database/sql"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/db"
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
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
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
	DeviceCode string `json:"deviceCode"`
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
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Technologies []string `json:"technologies"`
	GithubLink   *string  `json:"githubLink"`
	Challenges   *string  `json:"challenges"`
	WhatILearned *string  `json:"whatILearned"`
	Featured     bool     `json:"featured"`
}

type ProjectUpdate struct {
	Name         *string   `json:"name"`
	Description  *string   `json:"description"`
	Technologies *[]string `json:"technologies"`
	GithubLink   *string   `json:"githubLink"`
	Challenges   *string   `json:"challenges"`
	WhatILearned *string   `json:"whatILearned"`
	Featured     *bool     `json:"featured"`
}

type Rating struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"userId"`
	UserName  string    `json:"userName"`
	ProjectID int64     `json:"projectId"`
	Rating    int       `json:"rating"`
	Comment   *string   `json:"comment"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type RatingCreate struct {
	ProjectID int64   `json:"projectId"`
	Rating    int     `json:"rating"`
	Comment   *string `json:"comment"`
}

type RatingUpdate struct {
	Rating  *int    `json:"rating"`
	Comment *string `json:"comment"`
}

type Photo struct {
	ID        int64  `json:"id"`
	PhotoURL  string `json:"photoUrl"`
	ProjectID int64  `json:"projectId"`
}

type PhotoCreate struct {
	PhotoURL  string `json:"photoUrl"`
	ProjectID *int64 `json:"projectId"`
}

type Contact struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Message string `json:"message"`
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

func FromUser(user db.User) User {
	var lastLoginAt *time.Time
	if user.LastLoginAt.Valid {
		lastLoginAt = &user.LastLoginAt.Time
	}

	return User{
		ID:          user.ID,
		Email:       user.Email,
		Name:        user.Name,
		Role:        user.Role,
		CreatedAt:   user.CreatedAt,
		LastLoginAt: lastLoginAt,
	}
}

func Ptr[T any](value T) *T {
	return &value
}

func NullString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func PtrString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func PtrBool(value *bool) bool {
	if value == nil {
		return false
	}
	return *value
}
