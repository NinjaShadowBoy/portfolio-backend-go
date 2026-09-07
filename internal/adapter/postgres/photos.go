package postgres

import (
	"context"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

func (r *repositories) DeleteOrphanProfilePhotos(ctx context.Context) error {
	return repositoryError(r.q.DeleteOrphanProfilePhotos(ctx))
}

func (r *repositories) DeletePhoto(ctx context.Context, id int64) (int64, error) {
	row, err := r.q.DeletePhoto(ctx, id)
	return row, repositoryError(err)
}

func (r *repositories) ListPhotosByProjectID(ctx context.Context, projectID *int64) ([]domain.Photo, error) {
	row, err := r.q.ListPhotosByProjectID(ctx, nullInt64(projectID))
	return mapRows(row, fromPhoto), repositoryError(err)
}

func (r *repositories) ListPhotosByProjectIDs(ctx context.Context, ids []int64) ([]domain.Photo, error) {
	row, err := r.q.ListPhotosByProjectIDs(ctx, ids)
	return mapRows(row, fromPhoto), repositoryError(err)
}

func (r *repositories) SaveProfilePhoto(ctx context.Context, photoURL string) (domain.Photo, error) {
	row, err := r.q.SaveProfilePhoto(ctx, photoURL)
	return fromPhoto(row), repositoryError(err)
}

func (r *repositories) SaveProjectPhoto(ctx context.Context, photoURL string, projectID *int64) (domain.Photo, error) {
	row, err := r.q.SaveProjectPhoto(ctx, photoURL, nullInt64(projectID))
	return fromPhoto(row), repositoryError(err)
}
