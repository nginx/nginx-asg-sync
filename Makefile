.DEFAULT_GOAL := build-goreleaser

.PHONY: test
test:
	go test ./...

.PHONY: lint
lint:
	docker run --pull always --rm -v $(shell pwd):/nginx-asg-sync -w /nginx-asg-sync -v $(shell go env GOCACHE):/cache/go -e GOCACHE=/cache/go -e GOLANGCI_LINT_CACHE=/cache/go -v $(shell go env GOPATH)/pkg:/go/pkg golangci/golangci-lint:latest golangci-lint --color always run

nginx-asg-sync:
	@go version || (code=$$?; printf "\033[0;31mError\033[0m: unable to build locally, try using the parameter TARGET=container or TARGET=download\n"; exit $$code)
	CGO_ENABLED=0 GOFLAGS="-gcflags=-trimpath=$(shell go env GOPATH) -asmflags=-trimpath=$(shell go env GOPATH)" GOOS=linux go build -trimpath -ldflags "-s -w" -o nginx-asg-sync github.com/nginxinc/nginx-asg-sync/cmd/sync

.PHONY: build-goreleaser
build-goreleaser:
	@goreleaser -v || (code=$$?; printf "\033[0;31mError\033[0m: there was a problem with GoReleaser. Follow the docs to install it https://goreleaser.com/install\n"; exit $$code)
	@GOPATH=$(shell go env GOPATH) goreleaser release --rm-dist --snapshot

.PHONY: build-goreleaser-docker
build-goreleaser-docker:
	@docker -v || (code=$$?; printf "\033[0;31mError\033[0m: there was a problem with Docker\n"; exit $$code)
	@docker run --rm --privileged -v $(PWD):/go/src/github.com/nginxinc/nginx-asg-sync -v /var/run/docker.sock:/var/run/docker.sock -w /go/src/github.com/nginxinc/nginx-asg-sync goreleaser/goreleaser release --snapshot --rm-dist

.PHONY: clean
clean:
	-rm -r build_output
	-rm nginx-asg-sync

.PHONY: deps
deps: go.mod go.sum
	@go mod tidy && go mod verify && go mod download

LICENSES: go.mod go.sum
	go run github.com/google/go-licenses@latest csv ./... > $@

.PHONY: clean-cache
clean-cache:
	@go clean -modcache
