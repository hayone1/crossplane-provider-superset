# ====================================================================================
# Setup Project

PROJECT_NAME ?= provider-superset
PROJECT_REPO ?= github.com/hayone1/crossplane-provider-superset

export TERRAFORM_VERSION ?= 1.5.7

# Do not allow a version of terraform greater than 1.5.x, due to versions 1.6+ being
# licensed under BSL, which is not permitted.
TERRAFORM_VERSION_VALID := $(shell [ "$(TERRAFORM_VERSION)" = "`printf "$(TERRAFORM_VERSION)\n1.6" | sort -V | head -n1`" ] && echo 1 || echo 0)

export TERRAFORM_PROVIDER_SOURCE ?= platacard/superset
export TERRAFORM_PROVIDER_REPO ?= https://github.com/platacard/terraform-provider-superset
# The Go module of the Terraform provider. The provider compiles it in ("no-fork"), so the
# build that go.mod resolves, including its replace directive, is what ships. The schema
# (config/schema.json) and the documentation that the CRDs are generated from come from
# that same build, not from the Terraform registry, so that the CRDs always match it.
TERRAFORM_PROVIDER_MODULE ?= github.com/platacard/terraform-provider-superset
# The release that the embedded build is based on. Terraform needs a version to load the
# provider when it writes the schema.
export TERRAFORM_PROVIDER_VERSION ?= 1.0.0
export TERRAFORM_PROVIDER_DOWNLOAD_NAME ?= terraform-provider-superset
export TERRAFORM_NATIVE_PROVIDER_BINARY ?= terraform-provider-superset_v$(TERRAFORM_PROVIDER_VERSION)
export TERRAFORM_DOCS_PATH ?= docs/resources

PLATFORMS ?= linux_amd64 linux_arm64

# -include will silently skip missing files, which allows us
# to load those files with a target in the Makefile. If only
# "include" was used, the make command would fail and refuse
# to run a target until the include commands succeeded.
-include build/makelib/common.mk

# ====================================================================================
# Setup Output

-include build/makelib/output.mk

# ====================================================================================
# Setup Go

# Set a sane default so that the nprocs calculation below is less noisy on the initial
# loading of this file
NPROCS ?= 1

# each of our test suites starts a kube-apiserver and running many test suites in
# parallel can lead to high CPU utilization. by default we reduce the parallelism
# to half the number of CPU cores.
GO_TEST_PARALLEL := $(shell echo $$(( $(NPROCS) / 2 )))

GO_REQUIRED_VERSION ?= 1.26
GOLANGCILINT_VERSION ?= 2.13.0
GO_STATIC_PACKAGES = $(GO_PROJECT)/cmd/provider $(GO_PROJECT)/cmd/generator
GO_LDFLAGS += -X $(GO_PROJECT)/internal/version.Version=$(VERSION)
GO_SUBDIRS += cmd internal apis config
-include build/makelib/golang.mk

# ====================================================================================
# Setup Kubernetes tools

KIND_VERSION = v0.33.0
UPTEST_VERSION = v2.2.0
CRDDIFF_VERSION = v0.12.1
# The Crossplane CLI has its own versions. releases.crossplane.io serves v2.2.1, and the CLI
# only extracts packages here, so it does not need to match the Crossplane version below.
CROSSPLANE_CLI_VERSION = v2.2.1
# for e2e testing
CROSSPLANE_VERSION = 2.4.3
-include build/makelib/k8s_tools.mk

# ====================================================================================
# Setup Images

REGISTRY_ORGS ?= ghcr.io/hayone1
IMAGES = $(PROJECT_NAME)
-include build/makelib/imagelight.mk

# ====================================================================================
# Setup XPKG

XPKG_REG_ORGS ?= ghcr.io/hayone1
# NOTE(hasheddan): skip promoting on xpkg.crossplane.io as channel tags are
# inferred.
XPKG_REG_ORGS_NO_PROMOTE ?= ghcr.io/hayone1
XPKGS = $(PROJECT_NAME)
-include build/makelib/xpkg.mk

# ====================================================================================
# Fallthrough

# run `make help` to see the targets and options

# We want submodules to be set up the first time `make` is run.
# We manage the build/ folder and its Makefiles as a submodule.
# The first time `make` is run, the includes of build/*.mk files will
# all fail, and this target will be run. The next time, the default as defined
# by the includes will be run instead.
fallthrough: submodules
	@echo Initial setup complete. Running make again . . .
	@make

# NOTE(hasheddan): we force image building to happen prior to xpkg build so that
# we ensure image is present in daemon.
xpkg.build.provider-superset: do.build.images

