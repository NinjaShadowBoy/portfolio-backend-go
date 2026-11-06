package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

type ListRatingAggregate struct {
	Count   int
	Average float64
}

func (s *UseCases) ListProjects(ctx context.Context) ([]dto.Project, error) {
	projects, err := s.repos.Projects().ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	if len(projects) == 0 {
		return []dto.Project{}, nil
	}

	ids := make([]int64, len(projects))
	for i, p := range projects {
		ids[i] = p.ID
	}

	techRows, err := s.repos.Projects().ListAllTechnologiesByProjectID(ctx, ids)
	if err != nil {
		return nil, err
	}
	technologies := make(map[int64][]string, len(ids))
	for _, row := range techRows {
		technologies[row.ProjectID] = append(technologies[row.ProjectID], row.Technology)
	}

	photoRows, err := s.repos.Photos().ListPhotosByProjectIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	photosByProject := make(map[int64][]domain.Photo, len(ids))
	for _, photo := range photoRows {
		if photo.ProjectID != nil {
			photosByProject[*photo.ProjectID] = append(photosByProject[*photo.ProjectID], photo)
		}
	}

	aggRows, err := s.repos.Ratings().ListRatingAggregatesByProjectIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	aggregates := make(map[int64]ListRatingAggregate, len(aggRows))
	for _, row := range aggRows {
		aggregates[row.ProjectID] = ListRatingAggregate{
			Count:   int(row.Count),
			Average: row.Average,
		}
	}

	items := make([]dto.Project, 0, len(projects))
	for _, project := range projects {
		items = append(items, assembleProjectDTO(project, technologies[project.ID], photosByProject[project.ID], aggregates[project.ID]))
	}
	return items, nil
}

func (s *UseCases) GetProject(ctx context.Context, id int64) (dto.Project, error) {
	project, err := s.repos.Projects().GetProject(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return dto.Project{}, ErrNotFound
	}
	if err != nil {
		return dto.Project{}, err
	}

	technologies, err := s.repos.Projects().ListTechnologiesByProjectID(ctx, project.ID)
	if err != nil {
		return dto.Project{}, err
	}
	photos, err := s.repos.Photos().ListPhotosByProjectID(ctx, dto.Ptr(project.ID))
	if err != nil {
		return dto.Project{}, err
	}
	countRaw, err := s.repos.Ratings().RatingCountByProjectID(ctx, project.ID)
	if err != nil {
		return dto.Project{}, err
	}
	averageRaw, err := s.repos.Ratings().RatingAverageByProjectID(ctx, project.ID)
	if err != nil {
		return dto.Project{}, err
	}

	return assembleProjectDTO(project, technologies, photos, ListRatingAggregate{
		Count:   int(countRaw),
		Average: averageRaw,
	}), nil
}

func (s *UseCases) createProject(ctx context.Context, req dto.ProjectCreate) (dto.Project, error) {
	project, err := s.repos.Projects().CreateProject(ctx, domain.Project{
		Name: req.Name, Description: req.Description, Featured: req.Featured,
		GithubLink: req.GithubLink, DemoLink: nonEmptyString(req.DemoLink),
		Challenges: req.Challenges, WhatILearned: req.WhatILearned,
		DescriptionFr: nonEmptyString(req.DescriptionFr), ChallengesFr: nonEmptyString(req.ChallengesFr),
		WhatILearnedFr: nonEmptyString(req.WhatILearnedFr),
	})
	if err != nil {
		return dto.Project{}, err
	}
	if err := s.replaceProjectTechnologies(ctx, project.ID, req.Technologies); err != nil {
		return dto.Project{}, err
	}
	return s.GetProject(ctx, project.ID)
}

