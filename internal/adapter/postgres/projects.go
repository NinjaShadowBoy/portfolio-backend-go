package postgres

import (
	"context"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

func (r *repositories) CreateProject(ctx context.Context, project domain.Project) (domain.Project, error) {
	row, err := r.q.CreateProject(ctx, project.Name, project.Description, project.Featured, nullString(project.GithubLink), nullString(project.Challenges), nullString(project.WhatILearned), nullString(project.DemoLink))
	return fromProject(row), repositoryError(err)
}

func (r *repositories) DeleteProject(ctx context.Context, id int64) error {
	return repositoryError(r.q.DeleteProject(ctx, id))
}

func (r *repositories) DeleteProjectTechnologies(ctx context.Context, projectID int64) error {
	return repositoryError(r.q.DeleteProjectTechnologies(ctx, projectID))
}

func (r *repositories) GetProject(ctx context.Context, id int64) (domain.Project, error) {
	row, err := r.q.GetProject(ctx, id)
	return fromProject(row), repositoryError(err)
}

func (r *repositories) InsertProjectTechnologies(ctx context.Context, projectID int64, technologies []string) error {
	return repositoryError(r.q.InsertProjectTechnologies(ctx, projectID, technologies))
}

func (r *repositories) ListAllTechnologiesByProjectID(ctx context.Context, ids []int64) ([]domain.ProjectTechnology, error) {
	row, err := r.q.ListAllTechnologiesByProjectID(ctx, ids)
	return mapRows(row, fromProjectTechnology), repositoryError(err)
}

func (r *repositories) ListProjectIDs(ctx context.Context) ([]int64, error) {
	row, err := r.q.ListProjectIDs(ctx)
	return row, repositoryError(err)
}

func (r *repositories) SetProjectDisplayOrder(ctx context.Context, id int64, order int32) error {
	return repositoryError(r.q.SetProjectDisplayOrder(ctx, id, order))
}

func (r *repositories) ListProjects(ctx context.Context) ([]domain.Project, error) {
	row, err := r.q.ListProjects(ctx)
	return mapRows(row, fromProject), repositoryError(err)
}

func (r *repositories) ListTechnologiesByProjectID(ctx context.Context, projectID int64) ([]string, error) {
	row, err := r.q.ListTechnologiesByProjectID(ctx, projectID)
	return row, repositoryError(err)
}

func (r *repositories) ProjectExists(ctx context.Context, id int64) (bool, error) {
	row, err := r.q.ProjectExists(ctx, id)
	return row, repositoryError(err)
}

func (r *repositories) UpdateProject(ctx context.Context, project domain.Project) (domain.Project, error) {
	row, err := r.q.UpdateProject(ctx, project.ID, project.Name, project.Description, project.Featured, nullString(project.GithubLink), nullString(project.Challenges), nullString(project.WhatILearned), nullString(project.DemoLink))
	return fromProject(row), repositoryError(err)
}
