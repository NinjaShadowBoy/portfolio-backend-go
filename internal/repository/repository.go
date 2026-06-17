package repository

import "github.com/ninjashadowboy/portfolio-backend-go/internal/db"

type Repository interface {
	Queries() *db.Queries
}

type repository struct {
	queries *db.Queries
}

func New(q *db.Queries) Repository {
	return repository{queries: q}
}

func (r repository) Queries() *db.Queries {
	return r.queries
}
