package ratelimiter

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	redis "github.com/go-redis/redis/v8"
)

const (
	// redis actual points
	redisActualPointsFieldName = "actual_points"
	// redis reset time
	redisResetTimeFieldName = "reset_time"
	// redis max points field name
	redisMaxPointsFieldName = "max_points"
)

const (
	defaultRedisPrefix    = "rate_limiter_redistore_"
	defaultRedisTag       = "goratelimit"
	defaultRedisTagPrefix = "tags:"
)

// RedisConfig - struct for configure redis store
//
// For example if you want to set the limit of requests per second to 10 req per sec
// RedisConfig{ Interval: time.Second * 1, Points: 10}
// or 20 req per 2 minutes RedisConfig{ Interval: time.Minute * 2, Points: 20}
type RedisConfig struct {
	// redis tags for invalidating keys
	Tags []string
	// tagging keys
	TagsPrefix string
	// redis key prefix for redis store
	Prefix string
	// limiter interval
	Interval time.Duration
	// limiter max points
	Points uint64
}

type RedisClient interface {
	HMGet(ctx context.Context, key string, fields ...string) *redis.SliceCmd
	HSet(ctx context.Context, key string, values ...interface{}) *redis.IntCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd
	TxPipeline() redis.Pipeliner
	SMembers(ctx context.Context, key string) *redis.StringSliceCmd
}

// NewRedisStore make redis store
func NewRedisStore(instance RedisClient, cfg RedisConfig) Store {
	var prefix, tagPrefix string
	tags := make([]string, 0, len(cfg.Tags))
	if cfg.Prefix == "" {
		prefix = defaultRedisPrefix
	} else {
		prefix = cfg.Prefix
	}

	if cfg.TagsPrefix == "" {
		tagPrefix = defaultRedisTagPrefix
	} else {
		tagPrefix = cfg.TagsPrefix
	}

	if len(cfg.Tags) == 0 {
		tags = append(tags, fmt.Sprintf("%s%s", tagPrefix, defaultRedisTag))
	} else {
		for idx, tag := range cfg.Tags {
			tags[idx] = fmt.Sprintf("%s%s", tagPrefix, tag)
		}
	}

	return &redisStore{
		client:    instance,
		prefix:    prefix,
		interval:  cfg.Interval,
		maxPoints: cfg.Points,
		tags:      tags,
		tagPrefix: tagPrefix,
	}
}

var _ Store = (*redisStore)(nil)

// redisStore impl of Store
type redisStore struct {
	client RedisClient

	tags      []string
	tagPrefix string
	prefix    string
	maxPoints uint64
	interval  time.Duration
}

// Take returns the actual data on the key or creates a new key. Returns the number of tokens remaining, reset time
func (r redisStore) Take(ctx context.Context, key string) (limit, remaining, resetTime uint64, ok bool, err error) {
	limit, remaining, resetTime, ok, err = r.take(ctx, key)
	if err != nil {
		return 0, 0, 0, false, fmt.Errorf("take: %w", err)
	}

	return limit, remaining, resetTime, ok, nil
}

// Reset clean redis store
func (r redisStore) Reset(ctx context.Context) error {
	if err := r.reset(ctx); err != nil {
		return fmt.Errorf("reset: %w", err)
	}

	return nil
}

// TakeExcl not implemented yet
func (r redisStore) TakeExcl(ctx context.Context, key string, f ExclFunc) (limit, remaining, resetTime uint64, ok bool, err error) {
	// TODO implement me
	panic("implement me")
}

