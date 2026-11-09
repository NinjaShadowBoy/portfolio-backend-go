package usecase

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

func (s *UseCases) SaveProjectPhoto(ctx context.Context, photoURL string, projectID int64) (dto.Photo, error) {
	normalized, err := NormalizePhotoURL(photoURL)
	if err != nil {
		return dto.Photo{}, err
	}
	if err := s.requireProject(ctx, projectID); err != nil {
		return dto.Photo{}, err
	}
	photo, err := s.repos.Photos().SaveProjectPhoto(ctx, normalized, dto.Ptr(projectID))
	if err != nil {
		return dto.Photo{}, err
	}
	return dto.Photo{ID: photo.ID, PhotoURL: photo.PhotoURL, ProjectID: *photo.ProjectID, DisplayOrder: int(photo.DisplayOrder)}, nil
}

// ReorderProjectPhotos sets each of the project's photos to its index in ids.
// ids must list every photo of the project exactly once; the first becomes the
// cover. Nothing changes unless the whole order is valid.
func (s *UseCases) ReorderProjectPhotos(ctx context.Context, projectID int64, ids []int64) error {
	_, err := inTransaction(ctx, s, func(tx *UseCases) (struct{}, error) {
		return struct{}{}, tx.reorderProjectPhotos(ctx, projectID, ids)
	})
	return err
}

func (s *UseCases) reorderProjectPhotos(ctx context.Context, projectID int64, ids []int64) error {
	if err := s.requireProject(ctx, projectID); err != nil {
		return err
	}
	photos, err := s.repos.Photos().ListPhotosByProjectID(ctx, dto.Ptr(projectID))
	if err != nil {
		return err
	}
	belongs := make(map[int64]bool, len(photos))
	for _, photo := range photos {
		belongs[photo.ID] = true
	}
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if !belongs[id] {
			return fmt.Errorf("photo %d is not in project %d: %w", id, projectID, ErrBadRequest)
		}
		if seen[id] {
			return fmt.Errorf("photo %d is listed more than once: %w", id, ErrBadRequest)
		}
		seen[id] = true
	}
	// A partial list would leave the missing photos tied with listed ones.
	if len(ids) != len(photos) {
		return fmt.Errorf("order must list all %d photos of project %d, got %d: %w", len(photos), projectID, len(ids), ErrBadRequest)
	}
	for i, id := range ids {
		rows, err := s.repos.Photos().SetPhotoDisplayOrder(ctx, projectID, id, int32(i))
		if err != nil {
			return err
		}
		// The photo was listed above, so it was deleted or moved meanwhile.
		if rows == 0 {
			return fmt.Errorf("photo %d is no longer in project %d: %w", id, projectID, ErrBadRequest)
		}
	}
	return nil
}

func (s *UseCases) saveProfilePhoto(ctx context.Context, photoURL string) (dto.Photo, error) {
	normalized, err := NormalizePhotoURL(photoURL)
	if err != nil {
		return dto.Photo{}, err
	}
	if err := s.repos.Photos().DeleteOrphanProfilePhotos(ctx); err != nil {
		return dto.Photo{}, err
	}
	photo, err := s.repos.Photos().SaveProfilePhoto(ctx, normalized)
	if err != nil {
		return dto.Photo{}, err
	}
	return dto.Photo{ID: photo.ID, PhotoURL: photo.PhotoURL, ProjectID: 0}, nil
}

func (s *UseCases) DeletePhoto(ctx context.Context, id int64) error {
	rows, err := s.repos.Photos().DeletePhoto(ctx, id)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func NormalizePhotoURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("photo URL must not be blank: %w", ErrBadRequest)
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("photo URL must include a valid scheme and host: %w", ErrBadRequest)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("photo URL must use HTTP or HTTPS: %w", ErrBadRequest)
	}
	return trimmed, nil
}

func (s *UseCases) SaveProfilePhoto(ctx context.Context, photoURL string) (dto.Photo, error) {
	return inTransaction(ctx, s, func(tx *UseCases) (dto.Photo, error) { return tx.saveProfilePhoto(ctx, photoURL) })
}
