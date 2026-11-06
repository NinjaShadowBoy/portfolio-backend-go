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

// orderedStore holds projects 1, 2 and 3 at display orders 0, 1 and 2.
func orderedStore() *memoryStore {
	store := newStore()
	store.state.projects[1] = domain.Project{ID: 1, Name: "original", DisplayOrder: 0}
	store.state.projects[2] = domain.Project{ID: 2, Name: "second", DisplayOrder: 1}
	store.state.projects[3] = domain.Project{ID: 3, Name: "third", DisplayOrder: 2}
	return store
}

func displayOrders(state memoryState) map[int64]int32 {
	orders := make(map[int64]int32, len(state.projects))
	for id, project := range state.projects {
		orders[id] = project.DisplayOrder
	}
	return orders
}

func TestReorderProjectsSetsOrderFromPosition(t *testing.T) {
	store := orderedStore()
	if err := newUseCases(store).ReorderProjects(context.Background(), []int64{3, 1, 2}); err != nil {
		t.Fatal(err)
	}
	want := map[int64]int32{3: 0, 1: 1, 2: 2}
	if got := displayOrders(store.state); !reflect.DeepEqual(got, want) {
		t.Fatalf("display orders %v, want %v", got, want)
	}
	if store.commits != 1 {
		t.Fatalf("commits = %d, want 1", store.commits)
	}
}

func TestReorderProjectsRejectsUnknownOrRepeatedIDs(t *testing.T) {
	for name, ids := range map[string][]int64{
		"unknown id": {1, 99},
		"duplicate":  {2, 2},
	} {
		t.Run(name, func(t *testing.T) {
			store := orderedStore()
			before := store.state.clone()
			err := newUseCases(store).ReorderProjects(context.Background(), ids)
			if !errors.Is(err, usecase.ErrBadRequest) {
				t.Fatalf("error = %v, want ErrBadRequest", err)
			}
			if !reflect.DeepEqual(store.state, before) {
				t.Fatal("rejected reorder changed committed state")
			}
		})
	}
}

// Projects missing from the list keep their stored order. The handler does
// not require a full list, so this case is reachable.
func TestReorderProjectsRejectsPartialList(t *testing.T) {
	store := orderedStore()
	before := displayOrders(store.state)
	err := newUseCases(store).ReorderProjects(context.Background(), []int64{3})
	if !errors.Is(err, usecase.ErrBadRequest) {
		t.Fatalf("err = %v, want ErrBadRequest", err)
	}
	if got := displayOrders(store.state); !reflect.DeepEqual(got, before) {
		t.Fatalf("display orders changed to %v, want %v", got, before)
	}
}

func TestDemoLinkRoundTrip(t *testing.T) {
	store := newStore()
	svc := newUseCases(store)
	ctx := context.Background()

	project, err := svc.UpdateProject(ctx, 1, dto.ProjectUpdate{DemoLink: dto.Ptr("https://demo.example.com/app")})
	if err != nil {
		t.Fatal(err)
	}
	if project.DemoLink == nil || *project.DemoLink != "https://demo.example.com/app" {
		t.Fatalf("demoLink after set = %v", project.DemoLink)
	}

	// Omitting demoLink in an update leaves the stored link alone.
	if _, err := svc.UpdateProject(ctx, 1, dto.ProjectUpdate{Name: dto.Ptr("renamed")}); err != nil {
		t.Fatal(err)
	}
	if got := store.state.projects[1].DemoLink; got == nil || *got != "https://demo.example.com/app" {
		t.Fatalf("demoLink after unrelated update = %v", got)
	}

	// An empty string clears the link and stores NULL, not "".
	cleared, err := svc.UpdateProject(ctx, 1, dto.ProjectUpdate{DemoLink: dto.Ptr("")})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.DemoLink != nil || store.state.projects[1].DemoLink != nil {
		t.Fatalf("empty demoLink was stored as %v", store.state.projects[1].DemoLink)
	}
}

func TestCreateProjectEmptyDemoLinkIsNull(t *testing.T) {
	store := newStore()
	created, err := newUseCases(store).CreateProject(context.Background(), dto.ProjectCreate{Name: "new", DemoLink: dto.Ptr("")})
	if err != nil {
		t.Fatal(err)
	}
	if created.DemoLink != nil || store.state.projects[2].DemoLink != nil {
		t.Fatalf("empty demoLink was stored as %v", store.state.projects[2].DemoLink)
	}
}

func TestGetProjectReturnsDisplayOrder(t *testing.T) {
	store := orderedStore()
	project, err := newUseCases(store).GetProject(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if project.DisplayOrder != 1 {
		t.Fatalf("displayOrder = %d, want 1", project.DisplayOrder)
	}
}

func TestFrenchContentRoundTrip(t *testing.T) {
	store := newStore()
	svc := newUseCases(store)
	ctx := context.Background()

	project, err := svc.UpdateProject(ctx, 1, dto.ProjectUpdate{
		DescriptionFr:  dto.Ptr("Une description."),
		WhatILearnedFr: dto.Ptr("Les embeddings."),
	})
	if err != nil {
		t.Fatal(err)
	}
	if project.DescriptionFr == nil || *project.DescriptionFr != "Une description." {
		t.Fatalf("descriptionFr after set = %v", project.DescriptionFr)
	}
	if project.ChallengesFr != nil {
		t.Fatalf("challengesFr was not sent but is %v", *project.ChallengesFr)
	}

	// An empty string removes the translation instead of storing "".
	cleared, err := svc.UpdateProject(ctx, 1, dto.ProjectUpdate{DescriptionFr: dto.Ptr("")})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.DescriptionFr != nil || store.state.projects[1].DescriptionFr != nil {
		t.Fatalf("empty descriptionFr was stored as %v", store.state.projects[1].DescriptionFr)
	}
	if cleared.WhatILearnedFr == nil {
		t.Fatal("whatILearnedFr was dropped by an update that did not send it")
	}
}
