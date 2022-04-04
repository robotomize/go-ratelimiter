package ratelimiter

import "context"

var _ Store = (*noopStore)(nil)

// noopStore - no operation store
type noopStore struct{}

func (s *noopStore) Reset(ctx context.Context) error {
	return nil
}

func (s *noopStore) TakeExcl(ctx context.Context, key string, f ExclFunc) (limit, remaining, resetTime uint64, ok bool, err error) {
	// TODO implement me
	panic("implement me")
}

func NewNoop() Store { return &noopStore{} }

// Take always positive
func (s *noopStore) Take(_ context.Context, _ string) (limit, remaining, resetTime uint64, ok bool, err error) {
	return 0, 0, 0, true, nil
}
