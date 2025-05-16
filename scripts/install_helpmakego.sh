#!/usr/bin/env bash

set -euo pipefail

HELPMAKEGO_VERSION="$1"
HELPMAKEGO=bin/${HELPMAKEGO_VERSION}/helpmakego

if ! [ -x ${HELPMAKEGO} ]; then
    echo "Installing helpmakego@${HELPMAKEGO_VERSION}"
    go install -mod readonly "github.com/iwahbe/helpmakego@${HELPMAKEGO_VERSION}"
    mkdir -p "bin/${HELPMAKEGO_VERSION}"
    cp $(go env GOPATH)/bin/helpmakego ${HELPMAKEGO}
fi
