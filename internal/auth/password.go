package auth

import (
	"database/sql"
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
func VerifyPassword(stored sql.NullString, candidate string) bool {
	if !stored.Valid {
		return false
	}

	value := stored.String
	if isBcryptHash(value) {
		return bcrypt.CompareHashAndPassword([]byte(value), []byte(candidate)) == nil
	}

	// Legacy plaintext value.
	return value == candidate
}

func isBcryptHash(value string) bool {
	return strings.HasPrefix(value, "$2a$") ||
		strings.HasPrefix(value, "$2b$") ||
		strings.HasPrefix(value, "$2y$")
}
