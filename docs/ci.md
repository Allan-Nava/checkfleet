---
title: CI integration
nav_order: 7
---

[← back to index](index.md)

# CI integration

checkfleet is built to run in a pipeline. Because a check that ran is a
*success*, a normal run exits `0` regardless of findings — you decide when a
finding should fail the build.

## Gate with `--exit-on-bad`

The simplest gate: exit `2` when any BAD/ERROR finding is present.

```bash
checkfleet check all --config checkfleet.yml --exit-on-bad
```

WARN findings do **not** trip this gate — only BAD and ERROR do.

## GitHub Actions

```yaml
name: fleet-checks
on:
  schedule:
    - cron: "0 * * * *"   # hourly
  workflow_dispatch:

jobs:
  checks:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
      - run: go install github.com/Allan-Nava/checkfleet/cmd/checkfleet@latest
      - name: Run checks and post a report
        run: |
          checkfleet check all --config checkfleet.yml --output markdown >> "$GITHUB_STEP_SUMMARY"
          checkfleet check all --config checkfleet.yml --exit-on-bad
```

The first line attaches the ops report to the job summary; the second fails the
job on BAD/ERROR.

## Gating on JSON

If you'd rather branch in a script, parse the `worst` field:

```bash
worst=$(checkfleet check all --config checkfleet.yml --output json | jq -r '.worst')
case "$worst" in
  BAD|ERROR) echo "fleet unhealthy: $worst"; exit 1 ;;
  *)         echo "fleet ok ($worst)" ;;
esac
```

## Cron

```cron
# hourly, mail the report on BAD/ERROR only
0 * * * * checkfleet check all --config /etc/checkfleet.yml --exit-on-bad --output markdown || mail -s "checkfleet: fleet unhealthy" ops@example.com
```
