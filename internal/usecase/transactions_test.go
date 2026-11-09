package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/ports"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
)

var injectedFailure = errors.New("injected write failure")

// The memory adapter publishes its private snapshot only on commit. Root
// repositories reject writes, so tests also catch use cases escaping the UOW.
type memoryState struct {
	projects     map[int64]domain.Project
	technologies map[int64][]string
	users        map[int64]domain.User
	tokens       map[string]int64
	comments     map[int64]int64
	ratings      map[int64]int64
	photos       map[int64]domain.Photo
	profilePhoto string
}

func (s memoryState) clone() memoryState {
	s.projects = maps.Clone(s.projects)
	s.photos = maps.Clone(s.photos)
	s.technologies = maps.Clone(s.technologies)
	for id, values := range s.technologies {
		s.technologies[id] = slices.Clone(values)
	}
	s.users = maps.Clone(s.users)
	s.tokens = maps.Clone(s.tokens)
	s.comments = maps.Clone(s.comments)
	s.ratings = maps.Clone(s.ratings)
	return s
}

type memoryStore struct {
	state     memoryState
	fail      string
	commits   int
	rollbacks int
}

func newStore() *memoryStore {
	return &memoryStore{state: memoryState{
		projects:     map[int64]domain.Project{1: {ID: 1, Name: "original"}},
		technologies: map[int64][]string{1: {"Go"}},
		users:        map[int64]domain.User{7: {ID: 7, Provider: "ANON"}, 8: {ID: 8, Provider: "LOCAL"}},
		photos:       map[int64]domain.Photo{},
		tokens:       map[string]int64{"old-token": 7}, comments: map[int64]int64{1: 7}, ratings: map[int64]int64{1: 7}, profilePhoto: "old-photo",
	}}
}
func (s *memoryStore) repos() ports.Repositories { return &memoryRepos{state: &s.state, fail: s.fail} }
func (s *memoryStore) WithinTx(ctx context.Context, work func(ports.Repositories) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot := s.state.clone()
	if err := work(&memoryRepos{state: &snapshot, fail: s.fail, transactional: true}); err != nil {
		s.rollbacks++
		return err
	}
	s.state = snapshot
	s.commits++
	return nil
}
func newUseCases(s *memoryStore) usecase.Service {
	return usecase.New(usecase.Dependencies{Repositories: s.repos(), UnitOfWork: s})
}

type memoryRepos struct {
	ports.Repositories
	ports.ProjectsRepository
	ports.UsersRepository
	ports.RatingsRepository
	ports.CommentsRepository
	ports.PhotosRepository
	ports.AnonymousTokensRepository
	state         *memoryState
	fail          string
	transactional bool
}

func (r *memoryRepos) Projects() ports.ProjectsRepository               { return r }
func (r *memoryRepos) Users() ports.UsersRepository                     { return r }
func (r *memoryRepos) Ratings() ports.RatingsRepository                 { return r }
func (r *memoryRepos) Comments() ports.CommentsRepository               { return r }
func (r *memoryRepos) Photos() ports.PhotosRepository                   { return r }
func (r *memoryRepos) AnonymousTokens() ports.AnonymousTokensRepository { return r }
func (r *memoryRepos) write(step string) error {
	if !r.transactional {
		return errors.New("write escaped the unit of work")
	}
	if r.fail == step {
		return injectedFailure
	}
	return nil
}
func (r *memoryRepos) GetProject(_ context.Context, id int64) (domain.Project, error) {
	project, ok := r.state.projects[id]
	if !ok {
		return domain.Project{}, domain.ErrNotFound
	}
	return project, nil
}
func (r *memoryRepos) ProjectExists(_ context.Context, id int64) (bool, error) {
	_, ok := r.state.projects[id]
	return ok, nil
}
func (r *memoryRepos) CreateProject(_ context.Context, project domain.Project) (domain.Project, error) {
	if err := r.write("create-project"); err != nil {
		return domain.Project{}, err
	}
	project.ID = 2
	r.state.projects[2] = project
	return project, nil
}
func (r *memoryRepos) UpdateProject(_ context.Context, project domain.Project) (domain.Project, error) {
	if err := r.write("update-project"); err != nil {
		return domain.Project{}, err
	}
	id := project.ID
	r.state.projects[id] = project
	return project, nil
}
func (r *memoryRepos) DeleteProject(_ context.Context, id int64) error {
	if err := r.write("delete-project"); err != nil {
		return err
	}
	delete(r.state.projects, id)
	delete(r.state.technologies, id)
	return nil
}
func (r *memoryRepos) DeleteProjectTechnologies(_ context.Context, id int64) error {
	if err := r.write("delete-technologies"); err != nil {
		return err
	}
	delete(r.state.technologies, id)
	return nil
}
func (r *memoryRepos) InsertProjectTechnologies(_ context.Context, id int64, values []string) error {
	if err := r.write("insert-technologies"); err != nil {
		return err
	}
	r.state.technologies[id] = slices.Clone(values)
	return nil
}
func (r *memoryRepos) ListProjectIDs(context.Context) ([]int64, error) {
	ids := make([]int64, 0, len(r.state.projects))
	for id := range r.state.projects {
		ids = append(ids, id)
	}
	return ids, nil
}
func (r *memoryRepos) SetProjectDisplayOrder(_ context.Context, id int64, order int32) error {
	if err := r.write("set-display-order"); err != nil {
		return err
	}
	project := r.state.projects[id]
	project.DisplayOrder = order
	r.state.projects[id] = project
	return nil
}
func (r *memoryRepos) ListTechnologiesByProjectID(_ context.Context, id int64) ([]string, error) {
	return r.state.technologies[id], nil
}

