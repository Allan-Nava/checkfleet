#!/usr/bin/env bash
# Monthly discoverability snapshot (CF-148): the four numbers of
# marketing/SOCIAL-PLAN.md §8 that an API can answer, as a markdown section.
#
#   scripts/metrics.sh            # print the section
#   scripts/metrics.sh --append   # append it to marketing/METRICS.md
#
# GitHub traffic is a rolling 14-day window, so a monthly run samples it rather
# than summing it: compare runs, don't add them. Needs `gh` authenticated with
# push access to the repo (the traffic API refuses read-only tokens).
#
# The fifth metric (what assistants answer) and Search Console impressions have
# no API worth wiring here: fill those rows in by hand.
set -euo pipefail

repo="${CHECKFLEET_REPO:-Allan-Nava/checkfleet}"
out="$(dirname "$0")/../marketing/METRICS.md"

api() { gh api "repos/$repo/$1" --jq "$2"; }

views=$(api traffic/views '"\(.count) views / \(.uniques) unique"')
clones=$(api traffic/clones '"\(.count) clones / \(.uniques) unique"')
referrers=$(api traffic/popular/referrers \
  'if length == 0 then "none" else [.[] | "\(.referrer) (\(.count))"] | join(", ") end')
downloads=$(gh api --paginate "repos/$repo/releases?per_page=100" --jq '.[].assets[].download_count' |
  awk '{s += $1} END {print s + 0}')
latest=$(gh api "repos/$repo/releases/latest" --jq '"\(.tag_name): \([.assets[].download_count] | add // 0)"')
stars=$(gh api "repos/$repo" --jq '"\(.stargazers_count) stars, \(.forks_count) forks"')

section=$(cat <<EOF

## $(date +%Y-%m-%d)

| Metric | Value |
|---|---|
| Views (14 days) | $views |
| Clones (14 days) | $clones |
| Referring sites (14 days) | $referrers |
| Release downloads (all time) | $downloads (latest $latest) |
| Stars / forks | $stars |
| Search Console impressions & top queries | _fill in by hand_ |
| Assistant answers | _fill in by hand: see the prompts at the top of this file_ |
EOF
)

if [[ "${1:-}" == "--append" ]]; then
  printf '%s\n' "$section" >> "$out"
  echo "appended to $out"
else
  printf '%s\n' "$section"
fi
