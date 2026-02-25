.DEFAULT_GOAL := build-goreleaser
# renovate: datasource=github-tags depName=golangci/golangci-lint
GOLANGCI_LINT_VERSION = v2.8.0
# renovate: datasource=docker depName=goreleaser/goreleaser
GORELEASER_VERSION = v2.14.1
# renovate: datasource=go depName=google/go-licenses
GO_LICENSES = v1.6.0

.PHONY: test
test:
	go test -v -shuffle=on -race ./...

.PHONY: lint
lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --fix

nginx-asg-sync:
	@go version || (code=$$?; printf "\033[0;31mError\033[0m: unable to build locally, try using the parameter TARGET=container or TARGET=download\n"; exit $$code)
	CGO_ENABLED=0 GOFLAGS="-gcflags=-trimpath=$(shell go env GOPATH) -asmflags=-trimpath=$(shell go env GOPATH)" GOOS=linux go build -trimpath -ldflags "-s -w -X main.version=devel" -o nginx-asg-sync github.com/nginx/nginx-asg-sync/cmd/sync

.PHONY: build-goreleaser
build-goreleaser:
	@goreleaser -v || (code=$$?; printf "\033[0;31mError\033[0m: there was a problem with GoReleaser. Follow the docs to install it https://goreleaser.com/install\n"; exit $$code)
	@goreleaser release --clean --snapshot

.PHONY: build-goreleaser-docker
build-goreleaser-docker:
	@docker -v || (code=$$?; printf "\033[0;31mError\033[0m: there was a problem with Docker\n"; exit $$code)
	@docker run --pull always --rm --privileged -v $(PWD):/go/src/github.com/nginx/nginx-asg-sync -v /var/run/docker.sock:/var/run/docker.sock -w /go/src/github.com/nginx/nginx-asg-sync goreleaser/goreleaser:$(GORELEASER_VERSION) release --snapshot --clean

.PHONY: clean
clean:
	-rm -r build_output
	-rm nginx-asg-sync

.PHONY: deps
deps: go.mod go.sum
	@go mod tidy && go mod verify && go mod download

LICENSES: go.mod go.sum
	go run github.com/google/go-licenses@$(GO_LICENSES) csv ./... > $@

.PHONY: clean-cache
clean-cache:
	@go clean -modcache
