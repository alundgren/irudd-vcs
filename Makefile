DIST := dist
DIST_DIRS := $(DIST)/binaries $(DIST)/release

GO ?= go
SHASUM ?= shasum -a 256
COMMA := ,

XGO_VERSION := go-1.27.x

AIR_PACKAGE ?= github.com/air-verse/air@v1.65.3 # renovate: datasource=go
EDITORCONFIG_CHECKER_PACKAGE ?= github.com/editorconfig-checker/editorconfig-checker/v3/cmd/editorconfig-checker@v3.7.0 # renovate: datasource=go
GOLANGCI_LINT_PACKAGE ?= github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 # renovate: datasource=go
GXZ_PACKAGE ?= github.com/ulikunitz/xz/cmd/gxz@v0.5.15 # renovate: datasource=go
MISSPELL_PACKAGE ?= github.com/golangci/misspell/cmd/misspell@v0.8.0 # renovate: datasource=go
SWAGGER_PACKAGE ?= github.com/go-swagger/go-swagger/cmd/swagger@v0.34.1 # renovate: datasource=go
XGO_PACKAGE ?= src.techknowlogick.com/xgo@v1.9.0 # renovate: datasource=go
GOVULNCHECK_PACKAGE ?= golang.org/x/vuln/cmd/govulncheck@v1.4.0 # renovate: datasource=go
SHELLCHECK_IMAGE ?= docker.io/koalaman/shellcheck:v0.11.0@sha256:61862eba1fcf09a484ebcc6feea46f1782532571a34ed51fedf90dd25f925a8d # renovate: datasource=docker

CONTAINER_RUNTIME ?= $(shell hash docker >/dev/null 2>&1 && echo docker || echo podman)

HAS_GO := $(shell hash $(GO) > /dev/null 2>&1 && echo yes)
ifeq ($(HAS_GO), yes)
	CGO_EXTRA_CFLAGS := -DSQLITE_MAX_VARIABLE_NUMBER=32766
	CGO_CFLAGS ?= $(shell $(GO) env CGO_CFLAGS) $(CGO_EXTRA_CFLAGS)
endif

MAKE_EVIDENCE_DIR := .make_evidence

# Use sqlite as default database if running tests, only do so for local tests, not in CI.
# CI should explicitly set the database to avoid unexpected results.
ifneq ($(findstring test-,$(MAKECMDGOALS)),)
	ifeq ($(CI),)
		GITEA_TEST_DATABASE ?= sqlite
	endif
endif

TAGS ?=
TAGS_EVIDENCE := $(MAKE_EVIDENCE_DIR)/tags

CGO_ENABLED ?= 0
ifneq (,$(findstring sqlite_mattn,$(TAGS)))
	CGO_ENABLED = 1
endif

STATIC ?=
EXTLDFLAGS ?=
ifneq ($(STATIC),)
	EXTLDFLAGS = -extldflags "-static"
endif

ifeq ($(GOOS),windows)
	IS_WINDOWS := yes
else ifeq ($(patsubst Windows%,Windows,$(OS)),Windows)
	ifeq ($(GOOS),)
		IS_WINDOWS := yes
	endif
endif

# GOFLAGS and EXTRA_GOFLAGS are for the 'go build' command only
ifeq ($(IS_WINDOWS),yes)
	GOFLAGS := -v -buildmode=exe
	EXECUTABLE ?= gitea.exe
else
	GOFLAGS := -v
	EXECUTABLE ?= gitea
endif
EXTRA_GOFLAGS ?=

ifeq ($(shell sed --version 2>/dev/null | grep -q GNU && echo gnu),gnu)
	SED_INPLACE := sed -i
else
	SED_INPLACE := sed -i ''
endif

# GOTEST_FLAGS is for unit test and integration test
GOTEST_FLAGS ?= -timeout 40m

STORED_VERSION_FILE := VERSION

GITHUB_REF_TYPE ?= branch
GITHUB_REF_NAME ?= $(shell git rev-parse --abbrev-ref HEAD)

