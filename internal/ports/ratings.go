package ports

import (
	"context"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

type RatingsRepository interface {
	DeleteRating(ctx context.Context, id int64, userID int64) (int64, error)
	DeleteRatingsByProjectID(ctx context.Context, projectID int64) error
	DeleteRatingsByUserID(ctx context.Context, userID int64) error
	GetRating(ctx context.Context, id int64) (domain.Rating, error)
	HasUserRatedProject(ctx context.Context, userID int64, projectID int64) (bool, error)
	ListRatingAggregatesByProjectIDs(ctx context.Context, ids []int64) ([]domain.RatingAggregate, error)
	ListRatings(ctx context.Context) ([]domain.Rating, error)
	ListRatingsByProjectID(ctx context.Context, projectID int64) ([]domain.Rating, error)
	ListRatingsByUserID(ctx context.Context, userID int64) ([]domain.Rating, error)
	RatingAverageByProjectID(ctx context.Context, projectID int64) (float64, error)
	RatingCountByProjectID(ctx context.Context, projectID int64) (int32, error)
	RatingDistributionByProjectID(ctx context.Context, projectID int64) ([]domain.RatingDistribution, error)
	ReassignRatingsToUser(ctx context.Context, targetUserID int64, sourceUserID int64) error
	UpdateRating(ctx context.Context, id int64, rating int32) (domain.Rating, error)
	UpsertRating(ctx context.Context, rating int32, userID int64, projectID int64) (domain.Rating, error)
}