# NOTE(hasheddan): we ensure up is installed prior to running platform-specific
# build steps in parallel to avoid encountering an installation race condition.
build.init: $(UP) $(CROSSPLANE_CLI) check-terraform-version

# ====================================================================================
# Setup Terraform for fetching provider schema
TERRAFORM := $(TOOLS_HOST_DIR)/terraform-$(TERRAFORM_VERSION)
TERRAFORM_WORKDIR := $(WORK_DIR)/terraform
TERRAFORM_PROVIDER_SCHEMA := config/schema.json

check-terraform-version:
ifneq ($(TERRAFORM_VERSION_VALID),1)
	$(error invalid TERRAFORM_VERSION $(TERRAFORM_VERSION), must be less than 1.6.0 since that version introduced a not permitted BSL license))
endif

$(TERRAFORM): check-terraform-version
	@$(INFO) installing terraform $(HOSTOS)-$(HOSTARCH)
	@mkdir -p $(TOOLS_HOST_DIR)/tmp-terraform
	@curl -fsSL https://releases.hashicorp.com/terraform/$(TERRAFORM_VERSION)/terraform_$(TERRAFORM_VERSION)_$(SAFEHOST_PLATFORM).zip -o $(TOOLS_HOST_DIR)/tmp-terraform/terraform.zip
	@unzip $(TOOLS_HOST_DIR)/tmp-terraform/terraform.zip -d $(TOOLS_HOST_DIR)/tmp-terraform
	@mv $(TOOLS_HOST_DIR)/tmp-terraform/terraform $(TERRAFORM)
	@rm -fr $(TOOLS_HOST_DIR)/tmp-terraform
	@$(OK) installing terraform $(HOSTOS)-$(HOSTARCH)

TERRAFORM_PROVIDER_MIRROR := $(TERRAFORM_WORKDIR)/mirror
TERRAFORM_PROVIDER_MIRROR_DIR := $(TERRAFORM_PROVIDER_MIRROR)/registry.terraform.io/$(TERRAFORM_PROVIDER_SOURCE)/$(TERRAFORM_PROVIDER_VERSION)/$(SAFEHOST_PLATFORM)
TERRAFORM_DOCS_WORKDIR := $(WORK_DIR)/$(TERRAFORM_PROVIDER_SOURCE)/$(TERRAFORM_DOCS_PATH)

# The schema of the build that go.mod resolves: the provider binary is built from that
# module and offered to Terraform through a filesystem mirror. The lock file is written
# anew each time, because its checksum is that of the binary.
$(TERRAFORM_PROVIDER_SCHEMA): $(TERRAFORM)
	@$(INFO) generating provider schema for $(TERRAFORM_PROVIDER_MODULE) as resolved by go.mod
	@mkdir -p $(TERRAFORM_PROVIDER_MIRROR_DIR)
	@go build -o $(TERRAFORM_PROVIDER_MIRROR_DIR)/$(TERRAFORM_NATIVE_PROVIDER_BINARY) $(TERRAFORM_PROVIDER_MODULE)
	@printf 'provider_installation {\n  filesystem_mirror {\n    path    = "%s"\n    include = ["registry.terraform.io/%s"]\n  }\n}\n' "$(abspath $(TERRAFORM_PROVIDER_MIRROR))" "$(TERRAFORM_PROVIDER_SOURCE)" > $(TERRAFORM_WORKDIR)/terraformrc
	@echo '{"terraform":[{"required_providers":[{"provider":{"source":"'"$(TERRAFORM_PROVIDER_SOURCE)"'","version":"'"$(TERRAFORM_PROVIDER_VERSION)"'"}}],"required_version":"'"$(TERRAFORM_VERSION)"'"}]}' > $(TERRAFORM_WORKDIR)/main.tf.json
	@rm -f $(TERRAFORM_WORKDIR)/.terraform.lock.hcl
	@TF_CLI_CONFIG_FILE=$(abspath $(TERRAFORM_WORKDIR))/terraformrc $(TERRAFORM) -chdir=$(TERRAFORM_WORKDIR) init -upgrade > $(TERRAFORM_WORKDIR)/terraform-logs.txt 2>&1 || (cat $(TERRAFORM_WORKDIR)/terraform-logs.txt; exit 1)
	@TF_CLI_CONFIG_FILE=$(abspath $(TERRAFORM_WORKDIR))/terraformrc $(TERRAFORM) -chdir=$(TERRAFORM_WORKDIR) providers schema -json=true > $(TERRAFORM_WORKDIR)/schema.json 2>> $(TERRAFORM_WORKDIR)/terraform-logs.txt || (cat $(TERRAFORM_WORKDIR)/terraform-logs.txt; exit 1)
	@mv $(TERRAFORM_WORKDIR)/schema.json $(TERRAFORM_PROVIDER_SCHEMA)
	@$(OK) generating provider schema for $(TERRAFORM_PROVIDER_MODULE) as resolved by go.mod

