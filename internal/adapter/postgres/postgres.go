package postgres

import (
	"context"
	"database/sql"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/db"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/ports"
)

type Store struct{ db *sql.DB }
type repositories struct{ q *db.Queries }

var _ ports.UnitOfWork = (*Store)(nil)
var _ ports.Repositories = (*repositories)(nil)

func New(db *sql.DB) *Store                                              { return &Store{db: db} }
func (s *Store) Repositories() ports.Repositories                        { return &repositories{q: db.New(s.db)} }
func (r *repositories) Users() ports.UsersRepository                     { return r }
func (r *repositories) Projects() ports.ProjectsRepository               { return r }
func (r *repositories) Ratings() ports.RatingsRepository                 { return r }
func (r *repositories) Comments() ports.CommentsRepository               { return r }
func (r *repositories) Photos() ports.PhotosRepository                   { return r }
func (r *repositories) Contacts() ports.ContactsRepository               { return r }
func (r *repositories) AnonymousTokens() ports.AnonymousTokensRepository { return r }
func (r *repositories) Stats() ports.StatsRepository                     { return r }

func (s *Store) WithinTx(ctx context.Context, work func(ports.Repositories) error) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := work(&repositories{q: db.New(tx)}); err != nil {
		return err
	}
	return tx.Commit()
}
