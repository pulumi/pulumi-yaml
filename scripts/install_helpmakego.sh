#!/usr/bin/env bash

set -euo pipefail

HELPMAKEGO_VERSION="$1"
HELPMAKEGO=bin/${HELPMAKEGO_VERSION}/helpmakego

if ! [ -x "${HELPMAKEGO}" ]; then
    echo "Installing helpmakego@${HELPMAKEGO_VERSION}"
    GOBIN=${PWD}/bin/${HELPMAKEGO_VERSION} go install -mod readonly "github.com/iwahbe/helpmakego@${HELPMAKEGO_VERSION}"
fi
