# Examples

The `testdata/` directory holds two captured `terraform plan -json` streams you
can run against without a real Terraform install:

- `testdata/drift.jsonl` — one manually-changed bucket, a security-group update,
  a replaced instance and a deleted role. Reports 4 resources, exits `2`.
- `testdata/clean.jsonl` — infrastructure matches config. Prints `no drift`,
  exits `0`.

```console
$ tfstate-drift-notifier --plan-file testdata/drift.jsonl
drift detected: 4 resource(s)
  - aws_iam_role.dead (delete)
  ± aws_instance.app (replace)
  ~ aws_s3_bucket.logs (update)
  ~ aws_security_group.web (update)
summary: 2 update, 1 delete, 1 replace

$ echo $?
2
```

## Against real infrastructure

```console
$ tfstate-drift-notifier --chdir ./env/prod --slack-webhook "$SLACK_WEBHOOK_URL"
```

## Capturing a plan stream for later analysis

```console
$ terraform -chdir=./env/prod plan -json -detailed-exitcode > plan.jsonl
$ tfstate-drift-notifier --plan-file plan.jsonl --json
```