func (s *UseCases) updateProject(ctx context.Context, id int64, req dto.ProjectUpdate) (dto.Project, error) {
	existing, err := s.repos.Projects().GetProject(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return dto.Project{}, ErrNotFound
	}
	if err != nil {
		return dto.Project{}, err
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.Featured != nil {
		existing.Featured = *req.Featured
	}
	if req.GithubLink != nil {
		existing.GithubLink = req.GithubLink
	}
	if req.DemoLink != nil {
		// An empty string clears the link, so the column holds NULL, not "".
		existing.DemoLink = nonEmptyString(req.DemoLink)
	}
	if req.Challenges != nil {
		existing.Challenges = req.Challenges
	}
	if req.WhatILearned != nil {
		existing.WhatILearned = req.WhatILearned
	}
	// An empty string removes a translation, so the column holds NULL.
	if req.DescriptionFr != nil {
		existing.DescriptionFr = nonEmptyString(req.DescriptionFr)
	}
	if req.ChallengesFr != nil {
		existing.ChallengesFr = nonEmptyString(req.ChallengesFr)
	}
	if req.WhatILearnedFr != nil {
		existing.WhatILearnedFr = nonEmptyString(req.WhatILearnedFr)
	}

	_, err = s.repos.Projects().UpdateProject(ctx, existing)
	if err != nil {
		return dto.Project{}, err
	}
	if req.Technologies != nil {
		if err := s.replaceProjectTechnologies(ctx, id, *req.Technologies); err != nil {
			return dto.Project{}, err
		}
	}
	return s.GetProject(ctx, id)
}

func (s *UseCases) deleteProject(ctx context.Context, id int64) error {
	if err := s.requireProject(ctx, id); err != nil {
		return err
	}
	if err := s.repos.Ratings().DeleteRatingsByProjectID(ctx, id); err != nil {
		return err
	}
	return s.repos.Projects().DeleteProject(ctx, id)
}

func assembleProjectDTO(project domain.Project, technologies []string, photos []domain.Photo, agg ListRatingAggregate) dto.Project {
	photoURLs := make([]string, 0, len(photos))
	photoDTOs := make([]dto.Photo, 0, len(photos))
	for _, photo := range photos {
		photoURLs = append(photoURLs, photo.PhotoURL)
		photoDTOs = append(photoDTOs, dto.Photo{ID: photo.ID, PhotoURL: photo.PhotoURL, ProjectID: project.ID})
	}

	return dto.Project{
		ID:             project.ID,
		Name:           project.Name,
		Description:    project.Description,
		Technologies:   technologies,
		GithubLink:     project.GithubLink,
		DemoLink:       project.DemoLink,
		Challenges:     project.Challenges,
		WhatILearned:   project.WhatILearned,
		DescriptionFr:  project.DescriptionFr,
		ChallengesFr:   project.ChallengesFr,
		WhatILearnedFr: project.WhatILearnedFr,
		Featured:       project.Featured,
		DisplayOrder:   int(project.DisplayOrder),
		CreatedAt:      project.CreatedAt,
		UpdatedAt:      project.UpdatedAt,
		AverageRating:  agg.Average,
		TotalRatings:   agg.Count,
		PhotoUrls:      photoURLs,
		Photos:         photoDTOs,
	}
}

func (s *UseCases) requireProject(ctx context.Context, projectID int64) error {
	exists, err := s.repos.Projects().ProjectExists(ctx, projectID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

func (s *UseCases) replaceProjectTechnologies(ctx context.Context, projectID int64, technologies []string) error {
	if err := s.repos.Projects().DeleteProjectTechnologies(ctx, projectID); err != nil {
		return err
	}
	if len(technologies) > 0 {
		return s.repos.Projects().InsertProjectTechnologies(ctx, projectID, technologies)
	}
	return nil
}

func (s *UseCases) CreateProject(ctx context.Context, req dto.ProjectCreate) (dto.Project, error) {
	return inTransaction(ctx, s, func(tx *UseCases) (dto.Project, error) { return tx.createProject(ctx, req) })
}

func (s *UseCases) UpdateProject(ctx context.Context, id int64, req dto.ProjectUpdate) (dto.Project, error) {
	return inTransaction(ctx, s, func(tx *UseCases) (dto.Project, error) { return tx.updateProject(ctx, id, req) })
}

func (s *UseCases) DeleteProject(ctx context.Context, id int64) error {
	_, err := inTransaction(ctx, s, func(tx *UseCases) (struct{}, error) { return struct{}{}, tx.deleteProject(ctx, id) })
	return err
}

// ReorderProjects sets each project's displayOrder to its index in ids. Every id
// must name an existing project, and no id may repeat. Projects left out of ids
// keep their current displayOrder.
func (s *UseCases) ReorderProjects(ctx context.Context, ids []int64) error {
	_, err := inTransaction(ctx, s, func(tx *UseCases) (struct{}, error) { return struct{}{}, tx.reorderProjects(ctx, ids) })
	return err
}

func (s *UseCases) reorderProjects(ctx context.Context, ids []int64) error {
	known, err := s.repos.Projects().ListProjectIDs(ctx)
	if err != nil {
		return err
	}
	exists := make(map[int64]bool, len(known))
	for _, id := range known {
		exists[id] = true
	}
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if !exists[id] {
			return fmt.Errorf("project %d does not exist: %w", id, ErrBadRequest)
		}
		if seen[id] {
			return fmt.Errorf("project %d is listed more than once: %w", id, ErrBadRequest)
		}
		seen[id] = true
	}
	// A partial list would leave the missing projects tied with listed ones.
	if len(ids) != len(known) {
		return fmt.Errorf("order must list all %d projects, got %d: %w", len(known), len(ids), ErrBadRequest)
	}
	for i, id := range ids {
		if err := s.repos.Projects().SetProjectDisplayOrder(ctx, id, int32(i)); err != nil {
			return err
		}
	}
	return nil
}

// nonEmptyString maps a missing or empty optional string to nil, so an empty
// link is stored as NULL.
func nonEmptyString(value *string) *string {
	if value == nil || *value == "" {
		return nil
	}
	return value
}
