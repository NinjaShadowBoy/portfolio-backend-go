package auth

import (
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword returns a bcrypt hash of the given plaintext password, suitable
// for storing in the users.pwd column.
func HashPassword(plaintext string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword reports whether candidate matches the stored password.
//
// Stored values are expected to be bcrypt hashes. For backward compatibility
// with rows created before hashing was introduced, a stored value that is not a
// bcrypt hash is compared as plaintext.
func VerifyPassword(stored *string, candidate string) bool {
	if stored == nil {
		return false
	}

	value := *stored
	if isBcryptHash(value) {
		return bcrypt.CompareHashAndPassword([]byte(value), []byte(candidate)) == nil
	}

	// Legacy plaintext value.
	return value == candidate
}

func isBcryptHash(value string) bool {
	// All bcrypt variants ($2a$, $2b$, $2y$) share the "$2" prefix.
	return strings.HasPrefix(value, "$2")
}

// Passwords adapts bcrypt and the legacy verification policy to the use-case port.
type Passwords struct{}

func (Passwords) Hash(value string) (string, error) { return HashPassword(value) }
func (Passwords) Verify(stored *string, candidate string) bool {
	return VerifyPassword(stored, candidate)
}
