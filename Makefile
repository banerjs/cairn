.PHONY: fmt lint test integration build-all

fmt:
	gofmt -s -w .

lint:
	golangci-lint run ./...

test:
	go test ./...

integration:
	go test -tags=integration -timeout 30m ./internal/integration/...

build-all: ## Cross-compile release binaries into ./bin/
	mkdir -p bin
	GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/cairn-linux-amd64 ./cmd/cairn
	GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o bin/cairn-linux-arm64 ./cmd/cairn
	GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/cairn-darwin-amd64 ./cmd/cairn
	GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o bin/cairn-darwin-arm64 ./cmd/cairn
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/cairn-windows-amd64.exe ./cmd/cairn
