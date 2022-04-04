package ratelimiter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
)

func TestLimiterMiddleware(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string

		limit    uint64
		interval time.Duration

		remains uint64
		ok      bool

		err error

		expectedLimit                    string
		expectedRemaining                string
		expectedHeaderRateLimit          string
		expectedHeaderRateLimitRemaining string
		expectedStatusCode               int
		expectedStatus                   string
	}{
		{
			limit:   10,
			name:    "test_ok_remains_3_200",
			ok:      true,
			remains: 3,

			expectedLimit:      "10",
			expectedRemaining:  "3",
			expectedStatusCode: http.StatusOK,
			expectedStatus:     "200 OK",
		},
		{
			limit:   10,
			name:    "test_ok_remains_0_429",
			ok:      false,
			remains: 0,

			expectedLimit:      "10",
			expectedRemaining:  "0",
			expectedStatusCode: http.StatusTooManyRequests,
			expectedStatus:     "429 Too Many Requests",
		},
		{
			limit:   10,
			name:    "test_err_500",
			ok:      false,
			remains: 0,

			err: errors.New("mock error"),

			expectedLimit:      "",
			expectedRemaining:  "",
			expectedStatusCode: http.StatusInternalServerError,
			expectedStatus:     "500 Internal Server Error",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mx := http.NewServeMux()
			deps := testProvideMockDeps(t)
			deps.store.
				EXPECT().
				Take(gomock.Any(), gomock.Any()).
				Return(tc.limit, tc.remains, uint64(0), tc.ok, nil).
				AnyTimes()

			mw := LimiterMiddleware(deps.store, func(r *http.Request) (string, error) {
				return "1234", tc.err
			})

			mx.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			s := httptest.NewServer(mw(mx))

			client := s.Client()

			u, err := url.Parse(s.URL + "/test")
			if err != nil {
				t.Fatalf("url parse: %v", err)
			}

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u.String(), http.NoBody)
			if err != nil {
				t.Fatalf("unable create request: %v", err)

				return
			}

			res, err := client.Do(req)
			if err != nil {
				t.Fatalf("do request: %v", err)

				return
			}

			defer res.Body.Close()

			if diff := cmp.Diff(tc.expectedStatusCode, res.StatusCode); diff != "" {
				t.Errorf("bad body (+got, -want): %s", diff)
			}

			if diff := cmp.Diff(tc.expectedStatus, res.Status); diff != "" {
				t.Errorf("bad body (+got, -want): %s", diff)
			}

			if diff := cmp.Diff(tc.expectedLimit, res.Header.Get(HeaderRateLimitLimit)); diff != "" {
				t.Errorf("bad body (+got, -want): %s", diff)
			}

			if diff := cmp.Diff(tc.expectedRemaining, res.Header.Get(HeaderRateLimitRemaining)); diff != "" {
				t.Errorf("bad body (+got, -want): %s", diff)
			}
		})
	}
}

type mockDeps struct {
	ctrl  *gomock.Controller
	store *MockStore
}

func testProvideMockDeps(t *testing.T) mockDeps {
	var deps mockDeps
	// provide mock objects for the handler
	deps.ctrl = gomock.NewController(t)
	deps.store = NewMockStore(deps.ctrl)

	return deps
}