ifneq ($(GITHUB_REF_TYPE),branch)
	VERSION ?= $(subst v,,$(GITHUB_REF_NAME))
	GITEA_VERSION ?= $(VERSION)
else
	ifneq ($(GITHUB_REF_NAME),)
		VERSION ?= $(subst release/v,,$(GITHUB_REF_NAME))-nightly
	else
		VERSION ?= main
	endif

	STORED_VERSION=$(shell cat $(STORED_VERSION_FILE) 2>/dev/null)
	ifneq ($(STORED_VERSION),)
		GITEA_VERSION ?= $(STORED_VERSION)
	else
		GITEA_VERSION ?= $(shell git describe --tags --always | sed 's/-/+/' | sed 's/^v//')
	endif
endif

# if version = "main" then update version to "nightly"
ifeq ($(VERSION),main)
	VERSION := main-nightly
endif

LDFLAGS := $(LDFLAGS) -X "main.Version=$(GITEA_VERSION)" -X "main.Tags=$(TAGS)"

LINUX_ARCHS ?= linux/amd64,linux/386,linux/arm-5,linux/arm-6,linux/arm64,linux/riscv64

GO_TEST_PACKAGES ?= $(filter-out $(shell $(GO) list gitea.dev/models/migrations/...) gitea.dev/tests gitea.dev/tests/integration,$(shell $(GO) list ./... | grep -v /vendor/))
MIGRATE_TEST_PACKAGES ?= $(shell $(GO) list gitea.dev/models/migrations/...)

BINDATA_DEST_WILDCARD := modules/options/bindata.*

AIR_TMP_DIR := .air

GO_LICENSE_FILE := assets/go-licenses.json

TAR_EXCLUDES := .git data indexers queues log $(EXECUTABLE) $(DIST) $(MAKE_EVIDENCE_DIR) $(AIR_TMP_DIR)

GO_DIRS := build cmd models modules routers services tests tools

SPELLCHECK_FILES := $(GO_DIRS) docs $(wildcard *.go *.md *.yml *.yaml *.toml)
EDITORCONFIG_FILES := custom/conf/app.example.ini modules/apispec tools .air.toml

GO_SOURCES := $(wildcard *.go)
GO_SOURCES += $(shell find $(GO_DIRS) -type f -name "*.go")

SWAGGER_SPEC := modules/apispec/v1_json.tmpl
SWAGGER_SPEC_INPUT := modules/apispec/v1_input.json
SWAGGER_EXCLUDE := gitea.dev/sdk
OPENAPI3_SPEC := modules/apispec/v1_openapi3_json.tmpl

# Include local Makefile
# Makefile.local is listed in .gitignore
ifneq ("$(wildcard Makefile.local)","")
	include Makefile.local
endif

$(foreach v, $(filter TEST_%, $(.VARIABLES)), $(eval MAKEFILE_VARS+=$v=$($v)))
$(foreach v, $(filter GITEA_TEST_%, $(.VARIABLES)), $(eval MAKEFILE_VARS+=$v=$($v)))
export MAKEFILE_VARS

.PHONY: all
all: build

.PHONY: help
help: Makefile ## print Makefile help information.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m[TARGETS] default target: build\033[0m\n\n\033[35mTargets:\033[0m\n"} /^[0-9A-Za-z._-]+:.*?##/ { printf "  \033[36m%-45s\033[0m %s\n", $$1, $$2 }' Makefile #$(MAKEFILE_LIST)
	@printf "  \033[36m%-46s\033[0m %s\n" "test-backend[#TestSpecificName]" "run unit test (sqlite only)"
	@printf "  \033[36m%-46s\033[0m %s\n" "test-integration[#TestSpecificName]" "run integration test (sqlite only)"

