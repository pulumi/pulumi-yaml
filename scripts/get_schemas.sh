#!/bin/bash

default_url_template='https://raw.githubusercontent.com/pulumi/pulumi-_NAME_/v_VERSION_/provider/cmd/pulumi-resource-_NAME_/schema.json'
awsx_url='https://raw.githubusercontent.com/pulumi/pulumi-awsx/v_VERSION_/awsx/schema.json'
schemas=(
  "aws@5.4.0"
  "azure-native@1.56.0"
  "azure@4.18.0"
  "kubernetes@3.7.2"
  "random@4.2.0"
  "eks@0.40.0"
  "aws-native@0.13.0"
  "docker@3.1.0"
  "awsx@1.0.0-beta.5@${awsx_url}"
)

for s in "${schemas[@]}"; do
  IFS="@"
  set -- $s
  NAME="${1}"
  VERSION="${2}"
  URL="${3:-$default_url_template}"
  # Substitute name, version:

  URL="${URL//_NAME_/${NAME}}"
  URL="${URL//_VERSION_/${VERSION}}"


  FILEPATH="pkg/pulumiyaml/testing/test/testdata/${NAME}.json"
  if [ -f "${FILEPATH}" ]; then
    FOUND=$(jq -r '.version' "${FILEPATH}") &&
      if ! [ "$FOUND" = "${VERSION}" ]; then
        echo "${NAME} required version ${VERSION} but found existing version ${FOUND}"
        echo "Replacing ${NAME}.json."
        rm "${FILEPATH}"
      fi
  fi

  if [ ! -f "${FILEPATH}" ]; then
    curl "${URL}" \
      | jq ".version = \"${VERSION}\"" > "${FILEPATH}"
  fi
done
