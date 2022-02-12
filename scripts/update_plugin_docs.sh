#!/bin/sh

PLUGIN_NAME="$1"
PLUGIN_VERSION="$2"

set -xeu
for f in examples/**/README.md; do
  sed -i "s/install resource ${PLUGIN_NAME} [^s]*/install resource ${PLUGIN_NAME} ${PLUGIN_VERSION}/g" "$f"
done
