package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/ports"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
)

// adminRepos stubs only what the admin use cases touch; the embedded
// interfaces panic if anything else is called.
type adminRepos struct {
	ports.Repositories
	ports.UsersRepository
	ports.CommentsRepository
	users       map[int64]domain.User
	roles       map[int64]string
	deleted     []int64
	userFilter  domain.UserFilter
	comments    map[int64]bool
	commentSeen domain.CommentFilter
}

func (r *adminRepos) Users() ports.UsersRepository       { return r }
func (r *adminRepos) Comments() ports.CommentsRepository { return r }

func (r *adminRepos) GetUserByID(_ context.Context, id int64) (domain.User, error) {
	user, ok := r.users[id]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return user, nil
}
func (r *adminRepos) UpdateUserRole(_ context.Context, id int64, role string) (domain.User, error) {
	r.roles[id] = role
	user := r.users[id]
	user.Role = role
	return user, nil
}
func (r *adminRepos) DeleteUser(_ context.Context, id int64) error {
	r.deleted = append(r.deleted, id)
	return nil
}
func (r *adminRepos) CountUsersForAdmin(_ context.Context, filter domain.UserFilter) (int32, error) {
	r.userFilter = filter
	return 1, nil
}
func (r *adminRepos) ListUsersForAdmin(_ context.Context, filter domain.UserFilter) ([]domain.AdminUser, error) {
	return []domain.AdminUser{{User: domain.User{ID: 8, PasswordHash: dto.Ptr("hash")}, CommentCount: 2}}, nil
}
func (r *adminRepos) CountCommentsForAdmin(_ context.Context, filter domain.CommentFilter) (int32, error) {
	r.commentSeen = filter
	return 0, nil
}
func (r *adminRepos) ListCommentsForAdmin(context.Context, domain.CommentFilter) ([]domain.AdminComment, error) {
	return nil, nil
}
func (r *adminRepos) DeleteCommentByID(_ context.Context, id int64) (int64, error) {
	if !r.comments[id] {
		return 0, nil
	}
	delete(r.comments, id)
	return 1, nil
}

func newAdminUseCases() (*adminRepos, usecase.Service) {
	repos := &adminRepos{
		users: map[int64]domain.User{
			1: {ID: 1, Role: "ADMIN", Provider: "LOCAL"},
			7: {ID: 7, Role: "USER", Provider: "ANON"},
			8: {ID: 8, Role: "USER", Provider: "GITHUB"},
		},
		roles:    map[int64]string{},
		comments: map[int64]bool{5: true},
	}
	return repos, usecase.New(usecase.Dependencies{Repositories: repos})
}

func TestUpdateUserRoleGuards(t *testing.T) {
	for _, tc := range []struct {
		name         string
		actor, user  int64
		role         string
		want         error
		wantRoleSave bool
	}{
		{"promotes another account", 1, 8, "ADMIN", nil, true},
		{"refuses own role", 1, 1, "USER", usecase.ErrBadRequest, false},
		{"refuses anonymous identity", 1, 7, "ADMIN", usecase.ErrBadRequest, false},
		{"refuses unknown role", 1, 8, "OWNER", usecase.ErrBadRequest, false},
		{"missing user", 1, 99, "USER", usecase.ErrNotFound, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repos, svc := newAdminUseCases()
			user, err := svc.UpdateUserRole(context.Background(), tc.actor, tc.user, tc.role)
			if !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
				t.Fatalf("got error %v, want %v", err, tc.want)
			}
			_, saved := repos.roles[tc.user]
			if saved != tc.wantRoleSave {
				t.Fatalf("role saved = %v, want %v", saved, tc.wantRoleSave)
			}
			if tc.wantRoleSave && user.Role != tc.role {
				t.Fatalf("returned role %q, want %q", user.Role, tc.role)
			}
		})
	}
}

func TestDeleteUserAsAdminGuards(t *testing.T) {
	repos, svc := newAdminUseCases()
	if err := svc.DeleteUserAsAdmin(context.Background(), 1, 1); !errors.Is(err, usecase.ErrBadRequest) {
		t.Fatalf("self delete: got %v, want ErrBadRequest", err)
	}
	if err := svc.DeleteUserAsAdmin(context.Background(), 1, 99); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("missing user: got %v, want ErrNotFound", err)
	}
	if err := svc.DeleteUserAsAdmin(context.Background(), 1, 8); err != nil {
		t.Fatalf("delete other user: %v", err)
	}
	if len(repos.deleted) != 1 || repos.deleted[0] != 8 {
		t.Fatalf("deleted %v, want [8]", repos.deleted)
	}
}

func TestListUsersForAdminAppliesPageBoundsAndSearch(t *testing.T) {
	repos, svc := newAdminUseCases()
	page, err := svc.ListUsersForAdmin(context.Background(), dto.AdminUserQuery{Search: "  ann ", Limit: 5000})
	if err != nil {
		t.Fatal(err)
	}
	if repos.userFilter.Limit != 200 || page.Limit != 200 {
		t.Fatalf("limit %d / page %d, want both capped at 200", repos.userFilter.Limit, page.Limit)
	}
	if repos.userFilter.Search == nil || *repos.userFilter.Search != "ann" {
		t.Fatalf("search %v, want trimmed \"ann\"", repos.userFilter.Search)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].CommentCount != 2 {
		t.Fatalf("unexpected page %+v", page)
	}

	if _, err := svc.ListUsersForAdmin(context.Background(), dto.AdminUserQuery{}); err != nil {
		t.Fatal(err)
	}
	if repos.userFilter.Limit != 50 || repos.userFilter.Search != nil {
		t.Fatalf("defaults: got limit %d search %v, want 50 and nil", repos.userFilter.Limit, repos.userFilter.Search)
	}
}

func TestListCommentsForAdminValidatesFilters(t *testing.T) {
	repos, svc := newAdminUseCases()
	if _, err := svc.ListCommentsForAdmin(context.Background(), dto.AdminCommentQuery{Type: "spam"}); !errors.Is(err, usecase.ErrBadRequest) {
		t.Fatalf("bad type: got %v, want ErrBadRequest", err)
	}
	if _, err := svc.ListCommentsForAdmin(context.Background(), dto.AdminCommentQuery{Target: "user"}); !errors.Is(err, usecase.ErrBadRequest) {
		t.Fatalf("bad target: got %v, want ErrBadRequest", err)
	}
	page, err := svc.ListCommentsForAdmin(context.Background(), dto.AdminCommentQuery{Type: "suggestion", Target: "article"})
	if err != nil {
		t.Fatal(err)
	}
	if *repos.commentSeen.Type != "suggestion" || *repos.commentSeen.Target != "article" {
		t.Fatalf("filter %+v not passed through", repos.commentSeen)
	}
	if page.Items == nil {
		t.Fatal("items must serialize as [] rather than null")
	}
}

func TestDeleteCommentAsAdmin(t *testing.T) {
	_, svc := newAdminUseCases()
	if err := svc.DeleteCommentAsAdmin(context.Background(), 5); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteCommentAsAdmin(context.Background(), 5); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("second delete: got %v, want ErrNotFound", err)
	}
}
