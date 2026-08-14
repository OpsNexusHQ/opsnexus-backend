# Contributing to OpsNexus Backend

Describe HTTP, migration, authentication, alerting, events, notification, retention, and operational impact in the pull request. Keep behavior aligned with opsnexus-api and shared models aligned with opsnexus-common.

Run gofmt, go vet ./..., and go test ./... where available. Database changes require migration and rollback notes. Never commit secrets, local databases, generated binaries, or production data.
