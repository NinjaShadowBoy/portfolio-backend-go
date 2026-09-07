package ports

import (
	"context"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

type PhotosRepository interface {
	DeleteOrphanProfilePhotos(ctx context.Context) error
	DeletePhoto(ctx context.Context, id int64) (int64, error)
	ListPhotosByProjectID(ctx context.Context, projectID *int64) ([]domain.Photo, error)
	ListPhotosByProjectIDs(ctx context.Context, ids []int64) ([]domain.Photo, error)
	SaveProfilePhoto(ctx context.Context, photoURL string) (domain.Photo, error)
	SaveProjectPhoto(ctx context.Context, photoURL string, projectID *int64) (domain.Photo, error)
}