// ListPhotosByProjectID returns the project's photos by display order, then id,
// as the SQL query does.
func (r *memoryRepos) ListPhotosByProjectID(_ context.Context, projectID *int64) ([]domain.Photo, error) {
	if r.fail == "load-photos" {
		return nil, injectedFailure
	}
	var photos []domain.Photo
	for _, photo := range r.state.photos {
		if projectID != nil && photo.ProjectID != nil && *photo.ProjectID == *projectID {
			photos = append(photos, photo)
		}
	}
	slices.SortFunc(photos, func(a, b domain.Photo) int {
		if a.DisplayOrder != b.DisplayOrder {
			return int(a.DisplayOrder - b.DisplayOrder)
		}
		return int(a.ID - b.ID)
	})
	return photos, nil
}

// SaveProjectPhoto appends after the project's last photo, as the SQL query
// does. The use case runs it as a single statement outside a transaction, so
// it does not go through write.
func (r *memoryRepos) SaveProjectPhoto(_ context.Context, url string, projectID *int64) (domain.Photo, error) {
	if r.fail == "save-project-photo" {
		return domain.Photo{}, injectedFailure
	}
	next := int32(0)
	var maxID int64
	for id, photo := range r.state.photos {
		maxID = max(maxID, id)
		if photo.ProjectID != nil && *photo.ProjectID == *projectID {
			next = max(next, photo.DisplayOrder+1)
		}
	}
	photo := domain.Photo{ID: maxID + 1, PhotoURL: url, ProjectID: projectID, DisplayOrder: next}
	r.state.photos[photo.ID] = photo
	return photo, nil
}

