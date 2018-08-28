SHELL=/bin/bash
PROJECT_NAME := "infrastructure-controller"
IMAGE_TAG ?= "latest"
PKG := "github.com/containership/$(PROJECT_NAME)"
PKG_LIST := $(shell glide novendor)
GO_FILES := $(shell find . -type f -not -path './vendor/*' -name '*.go')

# TODO remove this hack. We need Jenkins Dockerfiles until GKE supports a
# version of Docker that supports multi-stage builds.
ifeq ($(JENKINS), 1)
	DOCKERFILE=Dockerfile.jenkins
else
	DOCKERFILE=Dockerfile
endif

.PHONY: all build deploy fmt-check lint test vet release

all: build deploy ## (default) Build and deploy

fmt-check: ## Check the file format
	@gofmt -s -e -d ${GO_FILES}

lint: ## Lint the files
	@golint -set_exit_status ${PKG_LIST}

test: ## Run unittests
	@go test -short ${PKG_LIST}

vet: ## Vet the files
	@go vet ${PKG_LIST}

## Read about data race https://golang.org/doc/articles/race_detector.html
## to not test file for race use `// +build !race` at top
race: ## Run data race detector
	@go test -race -short ${PKG_LIST}

msan: ## Run memory sanitizer (only works on linux/amd64)
	@go test -msan -short ${PKG_LIST}

help: ## Display this help screen
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

### Commands for local development
deploy: ## Deploy the controller
	kubectl apply -f deploy/development/infrastructure-controller.yaml

undeploy: ## Delete the controller
	kubectl delete --now -f deploy/development/infrastructure-controller.yaml

build: ## Build the controller in Docker
	@docker image build -t containership/$(PROJECT_NAME):$(IMAGE_TAG) \
		--build-arg GIT_DESCRIBE=`git describe --dirty` \
		--build-arg GIT_COMMIT=`git rev-parse --short HEAD` \
		-f $(DOCKERFILE) .

release: ## Build release image for controller (must be on semver tag)
	@./hack/build-release.sh
