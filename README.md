# tfstate-drift-notifier

[![CI](https://github.com/moveeeax/tfstate-drift-notifier/actions/workflows/ci.yml/badge.svg)](https://github.com/moveeeax/tfstate-drift-notifier/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/moveeeax/tfstate-drift-notifier.svg)](https://pkg.go.dev/github.com/moveeeax/tfstate-drift-notifier)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/moveeeax/tfstate-drift-notifier)](https://goreportcard.com/report/github.com/moveeeax/tfstate-drift-notifier)

Detect Terraform drift **on a schedule** and alert only when it actually appears.

`terraform plan` in CI only catches drift when someone opens a PR. Infra drifts
silently between applies — a manual console edit here, an out-of-band change
there — and the team finds out during the next unrelated PR when the plan is
suddenly huge. `tfstate-drift-notifier` wraps `terraform plan -json`, classifies
the resource changes, prints a compact report, and **exits `2` only when real
drift is found** so a scheduled CI job can fail loudly and post to Slack — and
stays quiet on clean runs.

## Install

Requires Go 1.26 or newer.

```console
go install github.com/moveeeax/tfstate-drift-notifier/cmd/tfstate-drift-notifier@latest
```

## Usage

```console
tfstate-drift-notifier --chdir ./env/prod --slack-webhook "$SLACK_WEBHOOK_URL"
```

Output on a drifted environment:

```text
drift detected: 4 resource(s)
  - aws_iam_role.dead (delete)
  ± aws_instance.app (replace)
  ~ aws_s3_bucket.logs (update)
  ~ aws_security_group.web (update)
summary: 2 update, 1 delete, 1 replace
```

Exit codes mirror `terraform plan -detailed-exitcode`:

| code | meaning       |
|------|---------------|
| `0`  | no drift      |
| `2`  | drift present |
| `1`  | error         |

### Flags

| flag | default | description |
|------|---------|-------------|
| `--chdir` | `.` | directory to run terraform in |
| `--plan-file` | | read a pre-captured `terraform plan -json` stream instead of running terraform |
| `--slack-webhook` | | Slack incoming webhook URL to post the report to |
| `--json` | `false` | emit the report as JSON |
| `--terraform-bin` | `terraform` | terraform binary to invoke |

Markers: `+` create · `~` update · `-` delete · `±` replace. Data-source reads
and no-ops are ignored — they aren't drift.

## Scheduled drift check in CI

```yaml
name: drift
on:
  schedule:
    - cron: '0 * * * *'   # hourly
  workflow_dispatch:
jobs:
  drift:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version: stable
      - uses: hashicorp/setup-terraform@v4
      - run: terraform -chdir=env/prod init -input=false
      - run: |
          go install github.com/moveeeax/tfstate-drift-notifier/cmd/tfstate-drift-notifier@latest
          tfstate-drift-notifier --chdir env/prod --slack-webhook "${{ secrets.SLACK_WEBHOOK_URL }}"
```

The step fails (exit `2`) when drift is detected, so the workflow goes red and
Slack gets a message — otherwise it's a no-op.

## How it works

Terraform's `-json` mode emits one JSON object per line. The tool streams that
output, keeps only `resource_drift` and `planned_change` lines, and collapses
Terraform's action encodings (scalar `action`, `actions` list, and the
`create-then-delete` / `delete-then-create` replace forms) into one of
create/update/delete/replace. Non-JSON and unrelated log lines are tolerated, so
it survives merged stderr and version banners.

## Development

Requires Go 1.26 or newer; CI tests against Go 1.26 and 1.27.

```console
go test ./...
go build ./cmd/tfstate-drift-notifier
```

See [`examples/`](examples/) for runnable fixtures.

## License

[MIT](LICENSE)