.PHONY: clean
clean: ## delete backend and integration files
	rm -f $(EXECUTABLE) test-*.test tests/*.ini
	rm -rf  $(DIST) $(BINDATA_DEST_WILDCARD) man tests/integration/gitea-integration-*

.PHONY: fmt
fmt: ## format the Go code
	$(GO) run $(GOLANGCI_LINT_PACKAGE) fmt

.PHONY: fmt-check
fmt-check: fmt
	@diff=$$(git diff --color=always $(GO_SOURCES)); \
	if [ -n "$$diff" ]; then \
	  echo "Please run 'make fmt' and commit the result:"; \
	  printf "%s" "$${diff}"; \
	  exit 1; \
	fi

.PHONY: $(TAGS_EVIDENCE)
$(TAGS_EVIDENCE):
	@mkdir -p $(MAKE_EVIDENCE_DIR)
	@echo "$(TAGS)" > $(TAGS_EVIDENCE)

ifneq "$(TAGS)" "$(shell cat $(TAGS_EVIDENCE) 2>/dev/null)"
TAGS_PREREQ := $(TAGS_EVIDENCE)
endif

.PHONY: generate-swagger
generate-swagger: $(SWAGGER_SPEC) $(OPENAPI3_SPEC) ## generate the Swagger 2.0 and OpenAPI 3.0 specs from code comments

$(SWAGGER_SPEC): $(GO_SOURCES) $(SWAGGER_SPEC_INPUT)
	$(GO) run $(SWAGGER_PACKAGE) generate spec --exclude "$(SWAGGER_EXCLUDE)" --input "$(SWAGGER_SPEC_INPUT)" --output './$(SWAGGER_SPEC)'

.PHONY: swagger-check
swagger-check: generate-swagger
	@diff=$$(git diff --color=always '$(SWAGGER_SPEC)'); \
	if [ -n "$$diff" ]; then \
		echo "Please run 'make generate-swagger' and commit the result:"; \
		printf "%s" "$${diff}"; \
		exit 1; \
	fi

.PHONY: swagger-validate
swagger-validate: ## check if the swagger spec is valid
	@# swagger "validate" requires that the "basePath" must start with a slash, but we are using Golang template "{{...}}"
	@$(SED_INPLACE) -E -e 's|"basePath":( *)"(.*)"|"basePath":\1"/\2"|g' './$(SWAGGER_SPEC)' # add a prefix slash to basePath
	@# FIXME: there are some warnings
	$(GO) run $(SWAGGER_PACKAGE) validate './$(SWAGGER_SPEC)'
	@$(SED_INPLACE) -E -e 's|"basePath":( *)"/(.*)"|"basePath":\1"\2"|g' './$(SWAGGER_SPEC)' # remove the prefix slash from basePath

.PHONY: generate-openapi3
generate-openapi3: $(OPENAPI3_SPEC) ## generate the OpenAPI 3.0 spec from the Swagger 2.0 spec

$(OPENAPI3_SPEC): $(SWAGGER_SPEC) build/generate-openapi.go $(wildcard build/openapi3gen/*.go)
	$(GO) run build/generate-openapi.go

.PHONY: openapi3-check
openapi3-check: generate-openapi3
	@diff=$$(git diff --color=always '$(OPENAPI3_SPEC)'); \
	if [ -n "$$diff" ]; then \
		echo "Please run 'make generate-openapi3' and commit the result:"; \
		printf "%s" "$${diff}"; \
		exit 1; \
	fi

.PHONY: checks
checks: checks-backend ## run various consistency checks

.PHONY: checks-backend
checks-backend: tidy-check swagger-check openapi3-check fmt-check swagger-validate security-check ## check backend files

.PHONY: lint
lint: lint-backend lint-spell lint-shell ## lint everything

.PHONY: lint-fix
lint-fix: lint-backend-fix lint-spell-fix ## lint everything and fix issues

.PHONY: lint-backend
lint-backend: lint-go lint-editorconfig ## lint backend files

.PHONY: lint-backend-fix
lint-backend-fix: lint-go-fix lint-editorconfig ## lint backend files and fix issues

.PHONY: lint-spell
lint-spell: ## lint spelling
	@git ls-files $(SPELLCHECK_FILES) | xargs go run $(MISSPELL_PACKAGE) -dict assets/misspellings.csv -error

.PHONY: lint-spell-fix
lint-spell-fix: ## lint spelling and fix issues
	@git ls-files $(SPELLCHECK_FILES) | xargs go run $(MISSPELL_PACKAGE) -dict assets/misspellings.csv -w

.PHONY: lint-go
lint-go: ## lint go files
	GO=$(GO) GOLANGCI_LINT_PACKAGE=$(GOLANGCI_LINT_PACKAGE) $(GO) run ./tools/lint-go-all.go

.PHONY: lint-go-fix
lint-go-fix: ## lint go files and fix issues
	GO=$(GO) GOLANGCI_LINT_PACKAGE=$(GOLANGCI_LINT_PACKAGE) $(GO) run ./tools/lint-go-all.go --fix

.PHONY: lint-editorconfig
lint-editorconfig: ## lint files with editorconfig rules
	@echo "Running editorconfig check..."
	@$(GO) run $(EDITORCONFIG_CHECKER_PACKAGE) $(EDITORCONFIG_FILES)

.PHONY: lint-shell
lint-shell: ## lint shell scripts
	@SHELLCHECK_IMAGE=$(SHELLCHECK_IMAGE) CONTAINER_RUNTIME=$(CONTAINER_RUNTIME) ./tools/lint-shell.sh $$(git ls-files '*.sh')

.PHONY: watch-backend
watch-backend: ## watch backend files and continuously rebuild
	GITEA_RUN_MODE=dev $(GO) run $(AIR_PACKAGE) -c .air.toml

.PHONY: test-backend
test-backend: ## test backend files
	@echo "Running go test with $(GOTEST_FLAGS) -tags '$(TAGS)'..."
	@$(GO) test $(GOTEST_FLAGS) -tags='$(TAGS)' $(GO_TEST_PACKAGES)

.PHONY: test-check
test-check:
	@echo "Running test-check...";
	@diff=$$(git status -s); \
	if [ -n "$$diff" ]; then \
		echo "make test-backend has changed files in the source tree:"; \
		printf "%s" "$${diff}"; \
		echo "You should change the tests to create these files in a temporary directory."; \
		echo "Do not simply add these files to .gitignore"; \
		exit 1; \
	fi

.PHONY: test-backend\#%
test-backend\#%:
	@echo "Running go test with -tags '$(TAGS)'..."
	@$(GO) test $(GOTEST_FLAGS) -tags='$(TAGS)' -run $(subst .,/,$*) $(GO_TEST_PACKAGES)

.PHONY: coverage
coverage:
	grep '^\(mode: .*\)\|\(.*:[0-9]\+\.[0-9]\+,[0-9]\+\.[0-9]\+ [0-9]\+ [0-9]\+\)$$' coverage.out > coverage-bodged.out
	grep '^\(mode: .*\)\|\(.*:[0-9]\+\.[0-9]\+,[0-9]\+\.[0-9]\+ [0-9]\+ [0-9]\+\)$$' integration.coverage.out > integration.coverage-bodged.out
	$(GO) run tools/gocovmerge.go integration.coverage-bodged.out coverage-bodged.out > coverage.all

.PHONY: unit-test-coverage
unit-test-coverage:
	@echo "Running unit-test-coverage $(GOTEST_FLAGS) -tags '$(TAGS)'..."
	@$(GO) test $(GOTEST_FLAGS) -tags='$(TAGS)' -cover -coverprofile coverage.out $(GO_TEST_PACKAGES) && echo "\n==>\033[32m Ok\033[m\n" || exit 1

.PHONY: tidy
tidy: ## run go mod tidy
	$(eval MIN_GO_VERSION := $(shell grep -Eo '^go\s+[0-9]+\.[0-9.]+' go.mod | cut -d' ' -f2))
	$(GO) mod tidy -compat=$(MIN_GO_VERSION)
	@$(MAKE) --no-print-directory $(GO_LICENSE_FILE)

vendor: go.mod go.sum
	$(GO) mod vendor
	@touch vendor

.PHONY: tidy-check
tidy-check: tidy
	@diff=$$(git diff --color=always go.mod go.sum $(GO_LICENSE_FILE)); \
	if [ -n "$$diff" ]; then \
		echo "Please run 'make tidy' and commit the result:"; \
		printf "%s" "$${diff}"; \
		exit 1; \
	fi

.PHONY: go-licenses
go-licenses: $(GO_LICENSE_FILE) ## regenerate go licenses

$(GO_LICENSE_FILE): go.mod go.sum
	GO=$(GO) $(GO) run build/generate-go-licenses.go $(GO_LICENSE_FILE)

.PHONY: test-integration
test-integration:
	@# Use a compiled binary: testlogger forwards gitea logs to t.Log, so `go test -v`
	@# would flood output per passing test. testcache can't help these tests anyway —
	@# they mutate the work directory, so cache inputs change between runs.
	$(GO) test $(GOTEST_FLAGS) -tags '$(TAGS)' -c gitea.dev/tests/integration -o ./test-integration-$(GITEA_TEST_DATABASE).test
	./tools/test-integration.sh ./test-integration-$(GITEA_TEST_DATABASE).test

.PHONY: test-integration-compile
test-integration-compile:
	$(GO) test $(GOTEST_FLAGS) -tags '$(TAGS)' -c -o /dev/null gitea.dev/tests/integration

.PHONY: test-integration\#%
test-integration\#%:
	$(GO) test $(GOTEST_FLAGS) -tags '$(TAGS)' -run $(subst .,/,$*) gitea.dev/tests/integration

.PHONY: test-migration
test-migration: migrations.individual.test

.PHONY: migrations.individual.test
migrations.individual.test:
	@# tests of multiple packages use the same database, don't run in parallel
	$(GO) test $(GOTEST_FLAGS) -tags '$(TAGS)' -p 1 $(MIGRATE_TEST_PACKAGES)

.PHONY: migrations.individual.test\#%
migrations.individual.test\#%:
	$(GO) test $(GOTEST_FLAGS) -tags '$(TAGS)' gitea.dev/models/migrations/$*

.PHONY: build
build: backend ## build the server

.PHONY: backend
backend: generate-backend $(EXECUTABLE) ## build the server binary

.PHONY: generate
generate: generate-backend ## run "go generate"

.PHONY: generate-backend
generate-backend: $(TAGS_PREREQ) generate-go

.PHONY: generate-go
generate-go: $(TAGS_PREREQ)
	@echo "Running go generate..."
	@CC= GOOS= GOARCH= CGO_ENABLED=0 $(GO) generate -tags '$(TAGS)' ./...

.PHONY: security-check
security-check:
	GOEXPERIMENT= go run $(GOVULNCHECK_PACKAGE) -show color ./... || true

$(EXECUTABLE): $(GO_SOURCES) $(TAGS_PREREQ)
	CGO_ENABLED="$(CGO_ENABLED)" CGO_CFLAGS="$(CGO_CFLAGS)" $(GO) build $(GOFLAGS) $(EXTRA_GOFLAGS) -tags '$(TAGS)' -ldflags '-s -w $(EXTLDFLAGS) $(LDFLAGS)' -o $@

.PHONY: release
release: generate release-windows release-linux release-darwin release-freebsd release-copy release-compress vendor release-sources release-check

$(DIST_DIRS):
	mkdir -p $(DIST_DIRS)

.PHONY: release-windows
release-windows: | $(DIST_DIRS)
	CGO_CFLAGS="$(CGO_CFLAGS)" $(GO) run $(XGO_PACKAGE) -go $(XGO_VERSION) -buildmode exe -dest $(DIST)/binaries -tags 'osusergo $(TAGS)' -ldflags '-s -w -linkmode external -extldflags "-static" $(LDFLAGS)' -targets 'windows/*' -out gitea-$(VERSION) .
ifeq (,$(findstring gogit,$(TAGS)))
	CGO_CFLAGS="$(CGO_CFLAGS)" $(GO) run $(XGO_PACKAGE) -go $(XGO_VERSION) -buildmode exe -dest $(DIST)/binaries -tags 'osusergo gogit $(TAGS)' -ldflags '-s -w -linkmode external -extldflags "-static" $(LDFLAGS)' -targets 'windows/*' -out gitea-$(VERSION)-gogit .
endif

.PHONY: release-linux
release-linux: | $(DIST_DIRS)
	CGO_CFLAGS="$(CGO_CFLAGS)" $(GO) run $(XGO_PACKAGE) -go $(XGO_VERSION) -dest $(DIST)/binaries -tags 'netgo osusergo $(TAGS)' -ldflags '-s -w -linkmode external -extldflags "-static" $(LDFLAGS)' -targets '$(LINUX_ARCHS)' -out gitea-$(VERSION) .

.PHONY: release-darwin
release-darwin: | $(DIST_DIRS)
	CGO_CFLAGS="$(CGO_CFLAGS)" $(GO) run $(XGO_PACKAGE) -go $(XGO_VERSION) -dest $(DIST)/binaries -tags 'netgo osusergo $(TAGS)' -ldflags '-s -w $(LDFLAGS)' -targets 'darwin-10.12/amd64,darwin-10.12/arm64' -out gitea-$(VERSION) .

.PHONY: release-freebsd
release-freebsd: | $(DIST_DIRS)
	CGO_CFLAGS="$(CGO_CFLAGS)" $(GO) run $(XGO_PACKAGE) -go $(XGO_VERSION) -dest $(DIST)/binaries -tags 'netgo osusergo $(TAGS)' -ldflags '-s -w $(LDFLAGS)' -targets 'freebsd/amd64' -out gitea-$(VERSION) .

.PHONY: release-copy
release-copy: | $(DIST_DIRS)
	cd $(DIST); for file in `find . -type f -name "*"`; do cp $${file} ./release/; done;

.PHONY: release-check
release-check: | $(DIST_DIRS)
	cd $(DIST)/release/; for file in `find . -type f -name "*"`; do echo "checksumming $${file}" && $(SHASUM) `echo $${file} | sed 's/^..//'` > $${file}.sha256; done;

.PHONY: release-compress
release-compress: | $(DIST_DIRS)
	cd $(DIST)/release/; for file in `find . -type f -name "*"`; do echo "compressing $${file}" && $(GO) run $(GXZ_PACKAGE) -k -9 $${file}; done;

.PHONY: release-sources
release-sources: | $(DIST_DIRS)
	echo $(VERSION) > $(STORED_VERSION_FILE)
# bsdtar needs a ^ to prevent matching subdirectories
	$(eval EXCL := --exclude=$(shell tar --help | grep -q bsdtar && echo "^")./)
# use transform to a add a release-folder prefix; in bsdtar the transform parameter equivalent is -s
	$(eval TRANSFORM := $(shell tar --help | grep -q bsdtar && echo "-s '/^./gitea-src-$(VERSION)/'" || echo "--transform 's|^./|gitea-src-$(VERSION)/|'"))
	tar $(addprefix $(EXCL),$(TAR_EXCLUDES)) $(TRANSFORM) -czf $(DIST)/release/gitea-src-$(VERSION).tar.gz .
	rm -f $(STORED_VERSION_FILE)

.PHONY: deps
deps: deps-backend deps-tools ## install dependencies

.PHONY: deps-backend
deps-backend: ## install backend dependencies
	$(GO) mod download

.PHONY: deps-tools
deps-tools: ## install tool dependencies
	$(GO) install $(AIR_PACKAGE) & \
	$(GO) install $(EDITORCONFIG_CHECKER_PACKAGE) & \
	$(GO) install $(GOLANGCI_LINT_PACKAGE) & \
	$(GO) install $(GXZ_PACKAGE) & \
	$(GO) install $(MISSPELL_PACKAGE) & \
	$(GO) install $(SWAGGER_PACKAGE) & \
	$(GO) install $(XGO_PACKAGE) & \
	$(GO) install $(GOVULNCHECK_PACKAGE) & \
	wait

.PHONY: update
update: update-go ## update dependencies

.PHONY: update-go
update-go: ## update go dependencies
	$(GO) get -u ./...
	$(MAKE) tidy

.PHONY: generate-gitignore
generate-gitignore: ## update gitignore files
	$(GO) run build/generate-gitignores.go

# Disable parallel execution because some targets don't specify exact dependencies
.NOTPARALLEL:
