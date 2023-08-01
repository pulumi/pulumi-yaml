PULUMI_TEST_ORG   ?= $(shell pulumi whoami)
PULUMI_TEST_OWNER ?= ${PULUMI_TEST_ORG}
PULUMI_LIVE_TEST  ?= false
export PULUMI_TEST_ORG
export PULUMI_TEST_OWNER

CONCURRENCY       := 10
SHELL := sh

PLUGIN_VERSION_AWS          := 5.35.0
PLUGIN_VERSION_AWS_NATIVE   := 0.56.0
PLUGIN_VERSION_EKS          := 1.0.1
PLUGIN_VERSION_AWSX         := 1.0.0-beta.5
PLUGIN_VERSION_AZURE_NATIVE := 1.99.1
PLUGIN_VERSION_RANDOM       := 4.12.1
GO                          := go

BUILD_FLAGS ?=

.phony: .EXPORT_ALL_VARIABLES
.EXPORT_ALL_VARIABLES:

default: ensure build

get_plugins::
	pulumi plugin install resource aws ${PLUGIN_VERSION_AWS}
	pulumi plugin install resource random ${PLUGIN_VERSION_RANDOM}
	pulumi plugin install resource aws-native ${PLUGIN_VERSION_AWS_NATIVE}
	# Required for eks:
	pulumi plugin install resource aws 4.15.0
	pulumi plugin install resource eks # version fails
	pulumi plugin install resource azure-native ${PLUGIN_VERSION_AZURE_NATIVE}
	pulumi plugin install resource awsx ${PLUGIN_VERSION_AWSX}

update_plugin_docs::
	./scripts/update_plugin_docs.sh aws ${PLUGIN_VERSION_AWS}
	./scripts/update_plugin_docs.sh random ${PLUGIN_VERSION_RANDOM}
	./scripts/update_plugin_docs.sh aws-native ${PLUGIN_VERSION_AWS_NATIVE}
	./scripts/update_plugin_docs.sh eks ${PLUGIN_VERSION_EKS}
	./scripts/update_plugin_docs.sh azure-native ${PLUGIN_VERSION_AZURE_NATIVE}
	./scripts/update_plugin_docs.sh awsx ${PLUGIN_VERSION_AWSX}

install::
	${GO} install ./cmd/...

clean::
	rm -f ./bin/*
	rm -f pkg/pulumiyaml/testing/test/testdata/{aws,azure-native,azure,kubernetes,random,eks,aws-native,docker}.json

ensure::
	${GO} mod download

.phony: lint
lint:: lint-copyright lint-golang
lint-golang:
	golangci-lint run
lint-copyright:
    # Generated examples don't have the copyright notice.
	pulumictl copyright -x 'pkg/tests/transpiled_examples/**'

build:: ensure
	mkdir -p ./bin
	${GO} build $(BUILD_FLAGS) -o ./bin -p ${CONCURRENCY} ./cmd/...

# Ensure that in tests, the language server is accessible
test:: build get_plugins get_schemas
	PATH="${PWD}/bin:${PATH}" PULUMI_LIVE_TEST="${PULUMI_LIVE_TEST}" \
	  ${GO} test -v --timeout 30m -count 1 -race -parallel ${CONCURRENCY} ./...

# Runs tests with code coverage tracking.
# Two output files are generated in the coverage/ directory:
#
#  unit.out        unit test coverage
#  integration.out integration test coverage
#
# Integration test coverage is only generated for tests that invoke the
# pulumi-language-yaml executable.
.phony: test_cover
test_cover: export GOEXPERIMENT = coverageredesign
test_cover: get_plugins get_schemas
	make build BUILD_FLAGS="-cover -coverpkg=github.com/pulumi/pulumi-yaml/..."
	rm -rf coverage && mkdir -p coverage
	$(eval COVERDIR := $(shell mktemp -d))
	PATH="${PWD}/bin:${PATH}" PULUMI_LIVE_TEST="${PULUMI_LIVE_TEST}" GOCOVERDIR=$(COVERDIR) \
	     ${GO} test -v --timeout 30m -coverprofile=coverage/unit.out -coverpkg=./... ./...
	go tool covdata textfmt -i=$(COVERDIR) -o=coverage/integration.out

test_short::
	${GO} test --timeout 30m -short -count 1 -parallel ${CONCURRENCY} ./...

test_live:: PULUMI_LIVE_TEST = true
test_live:: test_live_prereq test

test_live_prereq::
ifndef AWS_SECRET_ACCESS_KEY
	@	if ! ( aws sts get-caller-identity >/dev/null ); then echo "AWS credentials are required to run live tests"; exit 1; fi
endif

.phony: test_gen
# We don't include other.json because it is not a real schema
test_gen: get_schemas
	${GO} test --run=TestGenerate ./...

get_schemas:
	./scripts/get_schemas.sh

# assuming both repos follow gopath conventions, copy *.pp files into testdata
PULUMI_DIR := ../pulumi
get_testdata:
	rsync -avm --exclude='transpiled_examples' --include='*.pp' --include='*/' --exclude='*' --exclude='.*' \
		'${PULUMI_DIR}/pkg/codegen/testing/test/testdata/' \
		./pkg/pulumiyaml/testing/test/testdata/
