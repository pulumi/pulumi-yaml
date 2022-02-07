PULUMI_TEST_ORG   ?= $(shell pulumi whoami)
PULUMI_TEST_OWNER ?= ${PULUMI_TEST_ORG}
PULUMI_LIVE_TEST  ?= false
export PULUMI_TEST_ORG
export PULUMI_TEST_OWNER

CONCURRENCY       := 10

install::
	go install ./cmd/...

clean::
	rm ./bin/*

ensure::
	go mod download

build:: ensure
	mkdir -p ./bin
	go build -o ./bin -p ${CONCURRENCY} ./cmd/...

# Ensure that in tests, the language server is accessible
test:: build
	PATH="${PWD}/bin:${PATH}" PULUMI_LIVE_TEST="${PULUMI_LIVE_TEST}" \
	  go test -v --timeout 10m -count 1 -parallel ${CONCURRENCY} ./pkg/...

test_live:: PULUMI_LIVE_TEST = true
test_live:: test_live_prereq test

test_live_prereq::
ifndef AWS_SECRET_ACCESS_KEY
	@	if ! ( aws sts get-caller-identity >/dev/null ); then echo "AWS credentials are required to run live tests"; exit 1; fi
endif
