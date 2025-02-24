#!/usr/bin/env bash
# move changes to the dedicated branch created from the remote main and create link on the Gopher dashboard

# standard bash error handling
set -o nounset  # treat unset variables as an error and exit immediately.
set -o errexit  # exit immediately when a command fails.
set -E          # needs to be set if we want the ERR trap
set -o pipefail # prevents errors in a pipeline from being masked# link the PR from ^^ to gopher project board

pr_id=$(gh api repos/MarekMichali/kyma-environment-broker/pulls/122 | jq -r '.node_id')

# Gopher board node_id
readonly project_board_id=PVT_kwHOA1j9cM4APn2j
# "To Do" column on Gopher board node_id
readonly todo_column_id=f75ad846
# order in "To Do" column on Gopher board node_id
readonly status_field=PVTSSF_lAHOA1j9cM4APn2jzgJ-TWc

# insert projectv2 item (card on the gopher board)
resp=$(gh api graphql -f query='mutation{ addProjectV2ItemById(input:{projectId: "'${project_board_id}'" contentId: "'${pr_id}'"}){ item{id} }}' )
echo "response from inserting projectv2 item: $resp"
card_id=$(echo "$resp" | jq -r '.data.addProjectV2ItemById.item.id')

# move projectv2 item (card on the gopher board) to the top of the "To Do" column
# due to GitHub internal GraphQL limitation, adding item and update has to be two separate calls
# https://docs.github.com/en/issues/planning-and-tracking-with-projects/automating-your-project/using-the-api-to-manage-projects#updating-projects
gh api graphql -f query="$(cat << EOF
  mutation {
    set_status: updateProjectV2ItemFieldValue(input: {
      projectId: "$project_board_id"
      itemId: "$card_id"
      fieldId: "$status_field"
      value: {
        singleSelectOptionId: "$todo_column_id"
      }
    }){projectV2Item {id}}
    set_position: updateProjectV2ItemPosition(input: {
      projectId: "$project_board_id"
      itemId: "$card_id"
    }){items {totalCount}}
  }
EOF
)"
