package usecase_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
)

// photoStore gives project 1 photos 10, 11 and 12 at orders 0, 1 and 2, and
// project 2 photo 20 at order 0.
func photoStore() *memoryStore {
	store := newStore()
	store.state.projects[2] = domain.Project{ID: 2, Name: "second"}
	for id, photo := range map[int64]struct {
		project int64
		order   int32
	}{10: {1, 0}, 11: {1, 1}, 12: {1, 2}, 20: {2, 0}} {
		store.state.photos[id] = domain.Photo{ID: id, PhotoURL: "https://example.com/p.jpg", ProjectID: dto.Ptr(photo.project), DisplayOrder: photo.order}
	}
	return store
}

func photoOrders(state memoryState) map[int64]int32 {
	orders := make(map[int64]int32, len(state.photos))
	for id, photo := range state.photos {
		orders[id] = photo.DisplayOrder
	}
	return orders
}

func projectPhotoIDs(t *testing.T, svc usecase.Service, projectID int64) []int64 {
	t.Helper()
	project, err := svc.GetProject(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 0, len(project.Photos))
	for i, photo := range project.Photos {
		if photo.DisplayOrder != i {
			t.Fatalf("photo %d at index %d has displayOrder %d", photo.ID, i, photo.DisplayOrder)
		}
		if project.PhotoUrls[i] != photo.PhotoURL {
			t.Fatalf("photoUrls[%d] = %q, photos[%d] = %q", i, project.PhotoUrls[i], i, photo.PhotoURL)
		}
		ids = append(ids, photo.ID)
	}
	return ids
}

func TestReorderProjectPhotosSetsOrderFromPosition(t *testing.T) {
	store := photoStore()
	svc := newUseCases(store)
	if err := svc.ReorderProjectPhotos(context.Background(), 1, []int64{12, 10, 11}); err != nil {
		t.Fatal(err)
	}
	want := map[int64]int32{12: 0, 10: 1, 11: 2, 20: 0}
	if got := photoOrders(store.state); !reflect.DeepEqual(got, want) {
		t.Fatalf("photo orders %v, want %v", got, want)
	}
	if store.commits != 1 {
		t.Fatalf("commits = %d, want 1", store.commits)
	}
	if got := projectPhotoIDs(t, svc, 1); !reflect.DeepEqual(got, []int64{12, 10, 11}) {
		t.Fatalf("project photos %v, want [12 10 11]", got)
	}
}

func TestReorderProjectPhotosRejectsInvalidOrders(t *testing.T) {
	for name, ids := range map[string][]int64{
		"partial list":          {12, 10},
		"duplicate id":          {12, 12, 10},
		"another project photo": {12, 10, 20},
		"unknown photo":         {12, 10, 99},
		"extra photo":           {12, 10, 11, 20},
	} {
		t.Run(name, func(t *testing.T) {
			store := photoStore()
			before := store.state.clone()
			err := newUseCases(store).ReorderProjectPhotos(context.Background(), 1, ids)
			if !errors.Is(err, usecase.ErrBadRequest) {
				t.Fatalf("error = %v, want ErrBadRequest", err)
			}
			if !reflect.DeepEqual(store.state, before) {
				t.Fatal("rejected reorder changed committed state")
			}
			if store.commits != 0 || store.rollbacks != 1 {
				t.Fatalf("commits=%d, rollbacks=%d", store.commits, store.rollbacks)
			}
		})
	}
}

func TestReorderProjectPhotosRollsBackPartWayFailure(t *testing.T) {
	store := photoStore()
	store.fail = "set-photo-order-11"
	before := store.state.clone()
	err := newUseCases(store).ReorderProjectPhotos(context.Background(), 1, []int64{12, 10, 11})
	if !errors.Is(err, injectedFailure) {
		t.Fatalf("error = %v, want injected failure", err)
	}
	if !reflect.DeepEqual(store.state, before) {
		t.Fatal("failed reorder kept the photos it moved before the failure")
	}
	if store.commits != 0 || store.rollbacks != 1 {
		t.Fatalf("commits=%d, rollbacks=%d", store.commits, store.rollbacks)
	}
}

func TestReorderProjectPhotosUnknownProject(t *testing.T) {
	store := photoStore()
	before := store.state.clone()
	err := newUseCases(store).ReorderProjectPhotos(context.Background(), 99, []int64{10})
	if !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if !reflect.DeepEqual(store.state, before) {
		t.Fatal("reorder of an unknown project changed state")
	}
}

func TestUploadedPhotoGoesLast(t *testing.T) {
	store := photoStore()
	svc := newUseCases(store)
	if err := svc.ReorderProjectPhotos(context.Background(), 1, []int64{12, 10, 11}); err != nil {
		t.Fatal(err)
	}
	photo, err := svc.SaveProjectPhoto(context.Background(), "https://example.com/new.jpg", 1)
	if err != nil {
		t.Fatal(err)
	}
	if photo.DisplayOrder != 3 || photo.ProjectID != 1 {
		t.Fatalf("uploaded photo = %+v, want displayOrder 3 in project 1", photo)
	}
	if got := projectPhotoIDs(t, svc, 1); !reflect.DeepEqual(got, []int64{12, 10, 11, photo.ID}) {
		t.Fatalf("project photos %v, want [12 10 11 %d]", got, photo.ID)
	}
	// The old full list is now partial: the new photo must be placed too.
	if err := svc.ReorderProjectPhotos(context.Background(), 1, []int64{12, 10, 11}); !errors.Is(err, usecase.ErrBadRequest) {
		t.Fatalf("error = %v, want ErrBadRequest", err)
	}
}
