#!/bin/bash

LIST_ONLY=${LIST_ONLY:-false}

default_url_template='https://raw.githubusercontent.com/pulumi/pulumi-_NAME_/v_VERSION_/provider/cmd/pulumi-resource-_NAME_/schema.json'
awsx_url='https://raw.githubusercontent.com/pulumi/pulumi-awsx/v_VERSION_/awsx/schema.json'
function pulumi_schema { echo "$1@$2@pulumi/tests/testdata/codegen/$1-$2.json"; }
schemas=(
  "awsx@1.0.0-beta.5@${awsx_url}"
  "random@4.11.2"
  "kubernetes@3.7.0"
  "aws@5.4.0"
  $(pulumi_schema other 0.1.0)
  $(pulumi_schema std 1.0.0) # there's no pulumi-std 1.0.0
  $(pulumi_schema using-dashes 1.0.0)
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
    if [[ "${URL}" == http* ]]; then
      echo "Downloading ${FILEPATH} from ${URL}"
      curl "${URL}" \
        | jq ".version = \"${VERSION}\"" > "${FILEPATH}"
    else
      if [ ! -f "${URL}" ]; then
        echo "Missing ${URL}; run 'git submodule update --init' to fetch it" >&2
        exit 1
      fi
      echo "Copying ${FILEPATH} from ${URL}"
      jq ".version = \"${VERSION}\"" "${URL}" > "${FILEPATH}"
    fi
  fi
done
