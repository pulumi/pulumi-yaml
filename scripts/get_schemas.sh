#!/bin/bash

LIST_ONLY=${LIST_ONLY:-false}

default_url_template='https://raw.githubusercontent.com/pulumi/pulumi-_NAME_/v_VERSION_/provider/cmd/pulumi-resource-_NAME_/schema.json'
awsx_url='https://raw.githubusercontent.com/pulumi/pulumi-awsx/v_VERSION_/awsx/schema.json'
function pulumi_schema { echo "$1@$2@https://raw.githubusercontent.com/pulumi/pulumi/master/tests/testdata/codegen/$1-$2.json"; }
schemas=(
  "aws-native@0.99.0"
  "awsx@1.0.0-beta.5@${awsx_url}"
  "docker@4.0.0-alpha.0"
  "eks@0.40.0"
  "random@4.11.2"
  "kubernetes@3.7.0"
  "kubernetes@3.0.0"
  "azure@4.18.0"
  "azure-native@1.56.0"
  "azure-native@2.41.0"
  "aws@5.4.0"
  "google-native@0.18.2"
  $(pulumi_schema synthetic 1.0.0)
  $(pulumi_schema other 0.1.0)
  $(pulumi_schema splat 1.0.0)
  $(pulumi_schema std 1.0.0) # there's no pulumi-std 1.0.0
  $(pulumi_schema snowflake 0.66.1) # not a real pulumi-snowflake schema
  $(pulumi_schema using-dashes 1.0.0)
  $(pulumi_schema aws-static-website 0.4.0)
  $(pulumi_schema basic-unions 0.1.0)
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


  FILEPATH="pkg/pulumiyaml/testing/test/testdata/${NAME}-${VERSION}.json"

  if [ "${LIST_ONLY}" = "true" ]; then
      echo "${FILEPATH}"
      continue
  fi

  if [ -f "${FILEPATH}" ]; then
    FOUND=$(jq -r '.version' "${FILEPATH}") &&
      if ! [ "$FOUND" = "${VERSION}" ]; then
        echo "${NAME} required version ${VERSION} but found existing version ${FOUND}"
        echo "Replacing ${NAME}.json."
        rm "${FILEPATH}"
      fi
  fi
  if [ ! -f "${FILEPATH}" ]; then
    echo "Downloading ${FILEPATH} FROM ${URL}"
    curl "${URL}" \
      | jq ".version = \"${VERSION}\"" > "${FILEPATH}"
  fi
done
