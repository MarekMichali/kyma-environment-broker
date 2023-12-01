import requests
import os
import yaml
import sys

with open('.github/release.yml', 'r') as file:
    try:
        release_yaml = yaml.safe_load(file)
        label_pool = []
        for category in release_yaml['changelog']['categories']:
            label_pool.extend(category['labels'])
    except yaml.YAMLError as exc:
        print(exc)

# You need to pass your GitHub username and generated token
token = os.getenv('GITHUB_TOKEN')
repo = 'kyma-project/kyma-environment-broker'  # replace with your repo name

# Get the latest release
response = requests.get(f'https://api.github.com/repos/{repo}/releases/latest',
                        headers={'Authorization': f'token {token}'})
response.raise_for_status()
latest_release = response.json()

# Get the date of the latest release
latest_release_date = latest_release['created_at']

# Get PRs since the latest release
response = requests.get(f'https://api.github.com/repos/{repo}/pulls?state=closed&sort=updated&direction=desc',
                        headers={'Authorization': f'token {token}'})
response.raise_for_status()

all_closed_prs = response.json()

# Filter PRs to only include those since latest release
prs_since_last_release = [
    pr for pr in all_closed_prs
    if pr['merged_at'] is not None and pr['merged_at'] > latest_release_date
]

# Check each PR for labels
valid_prs = []
invalid_prs = []
for pr in prs_since_last_release:
    labels = [label['name'] for label in pr['labels']]
    common_labels = set(labels).intersection(label_pool)
    if len(common_labels) != 1:
        invalid_prs.append((pr['number'], pr['html_url']))
    else:
        valid_prs.append(pr['number'])

# Print valid PRs
print("\nThese PRs have exactly one label from the pool:")
for pr_number in valid_prs:
    print(f"PR #{pr_number}")

# Print invalid PRs
if invalid_prs:
    print("\nThese PRs do not have exactly one label from the pool:")
    for pr_number, pr_url in invalid_prs:
        print(f"PR #{pr_number}: {pr_url}")

    sys.exit(1)  # exit with failure status if invalid PRs exist
else:
    print("\nAll PRs have exactly one label from the pool")