// SetPhotoDisplayOrder fails on "set-photo-order", or on
// "set-photo-order-<id>" for one photo so a reorder can fail part way.
func (r *memoryRepos) SetPhotoDisplayOrder(_ context.Context, projectID, photoID int64, position int32) (int64, error) {
	if err := r.write("set-photo-order"); err != nil {
		return 0, err
	}
	if r.fail == fmt.Sprintf("set-photo-order-%d", photoID) {
		return 0, injectedFailure
	}
	photo, ok := r.state.photos[photoID]
	if !ok || photo.ProjectID == nil || *photo.ProjectID != projectID {
		return 0, nil
	}
	photo.DisplayOrder = position
	r.state.photos[photoID] = photo
	return 1, nil
}
func (r *memoryRepos) RatingCountByProjectID(context.Context, int64) (int32, error) { return 0, nil }
func (r *memoryRepos) RatingAverageByProjectID(context.Context, int64) (float64, error) {
	return 0, nil
}
func (r *memoryRepos) DeleteRatingsByProjectID(context.Context, int64) error {
	if err := r.write("delete-project-ratings"); err != nil {
		return err
	}
	clear(r.state.ratings)
	return nil
}
func (r *memoryRepos) CountUsersByNamePrefix(context.Context, string) (int32, error) { return 0, nil }
func (r *memoryRepos) CreateUser(_ context.Context, user domain.User) (domain.User, error) {
	if err := r.write("create-user"); err != nil {
		return domain.User{}, err
	}
	user.ID = 9
	r.state.users[user.ID] = user
	return user, nil
}
func (r *memoryRepos) CreateAnonToken(_ context.Context, hash string, id int64) error {
	if err := r.write("create-token"); err != nil {
		return err
	}
	r.state.tokens[hash] = id
	return nil
}
func (r *memoryRepos) GetUserByID(_ context.Context, id int64) (domain.User, error) {
	user, ok := r.state.users[id]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return user, nil
}
func (r *memoryRepos) ReassignCommentsToUser(_ context.Context, target, source int64) error {
	if err := r.write("transfer-comments"); err != nil {
		return err
	}
	for id, userID := range r.state.comments {
		if userID == source {
			r.state.comments[id] = target
		}
	}
	return nil
}
func (r *memoryRepos) ReassignRatingsToUser(_ context.Context, target, source int64) error {
	if err := r.write("transfer-ratings"); err != nil {
		return err
	}
	for id, userID := range r.state.ratings {
		if userID == source {
			r.state.ratings[id] = target
		}
	}
	return nil
}
func (r *memoryRepos) DeleteRatingsByUserID(_ context.Context, userID int64) error {
	if err := r.write("delete-user-ratings"); err != nil {
		return err
	}
	for id, owner := range r.state.ratings {
		if owner == userID {
			delete(r.state.ratings, id)
		}
	}
	return nil
}
func (r *memoryRepos) DeleteUser(_ context.Context, id int64) error {
	if err := r.write("delete-user"); err != nil {
		return err
	}
	delete(r.state.users, id)
	for token, owner := range r.state.tokens {
		if owner == id {
			delete(r.state.tokens, token)
		}
	}
	return nil
}
func (r *memoryRepos) DeleteOrphanProfilePhotos(context.Context) error {
	if err := r.write("delete-profile-photo"); err != nil {
		return err
	}
	r.state.profilePhoto = ""
	return nil
}
func (r *memoryRepos) SaveProfilePhoto(_ context.Context, url string) (domain.Photo, error) {
	if err := r.write("save-profile-photo"); err != nil {
		return domain.Photo{}, err
	}
	r.state.profilePhoto = url
	return domain.Photo{ID: 1, PhotoURL: url}, nil
}

func TestMultiWriteUseCasesRollBack(t *testing.T) {
	for _, tc := range []struct {
		name, fail string
		run        func(usecase.Service) error
	}{
		{"project create", "insert-technologies", func(s usecase.Service) error {
			_, err := s.CreateProject(context.Background(), dto.ProjectCreate{Name: "new", Technologies: []string{"Go"}})
			return err
		}},
		{"project update", "insert-technologies", func(s usecase.Service) error {
			_, err := s.UpdateProject(context.Background(), 1, dto.ProjectUpdate{Name: dto.Ptr("changed"), Technologies: dto.Ptr([]string{"TS"})})
			return err
		}},
		{"project response read", "load-photos", func(s usecase.Service) error {
			_, err := s.CreateProject(context.Background(), dto.ProjectCreate{Name: "new"})
			return err
		}},
		{"project delete", "delete-project", func(s usecase.Service) error { return s.DeleteProject(context.Background(), 1) }},
		{"project reorder", "set-display-order", func(s usecase.Service) error { return s.ReorderProjects(context.Background(), []int64{1}) }},
		{"anonymous identity", "create-token", func(s usecase.Service) error {
			id, token, err := s.CreateAnonymousUser(context.Background())
			if id != 0 || token != "" {
				t.Error("failed transaction exposed identity")
			}
			return err
		}},
		{"activity transfer", "transfer-ratings", func(s usecase.Service) error { return s.ClaimAnonymousActivity(context.Background(), 7, 8) }},
		{"account retirement", "delete-user", func(s usecase.Service) error { return s.ClaimAnonymousActivity(context.Background(), 7, 8) }},
		{"profile photo", "save-profile-photo", func(s usecase.Service) error {
			_, err := s.SaveProfilePhoto(context.Background(), "https://example.com/new.jpg")
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newStore()
			store.fail = tc.fail
			before := store.state.clone()
			if err := tc.run(newUseCases(store)); !errors.Is(err, injectedFailure) {
				t.Fatalf("error = %v", err)
			}
			if !reflect.DeepEqual(store.state, before) {
				t.Fatal("failed operation changed committed state")
			}
			if store.commits != 0 || store.rollbacks != 1 {
				t.Fatalf("commits=%d, rollbacks=%d", store.commits, store.rollbacks)
			}
		})
	}
}

