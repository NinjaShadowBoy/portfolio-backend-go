package usecase

import (
	"context"
	"errors"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

func (s *UseCases) GetProjectRatings(ctx context.Context, projectID int64) ([]dto.Rating, error) {
	if err := s.requireProject(ctx, projectID); err != nil {
		return nil, err
	}
	ratings, err := s.repos.Ratings().ListRatingsByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return s.ratingsDTO(ctx, ratings)
}

func (s *UseCases) GetProjectAverageRating(ctx context.Context, projectID int64) (float64, error) {
	if err := s.requireProject(ctx, projectID); err != nil {
		return 0, err
	}
	averageRaw, err := s.repos.Ratings().RatingAverageByProjectID(ctx, projectID)
	if err != nil {
		return 0, err
	}
	return averageRaw, nil
}

func (s *UseCases) GetProjectRatingCount(ctx context.Context, projectID int64) (int, error) {
	if err := s.requireProject(ctx, projectID); err != nil {
		return 0, err
	}
	count, err := s.repos.Ratings().RatingCountByProjectID(ctx, projectID)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

func (s *UseCases) GetProjectRatingDistribution(ctx context.Context, projectID int64) (map[int]int, error) {
	if err := s.requireProject(ctx, projectID); err != nil {
		return nil, err
	}
	rows, err := s.repos.Ratings().RatingDistributionByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	distribution := map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0}
	for _, row := range rows {
		distribution[int(row.Rating)] = int(row.Count)
	}
	return distribution, nil
}

func (s *UseCases) ResetProjectRatings(ctx context.Context, projectID int64) error {
	if err := s.requireProject(ctx, projectID); err != nil {
		return err
	}
	return s.repos.Ratings().DeleteRatingsByProjectID(ctx, projectID)
}

func (s *UseCases) ListRatings(ctx context.Context) ([]dto.Rating, error) {
	ratings, err := s.repos.Ratings().ListRatings(ctx)
	if err != nil {
		return nil, err
	}
	return s.ratingsDTO(ctx, ratings)
}

func (s *UseCases) GetRating(ctx context.Context, id int64) (dto.Rating, error) {
	rating, err := s.repos.Ratings().GetRating(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return dto.Rating{}, ErrNotFound
	}
	if err != nil {
		return dto.Rating{}, err
	}
	return s.ratingDTO(ctx, rating)
}

func (s *UseCases) GetRatingsByUserID(ctx context.Context, userID int64) ([]dto.Rating, error) {
	ratings, err := s.repos.Ratings().ListRatingsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.ratingsDTO(ctx, ratings)
}

func (s *UseCases) CreateRating(ctx context.Context, userID int64, req dto.RatingCreate) (dto.Rating, error) {
	if req.Rating < 1 || req.Rating > 5 {
		return dto.Rating{}, ErrBadRequest
	}
	if _, err := s.repos.Users().GetUserByID(ctx, userID); errors.Is(err, domain.ErrNotFound) {
		return dto.Rating{}, ErrNotFound
	} else if err != nil {
		return dto.Rating{}, err
	}
	if err := s.requireProject(ctx, req.ProjectID); err != nil {
		return dto.Rating{}, err
	}

	// A star is one-per-identity-per-project: upsert rather than reject repeats.
	rating, err := s.repos.Ratings().UpsertRating(ctx, int32(req.Rating), userID, req.ProjectID)
	if err != nil {
		return dto.Rating{}, err
	}
	return s.ratingDTO(ctx, rating)
}

func (s *UseCases) UpdateRating(ctx context.Context, id int64, userID int64, req dto.RatingUpdate) (dto.Rating, error) {
	if req.Rating != nil && (*req.Rating < 1 || *req.Rating > 5) {
		return dto.Rating{}, ErrBadRequest
	}
	rating, err := s.repos.Ratings().GetRating(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return dto.Rating{}, ErrNotFound
	}
	if err != nil {
		return dto.Rating{}, err
	}
	if rating.UserID != userID {
		return dto.Rating{}, ErrForbidden
	}

	ratingValue := rating.Rating
	if req.Rating != nil {
		ratingValue = int32(*req.Rating)
	}

	updated, err := s.repos.Ratings().UpdateRating(ctx, id, ratingValue)
	if err != nil {
		return dto.Rating{}, err
	}
	return s.ratingDTO(ctx, updated)
}

func (s *UseCases) DeleteRating(ctx context.Context, id int64, userID int64) error {
	rows, err := s.repos.Ratings().DeleteRating(ctx, id, userID)
	if err != nil {
		return err
	}
	if rows == 0 {
		exists, err := s.repos.Ratings().GetRating(ctx, id)
		if errors.Is(err, domain.ErrNotFound) {
			return ErrNotFound
		}
		if err == nil && exists.UserID != userID {
			return ErrForbidden
		}
		return ErrNotFound
	}
	return nil
}

func (s *UseCases) HasUserRatedProject(ctx context.Context, userID int64, projectID int64) (bool, error) {
	return s.repos.Ratings().HasUserRatedProject(ctx, userID, projectID)
}

func (s *UseCases) ratingsDTO(ctx context.Context, ratings []domain.Rating) ([]dto.Rating, error) {
	if len(ratings) == 0 {
		return []dto.Rating{}, nil
	}

	userIDs := make([]int64, 0, len(ratings))
	seen := make(map[int64]struct{}, len(ratings))
	for _, rating := range ratings {
		if _, ok := seen[rating.UserID]; ok {
			continue
		}
		seen[rating.UserID] = struct{}{}
		userIDs = append(userIDs, rating.UserID)
	}

	users, err := s.repos.Users().GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	names := make(map[int64]string, len(users))
	for _, user := range users {
		names[user.ID] = user.Name
	}

	items := make([]dto.Rating, 0, len(ratings))
	for _, rating := range ratings {
		items = append(items, ratingToDTO(rating, names[rating.UserID]))
	}
	return items, nil
}

func (s *UseCases) ratingDTO(ctx context.Context, rating domain.Rating) (dto.Rating, error) {
	user, err := s.repos.Users().GetUserByID(ctx, rating.UserID)
	if err != nil {
		return dto.Rating{}, err
	}
	return ratingToDTO(rating, user.Name), nil
}

func ratingToDTO(rating domain.Rating, userName string) dto.Rating {
	return dto.Rating{
		ID:        rating.ID,
		UserID:    rating.UserID,
		UserName:  userName,
		ProjectID: rating.ProjectID,
		Rating:    int(rating.Rating),
		CreatedAt: rating.CreatedAt,
		UpdatedAt: rating.UpdatedAt,
	}
}
