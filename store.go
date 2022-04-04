package ratelimiter

import (
	"context"
	"time"
)

// ExclFunc defines exceptional parameters for a given key
type ExclFunc func(key string) (ok bool, limit uint64, interval time.Duration)

//go:generate mockgen -source=store.go -destination=mocks.go -package=ratelimiter
type Store interface {
	// Take actual rate limit constraint by key or create new
	Take(ctx context.Context, key string) (limit, remaining, resetTime uint64, ok bool, err error)
	// Reset completely clears the store and resets all tokens
	Reset(ctx context.Context) error
	// TakeExcl actual rate limit constraint by key or create new with exclusive function
	TakeExcl(ctx context.Context, key string, f ExclFunc) (limit, remaining, resetTime uint64, ok bool, err error)
}
