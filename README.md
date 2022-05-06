# go-ratelimiter

[![Go Report](https://goreportcard.com/badge/github.com/robotomize/go-ratelimiter)](https://goreportcard.com/report/github.com/robotomize/gokuu)
[![codebeat badge](https://codebeat.co/badges/a4a12b24-98e6-4627-b01c-8b124561f2e1)](https://codebeat.co/projects/github-com-robotomize-go-ratelimiter-main)
[![codecov](https://codecov.io/gh/robotomize/go-ratelimiter/branch/main/graph/badge.svg)](https://codecov.io/gh/robotomize/go-ratelimiter)
[![Build status](https://github.com/robotomize/go-ratelimiter/actions/workflows/go.yml/badge.svg)](https://github.com/robotomize/go-ratelimiter/actions)
[![GitHub license](https://img.shields.io/github/license/robotomize/go-ratelimiter.svg)](https://github.com/robotomize/go-ratelimiter/blob/master/LICENSE)

A super easy rate limiting package for Go. Package provide Store interface, for which you can use your own
implementations

# Install

```shell
go get github.com/robotomize/go-ratelimiter
```

# Usage

Example of using redis datastore

```go

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/robotomize/go-ratelimiter"
)

func main() {
	// set a limit of 10 request per 1 seconds per api key
	redisStore, err := ratelimiter.DefaultRedisStore(context.Background(), ":6379", 1*time.Second, 10)
	if err != nil {
		log.Fatal(err)
	}

	// Retrieve data by the api key in the datastore
	limit, remaining, resetTime, ok, err := redisStore.Take(context.Background(), "apikey-1")
	if err != nil {
		log.Fatal(err)
	}

	if !ok {
		fmt.Println("limit exceeded")
	}

	// Print the constraints from the datastore
	fmt.Printf(
		"resource: maximum %d, remaining %d, reset time %s",
		limit, remaining, time.Unix(0, int64(resetTime)).UTC().Format(time.RFC1123),
	)
}

```

To limit access at the http level, you can use middleware, which can block the request by providing the http code 429
Too Many Requests

Example of using http middleware with redis datastore

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/robotomize/go-ratelimiter"
)

func main() {
	// set a limit of 5 request per 1 seconds per api key
	redisStore, err := ratelimiter.DefaultRedisStore(context.Background(), ":6379", 1*time.Second, 5)
	if err != nil {
		log.Fatal(err)
	}

	mx := http.NewServeMux()
	// Let's create a test handler
	healthHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	})
    
	mx.Handle(
		"/health", ratelimiter.LimiterMiddleware(
			redisStore, func(r *http.Request) (string, error) {
				// Example key func
				ctx := r.Context()
				// Get key value out of context
				ctxValue := ctx.Value("apikey")
				if key, ok := ctxValue.(string); ok {
					return key, nil
				}

				return "", errors.New("get api key from ctx")
			}, ratelimiter.WithSkipper(func() bool {
				// set a skipper, skip ratelimiter if DEBUG == 1
				return os.Getenv("DEBUG") == "1" 
			}, ),
		)(healthHandler),
	)

	// start listener
	if err = http.ListenAndServe(":8888", mx); err != nil {
		log.Fatal(err)
	}
}
```

# TODO

* ~add http middleware~
* ~add redis datastore~
* improve unit coverage
* improve redis datastore
* add tarantool datastore
* add aerospike datastore

## Contributing


## License

go-ratelimiter is under the Apache 2.0 license. See the [LICENSE](LICENSE) file for details.
