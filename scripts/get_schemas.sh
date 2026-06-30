#!/bin/bash

LIST_ONLY=${LIST_ONLY:-false}

default_url_template='https://raw.githubusercontent.com/pulumi/pulumi-_NAME_/v_VERSION_/provider/cmd/pulumi-resource-_NAME_/schema.json'
awsx_url='https://raw.githubusercontent.com/pulumi/pulumi-awsx/v_VERSION_/awsx/schema.json'
schemas=(
  "awsx@1.0.0-beta.5@${awsx_url}"
  "random@4.11.2"
  "kubernetes@3.7.0"
  "aws@5.4.0"
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
    echo "Downloading ${FILEPATH} from ${URL}"
    curl "${URL}" \
      | jq ".version = \"${VERSION}\"" > "${FILEPATH}"
  fi
done
