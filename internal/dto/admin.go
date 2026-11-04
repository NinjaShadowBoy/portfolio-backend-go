package dto

import (
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

// Page is one slice of an admin listing plus the total row count, so the
// client can page without a second request.
type Page[T any] struct {
	Items  []T `json:"items"`
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// AdminUser is the admin view of an account: provider and activity counts on
// top of the public User fields. The password hash never leaves the server.
type AdminUser struct {
	ID            int64      `json:"id"`
	Email         string     `json:"email"`
	Name          string     `json:"name"`
	Role          string     `json:"role"`
	Provider      string     `json:"provider"`
	ImageURL      *string    `json:"imageUrl"`
	EmailVerified bool       `json:"emailVerified"`
	CreatedAt     time.Time  `json:"createdAt"`
	LastLoginAt   *time.Time `json:"lastLoginAt"`
	CommentCount  int        `json:"commentCount"`
	RatingCount   int        `json:"ratingCount"`
}

// AdminUserQuery comes from the query string of GET /admin/users.
type AdminUserQuery struct {
	Search           string
	IncludeAnonymous bool
	Limit            int
	Offset           int
}

type UserRoleUpdate struct {
	Role string `json:"role" binding:"required,oneof=USER ADMIN"`
}

// AdminComment is a moderation feed entry: the public Comment plus the
// author's provider (ANON marks anonymous feedback) and the project name.
type AdminComment struct {
	Comment
	UserProvider string  `json:"userProvider"`
	ProjectName  *string `json:"projectName"`
}

// AdminCommentQuery comes from the query string of GET /admin/comments.
// Target is "project" or "article"; empty values mean no filter.
type AdminCommentQuery struct {
	Type   string
	Target string
	Limit  int
	Offset int
}

type AdminStats struct {
	Users          int `json:"users"`
	AnonymousUsers int `json:"anonymousUsers"`
	Admins         int `json:"admins"`
	Projects       int `json:"projects"`
	Comments       int `json:"comments"`
	Ratings        int `json:"ratings"`
	UnreadMessages int `json:"unreadMessages"`
}

func FromAdminUser(user domain.AdminUser) AdminUser {
	return AdminUser{
		ID:            user.ID,
		Email:         user.Email,
		Name:          user.Name,
		Role:          user.Role,
		Provider:      user.Provider,
		ImageURL:      user.ImageURL,
		EmailVerified: user.EmailVerified,
		CreatedAt:     user.CreatedAt,
		LastLoginAt:   user.LastLoginAt,
		CommentCount:  int(user.CommentCount),
		RatingCount:   int(user.RatingCount),
	}
}

func FromAdminComment(comment domain.AdminComment) AdminComment {
	return AdminComment{
		Comment: FromComment(domain.Comment{
			ID:          comment.ID,
			Body:        comment.Body,
			UserID:      comment.UserID,
			ProjectID:   comment.ProjectID,
			ArticleSlug: comment.ArticleSlug,
			Type:        comment.Type,
			CreatedAt:   comment.CreatedAt,
			UpdatedAt:   comment.UpdatedAt,
		}, comment.UserName, comment.UserImage),
		UserProvider: comment.UserProvider,
		ProjectName:  comment.ProjectName,
	}
}

func FromAdminStats(stats domain.AdminStats) AdminStats {
	return AdminStats{
		Users:          int(stats.Users),
		AnonymousUsers: int(stats.AnonymousUsers),
		Admins:         int(stats.Admins),
		Projects:       int(stats.Projects),
		Comments:       int(stats.Comments),
		Ratings:        int(stats.Ratings),
		UnreadMessages: int(stats.UnreadMessages),
	}
}
