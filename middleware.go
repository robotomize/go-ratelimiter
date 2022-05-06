package ratelimiter

import (
	"net"
	"net/http"
	"strconv"
	"time"
)

const (
	// HeaderRateLimitLimit - maximum number of calls
	HeaderRateLimitLimit = "X-RateLimit-Limit"
	// HeaderRateLimitRemaining - Number of calls before restrictions apply
	HeaderRateLimitRemaining = "X-RateLimit-Remaining"
	// HeaderRateLimitReset - Limit reset time
	HeaderRateLimitReset = "X-RateLimit-Reset"

	// HeaderRetryAfter is the header used to indicate when a client should retry
	// requests (when the rate limit expires), in UTC time.
	HeaderRetryAfter = "Retry-After"
)

// default time format for HeaderRetryAfter, HeaderRateLimitReset
var defaultDateFormat = time.RFC1123

// KeyFunc is a function that accepts an http request and returns a string key
// that uniquely identifies this request for the purpose of rate limiting.
//
// KeyFuncs are called on each request, so be mindful of performance and
// implement caching where possible. If a KeyFunc returns an error, the HTTP
// handler will return Internal Server Error and will NOT take from the limiter
// store.
type KeyFunc func(r *http.Request) (string, error)

type Option func(*Options)

// Options for middleware
type Options struct {
	dateFormat string
	skipper    func() bool
}

// WithDateFormat set custom date format into HeaderRetryAfter/HeaderRateLimitReset
func WithDateFormat(format string) Option {
	return func(options *Options) {
		options.dateFormat = format
	}
}

// WithSkipper set skipper function for skipping
func WithSkipper(skipper func() bool) Option {
	return func(options *Options) {
		options.skipper = skipper
	}
}

// IPKeyFunc rate limit by ip
func IPKeyFunc(headers ...string) KeyFunc {
	return func(r *http.Request) (string, error) {
		// If you want to get the ip from the headers, specify the headers
		// For example X-Forwarded-For
		for _, h := range headers {
			if v := r.Header.Get(h); v != "" {
				return v, nil
			}
		}

		// get from remote addr
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			return "", err
		}

		return ip, nil
	}
}

// LimiterMiddleware returns a handler, which is a rate limiter with data storage in store
func LimiterMiddleware(s Store, keyFunc KeyFunc, opts ...Option) func(next http.Handler) http.Handler {
	opt := Options{dateFormat: defaultDateFormat}
	for _, o := range opts {
		o(&opt)
	}

	// define options
	dateFormat := opt.dateFormat
	skipperFn := opt.skipper

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// check skipper
			if skipperFn != nil {
				// if need skip rate limiter middleware
				if skipperFn() {
					next.ServeHTTP(w, r)

					return
				}
			}

			if keyFunc == nil {
				// if key func is nil return 500 Internal Server Error
				w.WriteHeader(http.StatusInternalServerError)

				return
			}

			// extract entity
			key, err := keyFunc(r)
			if err != nil {
				// if key func return error set 500 Internal Server Error
				w.WriteHeader(http.StatusInternalServerError)

				return
			}

			// fetching limit, remaining and reset time from store
			limit, remaining, t, ok, err := s.Take(ctx, key)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)

				return
			}

			// format reset time
			resetTime := time.Unix(0, int64(t)).UTC().Format(dateFormat)

			// set rate limiter headers
			w.Header().Set(HeaderRateLimitLimit, strconv.FormatUint(limit, 10))
			w.Header().Set(HeaderRateLimitRemaining, strconv.FormatUint(remaining, 10))
			w.Header().Set(HeaderRateLimitReset, resetTime)

			if !ok {
				w.Header().Set(HeaderRetryAfter, resetTime)
				w.WriteHeader(http.StatusTooManyRequests)

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