# The documentation of the build that go.mod resolves, for the scraper.
pull-docs:
	@mkdir -p "$(TERRAFORM_DOCS_WORKDIR)"
	@go mod download $(TERRAFORM_PROVIDER_MODULE)
	@dir="$$(go list -m -f '{{.Dir}}' $(TERRAFORM_PROVIDER_MODULE))"; \
		if [ -z "$$dir" ] || [ ! -d "$$dir/$(TERRAFORM_DOCS_PATH)" ]; then echo "cannot find the docs of $(TERRAFORM_PROVIDER_MODULE)"; exit 1; fi; \
		cp -R "$$dir/$(TERRAFORM_DOCS_PATH)/." "$(TERRAFORM_DOCS_WORKDIR)/"
	@chmod -R u+w "$(TERRAFORM_DOCS_WORKDIR)"

generate.init: $(TERRAFORM_PROVIDER_SCHEMA) pull-docs

.PHONY: $(TERRAFORM_PROVIDER_SCHEMA) pull-docs check-terraform-version
# ====================================================================================
# Targets

# NOTE: the build submodule currently overrides XDG_CACHE_HOME in order to
# force the Helm 3 to use the .work/helm directory. This causes Go on Linux
# machines to use that directory as the build cache as well. We should adjust
# this behavior in the build submodule because it is also causing Linux users
# to duplicate their build cache, but for now we just make it easier to identify
# its location in CI so that we cache between builds.
go.cachedir:
	@go env GOCACHE

go.mod.cachedir:
	@go env GOMODCACHE

# Generate a coverage report for cobertura applying exclusions on
# - generated file
cobertura:
	@cat $(GO_TEST_OUTPUT)/coverage.txt | \
		grep -v zz_ | \
		$(GOCOVER_COBERTURA) > $(GO_TEST_OUTPUT)/cobertura-coverage.xml

# Update the submodules, such as the common build scripts.
submodules:
	@git submodule sync
	@git submodule update --init --recursive

# This is for running out-of-cluster locally, and is for convenience. Running
# this make target will print out the command which was used. For more control,
# try running the binary directly with different arguments.
run: go.build
	@$(INFO) Running Crossplane locally out-of-cluster . . .
	@# To see other arguments that can be provided, run the command with --help instead
	$(GO_OUT_DIR)/provider --debug --no-leader-election

# ====================================================================================
# End to End Testing
CROSSPLANE_NAMESPACE = crossplane-system
-include build/makelib/local.xpkg.mk
-include build/makelib/controlplane.mk

# uptest runs examples against your own Apache Superset. It reads these variables from the
# environment or from .env (see example.env); values in .env win:
# - UPTEST_CLOUD_CREDENTIALS (required): the credentials JSON object that a ProviderConfig
#   Secret holds, for example {"host": "http://superset:8088", "username": "admin", "password": "..."}.
#   cluster/test/setup.sh puts it into the Secret superset-creds.
# - UPTEST_EXAMPLE_LIST (required): a comma-separated list of examples to test.
# - UPTEST_DATASOURCE_PATH (optional), see https://github.com/crossplane/uptest#injecting-dynamic-values-and-datasource
# "make e2e" builds and deploys the provider to kind first; "make uptest" needs "make local-deploy" first.
LOAD_DOTENV = set -a; if [ -f .env ]; then . ./.env; fi; set +a

uptest.check:
	@$(LOAD_DOTENV); \
	if [ -z "$${UPTEST_CLOUD_CREDENTIALS:-}" ]; then echo "UPTEST_CLOUD_CREDENTIALS is not set. Add it to .env (see example.env)."; exit 1; fi; \
	if [ -z "$${UPTEST_EXAMPLE_LIST:-}" ]; then echo "UPTEST_EXAMPLE_LIST is not set, for example: make e2e UPTEST_EXAMPLE_LIST=examples/cluster/security/role.yaml"; exit 1; fi

uptest: uptest.check $(UPTEST) $(KUBECTL) $(CHAINSAW) $(CROSSPLANE_CLI)
	@$(INFO) running automated tests
	@$(LOAD_DOTENV); KUBECTL=$(KUBECTL) CHAINSAW=$(CHAINSAW) CROSSPLANE_CLI=$(CROSSPLANE_CLI) CROSSPLANE_NAMESPACE=$(CROSSPLANE_NAMESPACE) $(UPTEST) e2e "$${UPTEST_EXAMPLE_LIST}" --data-source="$${UPTEST_DATASOURCE_PATH:-}" --setup-script=cluster/test/setup.sh --default-conditions="Test" || $(FAIL)
	@$(OK) running automated tests

