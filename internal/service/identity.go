package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/db"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

// anonAnimals are the friendly names auto-provisioned anonymous users are given,
// as `anonymous-<animal>-N` where N is that animal's own running counter.
var anonAnimals = []string{
	"goose", "lion", "dolphin", "otter", "panda", "fox", "koala", "tiger",
	"owl", "whale", "lynx", "heron", "bison", "gecko", "raven", "seal",
	"moose", "crane", "ibex", "wren",
}

// CreateAnonymousUser provisions a fresh anonymous user and its opaque token.
// The caller stores the returned raw token in the visitor's cookie; only its
// SHA-256 hash is persisted.
func (s *service) CreateAnonymousUser(ctx context.Context) (int64, string, error) {
	animal := anonAnimals[randomIndex(len(anonAnimals))]
	count, err := s.Q.CountUsersByNamePrefix(ctx, "anonymous-"+animal+"-%")
	if err != nil {
		return 0, "", err
	}
	name := fmt.Sprintf("anonymous-%s-%d", animal, count+1)

	rawToken := randomToken()
	// email is UNIQUE NOT NULL; anon users get a synthetic, unroutable address.
	email := "anon-" + rawToken + "@anonymous.invalid"
	user, err := s.Q.CreateUser(ctx, email, sql.NullString{}, name, "ANON", sql.NullString{}, sql.NullString{}, false, sql.NullTime{})
	if err != nil {
		return 0, "", err
	}
	if err := s.Q.CreateAnonToken(ctx, hashToken(rawToken), user.ID); err != nil {
		return 0, "", err
	}
	return user.ID, rawToken, nil
}

// ResolveAnonToken maps a raw cookie token to its anon user id. ok is false when
// the token is unknown (e.g. cleared server-side), without erroring.
func (s *service) ResolveAnonToken(ctx context.Context, rawToken string) (int64, bool, error) {
	user, err := s.Q.GetUserByAnonToken(ctx, hashToken(rawToken))
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return user.ID, true, nil
}

// GetIdentity returns the caller's public identity for GET /auth/me.
func (s *service) GetIdentity(ctx context.Context, userID int64) (dto.Identity, error) {
	user, err := s.Q.GetUserByID(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
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
func (s *service) ClaimAnonymousActivity(ctx context.Context, anonUserID, realUserID int64) error {
	if anonUserID == realUserID {
		return nil
	}
	anon, err := s.Q.GetUserByID(ctx, anonUserID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	// Safety: only ever absorb a throwaway anonymous account.
	if anon.Provider != "ANON" {
		return nil
	}
	if err := s.Q.ReassignCommentsToUser(ctx, realUserID, anonUserID); err != nil {
		return err
	}
	if err := s.Q.ReassignRatingsToUser(ctx, realUserID, anonUserID); err != nil {
		return err
	}
	// Any ratings left on the anon user collided with the account's own; drop them.
	if err := s.Q.DeleteRatingsByUserID(ctx, anonUserID); err != nil {
		return err
	}
	return s.Q.DeleteUser(ctx, anonUserID)
}

func identityDTO(user db.User) dto.Identity {
	anon := user.Provider == "ANON"
	identity := dto.Identity{
		ID:        user.ID,
		Name:      user.Name,
		Role:      user.Role,
		Provider:  user.Provider,
		ImageURL:  dto.NullString(user.ImageUrl),
		Anonymous: anon,
	}
	if !anon {
		identity.Email = user.Email
	}
	return identity
}

func randomToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
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
