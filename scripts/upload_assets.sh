#!/usr/bin/env bash

# This script has the following argument:
#     - releaseID (mandatory)
# ./upload_assets.sh 12345678

RELEASE_ID=${1}
KEB_CHART=${2}

# standard bash error handling
set -o nounset  # treat unset variables as an error and exit immediately.
set -o errexit  # exit immediately when a command fails.
set -E          # needs to be set if we want the ERR trap
set -o pipefail # prevents errors in a pipeline from being masked

# Expected variables:
#   PULL_BASE_REF - name of the tag
#   BOT_GITHUB_TOKEN - github token used to upload the template yaml

uploadFile() {
  filePath=${1}
  ghAsset=${2}

  response=$(curl -s -o output.txt -w "%{http_code}" \
                  --request POST --data-binary @"$filePath" \
                  -H "Authorization: token $BOT_GITHUB_TOKEN" \
                  -H "Content-Type: text/yaml" \
                   $ghAsset)
  if [[ "$response" != "201" ]]; then
    echo "::error ::Unable to upload the asset ($filePath): "
    echo "::error ::HTTP Status: $response"
    cat output.txt
    exit 1
  else
    echo "$filePath uploaded"
  fi
}

KEB_CHART_PATH="./resources/$KEB_CHART"
UPLOAD_URL="https://uploads.github.com/repos/MarekMichali/kyma-environment-broker/releases/${RELEASE_ID}/assets"

echo -e "\n--- Updating GitHub release ${RELEASE_ID} with $}KEB_CHART} asset"

[[ ! -e ${KEB_CHART_PATH} ]] && echo "::error ::Packaged KEB chart does not exist" && exit 1

uploadFile "${KEB_CHART_PATH}" "${UPLOAD_URL}?name=${KEB_CHART}"