local-deploy: build controlplane.up local.xpkg.deploy.provider.$(PROJECT_NAME)
	@$(INFO) running locally built provider
	@$(KUBECTL) wait provider.pkg $(PROJECT_NAME) --for condition=Healthy --timeout 5m
	@$(KUBECTL) -n crossplane-system wait --for=condition=Available deployment --all --timeout=5m
	@$(OK) running locally built provider

e2e: uptest.check local-deploy uptest

# Apache Superset and a PostgreSQL in Docker (test/superset/compose.yaml), for test/e2e/run.sh,
# make e2e and make uptest. Superset joins kind's Docker network as "superset".
SUPERSET_COMPOSE = docker compose -f test/superset/compose.yaml

superset.up:
	@echo "Starting Apache Superset in Docker..."
	@docker network inspect kind >/dev/null 2>&1 || docker network create kind >/dev/null
	@$(SUPERSET_COMPOSE) up -d --quiet-pull --wait --wait-timeout 600 || ($(SUPERSET_COMPOSE) logs --tail 50 superset; exit 1)
	@echo "Apache Superset is up at http://localhost:8088 (http://superset:8088 inside kind), user admin, password admin."

superset.down:
	@$(SUPERSET_COMPOSE) down --volumes

.PHONY: uptest.check uptest local-deploy e2e superset.up superset.down

crddiff: $(UPTEST)
	@$(INFO) Checking breaking CRD schema changes
	@for crd in $${MODIFIED_CRD_LIST}; do \
		if ! git cat-file -e "$${GITHUB_BASE_REF}:$${crd}" 2>/dev/null; then \
			echo "CRD $${crd} does not exist in the $${GITHUB_BASE_REF} branch. Skipping..." ; \
			continue ; \
		fi ; \
		echo "Checking $${crd} for breaking API changes..." ; \
		changes_detected=$$(go run github.com/crossplane/uptest/cmd/crddiff@$(CRDDIFF_VERSION) revision --enable-upjet-extensions <(git cat-file -p "$${GITHUB_BASE_REF}:$${crd}") "$${crd}" 2>&1) ; \
		if [[ $$? != 0 ]] ; then \
			printf "\033[31m"; echo "Breaking change detected!"; printf "\033[0m" ; \
			echo "$${changes_detected}" ; \
			echo ; \
		fi ; \
	done
	@$(OK) Checking breaking CRD schema changes

schema-version-diff:
	@$(INFO) Checking for native state schema version changes
	@export PREV_PROVIDER_VERSION=$$(git cat-file -p "${GITHUB_BASE_REF}:Makefile" | sed -nr 's/^export[[:space:]]*TERRAFORM_PROVIDER_VERSION[[:space:]]*:=[[:space:]]*(.+)/\1/p'); \
	echo Detected previous Terraform provider version: $${PREV_PROVIDER_VERSION}; \
	echo Current Terraform provider version: $${TERRAFORM_PROVIDER_VERSION}; \
	mkdir -p $(WORK_DIR); \
	git cat-file -p "$${GITHUB_BASE_REF}:config/schema.json" > "$(WORK_DIR)/schema.json.$${PREV_PROVIDER_VERSION}"; \
	./scripts/version_diff.py config/generated.lst "$(WORK_DIR)/schema.json.$${PREV_PROVIDER_VERSION}" config/schema.json
	@$(OK) Checking for native state schema version changes

.PHONY: cobertura submodules fallthrough run crds.clean

# ====================================================================================
# Special Targets

define CROSSPLANE_MAKE_HELP
Crossplane Targets:
    cobertura             Generate a coverage report for cobertura applying exclusions on generated files.
    submodules            Update the submodules, such as the common build scripts.
    run                   Run crossplane locally, out-of-cluster. Useful for development.

endef
# The reason CROSSPLANE_MAKE_HELP is used instead of CROSSPLANE_HELP is because the crossplane
# binary will try to use CROSSPLANE_HELP if it is set, and this is for something different.
export CROSSPLANE_MAKE_HELP

crossplane.help:
	@echo "$$CROSSPLANE_MAKE_HELP"

help-special: crossplane.help

.PHONY: crossplane.help help-special

# TODO(negz): Update CI to use these targets.
vendor: modules.download
vendor.check: modules.check