func TestProjectWriteCommitsAllChanges(t *testing.T) {
	store := newStore()
	project, err := newUseCases(store).UpdateProject(context.Background(), 1, dto.ProjectUpdate{Name: dto.Ptr("changed"), Technologies: dto.Ptr([]string{"TS"})})
	if err != nil {
		t.Fatal(err)
	}
	if project.Name != "changed" || store.state.projects[1].Name != "changed" || !slices.Equal(store.state.technologies[1], []string{"TS"}) || store.commits != 1 {
		t.Fatalf("incomplete commit: %+v", store)
	}
}

func TestAnonymousActivityCommitsTogether(t *testing.T) {
	store := newStore()
	if err := newUseCases(store).ClaimAnonymousActivity(context.Background(), 7, 8); err != nil {
		t.Fatal(err)
	}
	if _, exists := store.state.users[7]; exists {
		t.Fatal("anonymous account was not retired")
	}
	if store.state.comments[1] != 8 || store.state.ratings[1] != 8 || len(store.state.tokens) != 0 || store.commits != 1 {
		t.Fatalf("incomplete claim: %+v", store)
	}
}

type oauthStub struct {
	ports.OAuthGateway
	status domain.DeviceAuthStatus
}

func (s oauthStub) Exchange(context.Context, domain.OAuthProvider, string, string, string) (domain.OAuthProfile, error) {
	return domain.OAuthProfile{Provider: domain.OAuthProviderGoogle, ProviderID: "123", Email: "oauth@example.com", DisplayName: "Reader", EmailVerified: true}, nil
}
func (s oauthStub) PollDevice(ctx context.Context, _ string) (domain.OAuthProfile, domain.DeviceAuthStatus, error) {
	profile, err := s.Exchange(ctx, domain.OAuthProviderGoogle, "", "", "")
	return profile, s.status, err
}

type tokenStub struct {
	store  *memoryStore
	called bool
}

func (s *tokenStub) GenerateLoginResponse(user domain.User) (dto.LoginResponse, error) {
	s.called = true
	if s.store.commits != 1 || user.LastLoginAt == nil {
		return dto.LoginResponse{}, errors.New("token issued before the login commit")
	}
	return dto.LoginResponse{Token: "signed", User: dto.FromUser(user)}, nil
}
func (r *memoryRepos) GetUserByProvider(context.Context, string, *string) (domain.User, error) {
	return domain.User{}, domain.ErrNotFound
}
func (r *memoryRepos) UserExistsByEmail(context.Context, string) (bool, error) { return false, nil }
func (r *memoryRepos) UpdateUserLogin(_ context.Context, id int64, lastLogin *time.Time) (domain.User, error) {
	if err := r.write("update-login"); err != nil {
		return domain.User{}, err
	}
	user := r.state.users[id]
	user.LastLoginAt = lastLogin
	r.state.users[id] = user
	return user, nil
}

func TestOAuthAccountAndLoginCommitBeforeToken(t *testing.T) {
	for _, device := range []bool{false, true} {
		for _, fail := range []string{"", "update-login"} {
			store := newStore()
			store.fail = fail
			before := store.state.clone()
			tokens := &tokenStub{store: store}
			svc := usecase.New(usecase.Dependencies{Repositories: store.repos(), UnitOfWork: store, Tokens: tokens, OAuth: oauthStub{status: domain.DeviceAuthComplete}})
			var response dto.LoginResponse
			var err error
			if device {
				response, _, err = svc.PollGitHubDeviceLogin(context.Background(), "device")
			} else {
				response, err = svc.OAuthLogin(context.Background(), domain.OAuthProviderGoogle, "code", "state", "verifier")
			}
			if fail != "" {
				if !errors.Is(err, injectedFailure) || tokens.called || response.Token != "" || !reflect.DeepEqual(before, store.state) {
					t.Fatalf("failed OAuth login was not atomic: %v", err)
				}
			} else if err != nil || !tokens.called || response.Token != "signed" || store.commits != 1 {
				t.Fatalf("OAuth login failed: %v", err)
			}
		}
	}
}

func TestPendingDeviceLoginDoesNotOpenTransaction(t *testing.T) {
	store := newStore()
	svc := usecase.New(usecase.Dependencies{UnitOfWork: store, OAuth: oauthStub{status: domain.DeviceAuthPending}})
	_, status, err := svc.PollGitHubDeviceLogin(context.Background(), "device")
	if err != nil || status != domain.DeviceAuthPending || store.commits != 0 || store.rollbacks != 0 {
		t.Fatalf("pending login touched persistence: %v", err)
	}
}
