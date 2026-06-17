package auth

import "database/sql"

func VerifyPlaintextPassword(stored sql.NullString, candidate string) bool {
	return stored.Valid && stored.String == candidate
}
