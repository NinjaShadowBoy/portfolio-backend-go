package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

// anonAnimals are the friendly names auto-provisioned anonymous users are given,
// as `anonymous-<animal>-N` where N is that animal's own running counter.
var anonAnimals = []string{
	"goose", "lion", "dolphin", "otter", "panda", "fox", "koala", "tiger",
	"owl", "whale", "lynx", "heron", "bison", "gecko", "raven", "seal",
	"moose", "crane", "ibex", "wren",
}

func (s *UseCases) CreateAnonymousUser(ctx context.Context) (int64, string, error) {
	rawToken, err := randomToken()
	if err != nil {
		return 0, "", err
	}
	animal := anonAnimals[randomIndex(len(anonAnimals))]
	user, err := inTransaction(ctx, s, func(tx *UseCases) (domain.User, error) {
		count, err := tx.repos.Users().CountUsersByNamePrefix(ctx, "anonymous-"+animal+"-%")
		if err != nil {
			return domain.User{}, err
		}
		name := fmt.Sprintf("anonymous-%s-%d", animal, count+1)
		user, err := tx.repos.Users().CreateUser(ctx, domain.User{Email: "anon-" + rawToken + "@anonymous.invalid", Name: name, Provider: "ANON"})
		if err != nil {
			return domain.User{}, err
		}
		if err := tx.repos.AnonymousTokens().CreateAnonToken(ctx, hashToken(rawToken), user.ID); err != nil {
			return domain.User{}, err
		}
		return user, nil
	})
	if err != nil {
		return 0, "", err
	}
	return user.ID, rawToken, nil
}

func (s *UseCases) ResolveAnonToken(ctx context.Context, rawToken string) (int64, bool, error) {
	user, err := s.repos.AnonymousTokens().GetUserByAnonToken(ctx, hashToken(rawToken))
	if errors.Is(err, domain.ErrNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return user.ID, true, nil
}

// GetIdentity returns the caller's public identity for GET /auth/me.

func (s *UseCases) GetIdentity(ctx context.Context, userID int64) (dto.Identity, error) {
	user, err := s.repos.Users().GetUserByID(ctx, userID)
	if errors.Is(err, domain.ErrNotFound) {
		return dto.Identity{}, ErrNotFound
	}
	if err != nil {
		return dto.Identity{}, err
	}
	return identityDTO(user), nil
}

// ClaimAnonymousActivity re-associates an anonymous user's ratings and comments
// to a real account, then retires the anon user. Comments always transfer; for
// the one-per-project star, projects the account already rated keep the
// account's existing star and the anon star is dropped.

func (s *UseCases) claimAnonymousActivity(ctx context.Context, anonUserID, realUserID int64) error {
	if anonUserID == realUserID {
		return nil
	}
	anon, err := s.repos.Users().GetUserByID(ctx, anonUserID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	// Safety: only ever absorb a throwaway anonymous account.
	if anon.Provider != "ANON" {
		return nil
	}
	if err := s.repos.Comments().ReassignCommentsToUser(ctx, realUserID, anonUserID); err != nil {
		return err
	}
	if err := s.repos.Ratings().ReassignRatingsToUser(ctx, realUserID, anonUserID); err != nil {
		return err
	}
	// Any ratings left on the anon user collided with the account's own; drop them.
	if err := s.repos.Ratings().DeleteRatingsByUserID(ctx, anonUserID); err != nil {
		return err
	}
	return s.repos.Users().DeleteUser(ctx, anonUserID)
}

func identityDTO(user domain.User) dto.Identity {
	anon := user.Provider == "ANON"
	identity := dto.Identity{
		ID:        user.ID,
		Name:      user.Name,
		Role:      user.Role,
		Provider:  user.Provider,
		ImageURL:  user.ImageURL,
		Anonymous: anon,
	}
	if !anon {
		identity.Email = user.Email
	}
	return identity
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func hashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

// randomIndex returns a crypto-random index in [0, n). The mod bias across a
// 256-value byte and a 20-element list is negligible for a cosmetic name.

func randomIndex(n int) int {
	if n <= 0 {
		return 0
	}
	buf := make([]byte, 1)
	if _, err := rand.Read(buf); err != nil {
		return 0
	}
	return int(buf[0]) % n
}

func (s *UseCases) ClaimAnonymousActivity(ctx context.Context, anonUserID int64, realUserID int64) error {
	_, err := inTransaction(ctx, s, func(tx *UseCases) (struct{}, error) {
		return struct{}{}, tx.claimAnonymousActivity(ctx, anonUserID, realUserID)
	})
	return err
}
