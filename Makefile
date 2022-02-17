PULUMI_TEST_ORG   ?= $(shell pulumi whoami)
PULUMI_TEST_OWNER ?= ${PULUMI_TEST_ORG}
PULUMI_LIVE_TEST  ?= false
export PULUMI_TEST_ORG
export PULUMI_TEST_OWNER

CONCURRENCY       := 10
SHELL := sh

PLUGIN_VERSION_AWS          := 4.37.3
PLUGIN_VERSION_AWS_NATIVE   := 0.11.0
PLUGIN_VERSION_AZURE_NATIVE := 1.56.0
PLUGIN_VERSION_RANDOM       := 4.3.1

.phony: .EXPORT_ALL_VARIABLES
.EXPORT_ALL_VARIABLES:

get_plugins::
	pulumi plugin install resource aws ${PLUGIN_VERSION_AWS}
	pulumi plugin install resource random ${PLUGIN_VERSION_RANDOM}
	pulumi plugin install resource aws-native ${PLUGIN_VERSION_AWS_NATIVE}
	pulumi plugin install resource azure-native ${PLUGIN_VERSION_AZURE_NATIVE}

update_plugin_docs::
	./scripts/update_plugin_docs.sh aws ${PLUGIN_VERSION_AWS}
	./scripts/update_plugin_docs.sh random ${PLUGIN_VERSION_RANDOM}
	./scripts/update_plugin_docs.sh aws-native ${PLUGIN_VERSION_AWS_NATIVE}
	./scripts/update_plugin_docs.sh azure-native ${PLUGIN_VERSION_AZURE_NATIVE}

install::
	go install ./cmd/...

clean::
	rm -f ./bin/*
	rm -f pkg/pulumiyaml/testing/test/testdata/{aws,azure-native,azure,kubernetes,random}.json

ensure::
	go mod download

.phony: lint
lint::
	golangci-lint run
	pulumictl copyright

build:: ensure
	mkdir -p ./bin
	go build -o ./bin -p ${CONCURRENCY} ./cmd/...

# Ensure that in tests, the language server is accessible
test:: build get_plugins get_schemas
	PATH="${PWD}/bin:${PATH}" PULUMI_LIVE_TEST="${PULUMI_LIVE_TEST}" \
	  go test -v --timeout 30m -count 1 -parallel ${CONCURRENCY} ./pkg/...

test_live:: PULUMI_LIVE_TEST = true
test_live:: test_live_prereq test

test_live_prereq::
ifndef AWS_SECRET_ACCESS_KEY
	@	if ! ( aws sts get-caller-identity >/dev/null ); then echo "AWS credentials are required to run live tests"; exit 1; fi
endif

.phony: test_gen
# We don't include other.json because it is not a real schema
test_gen: get_schemas
	go test --run=TestGenerate ./...

name=$(subst schema-,,$(word 1,$(subst !, ,$@)))
version=$(word 2,$(subst !, ,$@))
schema-%:
	@echo Ensuring $@
	@[ -f pkg/pulumiyaml/testing/test/testdata/${name}.json ] || \
		curl "https://raw.githubusercontent.com/pulumi/pulumi-${name}/v${version}/provider/cmd/pulumi-resource-${name}/schema.json" \
	 	| jq '.version = "${version}"' >  pkg/pulumiyaml/testing/test/testdata/${name}.json
get_schemas: schema-aws!4.26.0 schema-azure-native!1.29.0 schema-azure!4.18.0 schema-kubernetes!3.7.2 schema-random!4.2.0
