package postgres

import (
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/db"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

func fromListCommentsByArticleSlugRow(row db.ListCommentsByArticleSlugRow) domain.CommentWithAuthor {
	return domain.CommentWithAuthor{
		ID:          row.ID,
		Body:        row.Body,
		UserID:      row.UserID,
		ProjectID:   int64Ptr(row.ProjectID),
		ArticleSlug: stringPtr(row.ArticleSlug),
		Type:        row.Type,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
		UserName:    row.UserName,
		UserImage:   stringPtr(row.UserImage),
	}
}

func fromListCommentsByProjectIDRow(row db.ListCommentsByProjectIDRow) domain.CommentWithAuthor {
	return domain.CommentWithAuthor{
		ID:          row.ID,
		Body:        row.Body,
		UserID:      row.UserID,
		ProjectID:   int64Ptr(row.ProjectID),
		ArticleSlug: stringPtr(row.ArticleSlug),
		Type:        row.Type,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
		UserName:    row.UserName,
		UserImage:   stringPtr(row.UserImage),
	}
}

func fromListCommentsForAdminRow(row db.ListCommentsForAdminRow) domain.AdminComment {
	return domain.AdminComment{
		CommentWithAuthor: domain.CommentWithAuthor{
			ID:          row.ID,
			Body:        row.Body,
			UserID:      row.UserID,
			ProjectID:   int64Ptr(row.ProjectID),
			ArticleSlug: stringPtr(row.ArticleSlug),
			Type:        row.Type,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
			UserName:    row.UserName,
			UserImage:   stringPtr(row.UserImage),
		},
		UserProvider: row.UserProvider,
		ProjectName:  stringPtr(row.ProjectName),
	}
}

func fromComment(row db.Comment) domain.Comment {
	return domain.Comment{
		ID:          row.ID,
		Body:        row.Body,
		UserID:      row.UserID,
		ProjectID:   int64Ptr(row.ProjectID),
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
		Type:        row.Type,
		ArticleSlug: stringPtr(row.ArticleSlug),
	}
}

func fromContactMessage(row db.ContactMessage) domain.ContactMessage {
	return domain.ContactMessage{
		ID:          row.ID,
		Name:        row.Name,
		Email:       row.Email,
		Message:     row.Message,
		SubmittedAt: row.SubmittedAt,
		IsRead:      row.IsRead,
		IsReplied:   row.IsReplied,
	}
}

func fromPhoto(row db.Photo) domain.Photo {
	return domain.Photo{
		ID:        row.ID,
		PhotoURL:  row.PhotoUrl,
		ProjectID: int64Ptr(row.ProjectID),
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func fromProject(row db.Project) domain.Project {
	return domain.Project{
		ID:           row.ID,
		Name:         row.Name,
		Description:  row.Description,
		Featured:     row.Featured,
		GithubLink:   stringPtr(row.GithubLink),
		DemoLink:     stringPtr(row.DemoLink),
		Challenges:   stringPtr(row.Challenges),
		WhatILearned: stringPtr(row.WhatILearned),
		DisplayOrder: row.DisplayOrder,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

func fromProjectTechnology(row db.ProjectTechnology) domain.ProjectTechnology {
	return domain.ProjectTechnology{
		ProjectID:  row.ProjectID,
		Technology: row.Technology,
	}
}

func fromRating(row db.Rating) domain.Rating {
	return domain.Rating{
		ID:        row.ID,
		Rating:    row.Rating,
		UserID:    row.UserID,
		ProjectID: row.ProjectID,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func fromUser(row db.User) domain.User {
	return domain.User{
		ID:            row.ID,
		Email:         row.Email,
		PasswordHash:  stringPtr(row.Pwd),
		Name:          row.Name,
		Role:          row.Role,
		Provider:      row.Provider,
		ProviderID:    stringPtr(row.ProviderID),
		ImageURL:      stringPtr(row.ImageUrl),
		EmailVerified: row.EmailVerified,
		LastLoginAt:   timePtr(row.LastLoginAt),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func fromListUsersForAdminRow(row db.ListUsersForAdminRow) domain.AdminUser {
	return domain.AdminUser{
		User: fromUser(db.User{
			ID:            row.ID,
			Email:         row.Email,
			Pwd:           row.Pwd,
			Name:          row.Name,
			Role:          row.Role,
			Provider:      row.Provider,
			ProviderID:    row.ProviderID,
			ImageUrl:      row.ImageUrl,
			EmailVerified: row.EmailVerified,
			LastLoginAt:   row.LastLoginAt,
			CreatedAt:     row.CreatedAt,
			UpdatedAt:     row.UpdatedAt,
		}),
		CommentCount: row.CommentCount,
		RatingCount:  row.RatingCount,
	}
}

func fromListRatingAggregatesByProjectIDsRow(row db.ListRatingAggregatesByProjectIDsRow) domain.RatingAggregate {
	return domain.RatingAggregate{
		ProjectID: row.ProjectID,
		Count:     row.Count,
		Average:   number(row.Average),
	}
}

func fromRatingDistributionByProjectIDRow(row db.RatingDistributionByProjectIDRow) domain.RatingDistribution {
	return domain.RatingDistribution{
		Rating: row.Rating,
		Count:  row.Count,
	}
}

func repositoryError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}
func stringPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}
func int64Ptr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}
func timePtr(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	return &v.Time
}
func nullString(v *string) sql.NullString {
	if v == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *v, Valid: true}
}
func nullInt64(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}
func nullTime(v *time.Time) sql.NullTime {
	if v == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *v, Valid: true}
}
func mapRows[A, B any](rows []A, convert func(A) B) []B {
	if rows == nil {
		return nil
	}
	result := make([]B, len(rows))
	for i, row := range rows {
		result[i] = convert(row)
	}
	return result
}
func number(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int64:
		return float64(v)
	case int32:
		return float64(v)
	case int:
		return float64(v)
	case []byte:
		n, _ := strconv.ParseFloat(string(v), 64)
		return n
	case string:
		n, _ := strconv.ParseFloat(v, 64)
		return n
	default:
		return 0
	}
}
