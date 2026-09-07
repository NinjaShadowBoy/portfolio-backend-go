package ports

import "context"

// UnitOfWork commits only when work succeeds. On error or panic it rolls back.
// Use only the callback's repositories inside work; they share one transaction.
// External calls belong outside work. Nested transactions are not supported.
type UnitOfWork interface {
	WithinTx(ctx context.Context, work func(Repositories) error) error
}