func (r redisStore) take(ctx context.Context, key string) (limit, remaining, resetTimeUint uint64, ok bool, err error) {
	prefixedKey := fmt.Sprintf("%s%s", r.prefix, key)

	//Trying to get points from the current key
	vals, err := r.client.HMGet(ctx, prefixedKey, redisMaxPointsFieldName, redisActualPointsFieldName, redisResetTimeFieldName).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return r.newBucket(ctx, prefixedKey)
		}

		return 0, 0, 0, false, fmt.Errorf("HMGet: %w", err)
	}

	if vals[0] == nil {
		// If the first element is nil, need to create a new value in redis
		return r.newBucket(ctx, prefixedKey)
	}

	var v string

	{
		// Limit type assertion
		v, ok = vals[0].(string)
		if !ok {
			return 0, 0, 0, false, fmt.Errorf("redis mismatch types")
		}

		// parse limit field to uint64
		l, pErr := strconv.ParseUint(v, 0, 64)
		if err != nil {
			return 0, 0, 0, false, fmt.Errorf("parse uint: %w", pErr)
		}

		limit = l
	}

	{
		// Remaining tokens type assertion
		v, ok = vals[1].(string)
		if !ok {
			return 0, 0, 0, false, fmt.Errorf("redis mismatch types")
		}

		// parse remaining field to uint64
		a, pErr := strconv.ParseUint(v, 0, 64)
		if err != nil {
			return 0, 0, 0, false, fmt.Errorf("parse uint: %w", pErr)
		}

		remaining = a
	}

	{
		// Reset time type assertion
		v, ok = vals[2].(string)
		if !ok {
			return 0, 0, 0, false, fmt.Errorf("redis mismatch types")
		}

		// parse reset time field to uint64
		t, pErr := strconv.ParseUint(v, 0, 64)
		if err != nil {
			return 0, 0, 0, false, fmt.Errorf("parse uint: %w", pErr)
		}

		resetTimeUint = t
	}

	// Reduce the number of tokens by one in case of success
	if remaining > 0 {
		// decrement remaining points
		remaining--

		// Update actual tokens
		if err = r.update(ctx, prefixedKey, []string{redisActualPointsFieldName, strconv.FormatUint(remaining, 10)}); err != nil {
			return 0, 0, 0, true, fmt.Errorf("redis hmset: %w", err)
		}

		return limit, remaining, resetTimeUint, true, nil
	}

	return limit, remaining, resetTimeUint, false, nil
}

func (r redisStore) reset(ctx context.Context) error {
	pipe := r.client.TxPipeline()

	defer pipe.Close()

	// Unable to clear state without tags
	if len(r.tags) == 0 {
		return fmt.Errorf("tags not set")
	}

	// Get all keys by tag
	keys, err := r.client.SMembers(ctx, r.tags[0]).Result()
	if err != nil {
		return fmt.Errorf("redis smembers: %w", err)
	}

	// Delete all keys
	_ = pipe.Process(ctx, pipe.Del(ctx, keys...))
	// Delete all tag keys
	_ = pipe.Process(ctx, pipe.Del(ctx, r.tags...))

	if _, err = pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis pipeline exec: %w", err)
	}

	return nil
}

// update actual tokens
func (r redisStore) update(ctx context.Context, key string, fields []string) error {
	if err := r.client.HSet(ctx, key, fields).Err(); err != nil {
		return fmt.Errorf("redis hset: %w", err)
	}

	return nil
}

// hset execute redis HSET and expire command with transaction
func (r redisStore) hset(ctx context.Context, key string, fields []string, expiration time.Duration) error {
	pipe := r.client.TxPipeline()

	defer pipe.Close()

	// Update tag set
	for _, tag := range r.tags {
		// Add key to tag set
		_ = pipe.Process(ctx, pipe.SAdd(ctx, tag, key))
		// Set expire
		_ = pipe.Process(ctx, pipe.Expire(ctx, tag, expiration))
	}

	_ = pipe.Process(ctx, pipe.HSet(ctx, key, fields))
	_ = pipe.Process(ctx, pipe.Expire(ctx, key, expiration))

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis pipeline exec: %w", err)
	}

	return nil
}

// newBucket helper for create new bucket
func (r redisStore) newBucket(ctx context.Context, key string) (limit, remaining, resetTimeUint uint64, ok bool, err error) {
	actual, resetTime := r.maxPoints, uint64(time.Now().Add(r.interval).UnixNano())

	if err = r.hset(
		ctx, key, []string{redisMaxPointsFieldName, strconv.FormatUint(r.maxPoints, 10),
			redisActualPointsFieldName, strconv.FormatUint(actual, 10),
			redisResetTimeFieldName, strconv.FormatUint(resetTime, 10),
		}, r.interval); err != nil {
		return 0, 0, 0, false, fmt.Errorf("redis hmset: %w", err)
	}

	return r.maxPoints, r.maxPoints, resetTime, true, nil
}
