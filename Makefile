.PHONY: lint
lint:
	golangci-lint run --timeout 5m --fix -v ./...

.PHONY: test
test:
	go test -race -short ./...

.PHONY: test
test-cover:
	go test -race -timeout=10m ./... -coverprofile=coverage.out
