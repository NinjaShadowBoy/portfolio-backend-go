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
	return dto.Photo{ID: photo.ID, PhotoURL: photo.PhotoURL, ProjectID: *photo.ProjectID}, nil
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
