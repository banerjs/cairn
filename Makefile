.PHONY: fmt lint test integration build build-all terraform-fmt terraform-fmt-check terraform-validate terraform-tflint terraform-lint

fmt:
	gofmt -s -w .

lint:
	golangci-lint run ./...

test:
	go test ./...

integration:
	go test -tags=integration -timeout 30m ./internal/integration/...

build: ## Native binary for this machine → ./bin/cairn
	mkdir -p bin
	go build -trimpath -ldflags="-s -w" -o bin/cairn ./cmd/cairn

build-all: ## Cross-compile release binaries into ./bin/
	mkdir -p bin
	GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/cairn-linux-amd64 ./cmd/cairn
	GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o bin/cairn-linux-arm64 ./cmd/cairn
	GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/cairn-darwin-amd64 ./cmd/cairn
	GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o bin/cairn-darwin-arm64 ./cmd/cairn
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/cairn-windows-amd64.exe ./cmd/cairn

terraform-fmt: ## Rewrite infra/terraform with canonical formatting
	cd infra/terraform && terraform fmt -recursive

terraform-fmt-check: ## Fail CI-style if Terraform formatting drifts
	cd infra/terraform && terraform fmt -check -recursive

terraform-validate: ## terraform validate (runs init if needed)
	cd infra/terraform && terraform init -input=false && terraform validate

terraform-tflint: ## Optional: requires tflint on PATH (see infra/terraform/.tflint.hcl)
	cd infra/terraform && tflint --init && tflint

terraform-lint: terraform-fmt-check terraform-validate ## fmt-check + validate (run `make terraform-tflint` if tflint is installed)
