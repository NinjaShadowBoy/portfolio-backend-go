package ports

import (
	"context"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
)

type ProjectsRepository interface {
	CreateProject(ctx context.Context, project domain.Project) (domain.Project, error)
	DeleteProject(ctx context.Context, id int64) error
	DeleteProjectTechnologies(ctx context.Context, projectID int64) error
	GetProject(ctx context.Context, id int64) (domain.Project, error)
	InsertProjectTechnologies(ctx context.Context, projectID int64, technologies []string) error
	ListAllTechnologiesByProjectID(ctx context.Context, ids []int64) ([]domain.ProjectTechnology, error)
	ListProjects(ctx context.Context) ([]domain.Project, error)
	ListTechnologiesByProjectID(ctx context.Context, projectID int64) ([]string, error)
	ProjectExists(ctx context.Context, id int64) (bool, error)
	UpdateProject(ctx context.Context, project domain.Project) (domain.Project, error)
}
