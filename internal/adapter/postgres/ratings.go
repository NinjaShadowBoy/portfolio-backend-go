package postgres

import (
	"context"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

func (r *repositories) DeleteRating(ctx context.Context, id int64, userID int64) (int64, error) {
	row, err := r.q.DeleteRating(ctx, id, userID)
	return row, repositoryError(err)
}

func (r *repositories) DeleteRatingsByProjectID(ctx context.Context, projectID int64) error {
	return repositoryError(r.q.DeleteRatingsByProjectID(ctx, projectID))
}

func (r *repositories) DeleteRatingsByUserID(ctx context.Context, userID int64) error {
	return repositoryError(r.q.DeleteRatingsByUserID(ctx, userID))
}

func (r *repositories) GetRating(ctx context.Context, id int64) (domain.Rating, error) {
	row, err := r.q.GetRating(ctx, id)
	return fromRating(row), repositoryError(err)
}

func (r *repositories) HasUserRatedProject(ctx context.Context, userID int64, projectID int64) (bool, error) {
	row, err := r.q.HasUserRatedProject(ctx, userID, projectID)
	return row, repositoryError(err)
}

func (r *repositories) ListRatingAggregatesByProjectIDs(ctx context.Context, ids []int64) ([]domain.RatingAggregate, error) {
	row, err := r.q.ListRatingAggregatesByProjectIDs(ctx, ids)
	return mapRows(row, fromListRatingAggregatesByProjectIDsRow), repositoryError(err)
}

func (r *repositories) ListRatings(ctx context.Context) ([]domain.Rating, error) {
	row, err := r.q.ListRatings(ctx)
	return mapRows(row, fromRating), repositoryError(err)
}

func (r *repositories) ListRatingsByProjectID(ctx context.Context, projectID int64) ([]domain.Rating, error) {
	row, err := r.q.ListRatingsByProjectID(ctx, projectID)
	return mapRows(row, fromRating), repositoryError(err)
}

func (r *repositories) ListRatingsByUserID(ctx context.Context, userID int64) ([]domain.Rating, error) {
	row, err := r.q.ListRatingsByUserID(ctx, userID)
	return mapRows(row, fromRating), repositoryError(err)
}

func (r *repositories) RatingAverageByProjectID(ctx context.Context, projectID int64) (float64, error) {
	row, err := r.q.RatingAverageByProjectID(ctx, projectID)
	if err != nil {
		return 0, repositoryError(err)
	}
	return number(row), repositoryError(err)
}

func (r *repositories) RatingCountByProjectID(ctx context.Context, projectID int64) (int32, error) {
	row, err := r.q.RatingCountByProjectID(ctx, projectID)
	return row, repositoryError(err)
}

func (r *repositories) RatingDistributionByProjectID(ctx context.Context, projectID int64) ([]domain.RatingDistribution, error) {
	row, err := r.q.RatingDistributionByProjectID(ctx, projectID)
	return mapRows(row, fromRatingDistributionByProjectIDRow), repositoryError(err)
}

func (r *repositories) ReassignRatingsToUser(ctx context.Context, targetUserID int64, sourceUserID int64) error {
	return repositoryError(r.q.ReassignRatingsToUser(ctx, targetUserID, sourceUserID))
}

func (r *repositories) UpdateRating(ctx context.Context, id int64, rating int32) (domain.Rating, error) {
	row, err := r.q.UpdateRating(ctx, id, rating)
	return fromRating(row), repositoryError(err)
}

func (r *repositories) UpsertRating(ctx context.Context, rating int32, userID int64, projectID int64) (domain.Rating, error) {
	row, err := r.q.UpsertRating(ctx, rating, userID, projectID)
	return fromRating(row), repositoryError(err)
}
