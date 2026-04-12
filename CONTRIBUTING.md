# Contributing

Thanks for contributing to `backrest-config-backup`.

This project backs up Backrest `config.json` files to S3-compatible storage after encrypting them locally with `age`. Good contributions are small, well-tested, and clear about behavior changes.

## Before You Start

- Read [README.md](README.md) for the project purpose, runtime configuration, and Docker usage.
- Check existing issues and pull requests before starting work.
- Before opening any pull request, first create an issue describing the bug you want to fix or the feature you want to implement.
- Use that issue to document the problem, scope, and proposed approach before implementation starts.

## Ways to Contribute

Contributions are welcome.

- Report bugs or unexpected behavior.
- Improve documentation and examples.
- Add tests or fix missing edge cases.
- Submit code changes for agreed improvements.

## Reporting Issues

When opening an issue, include:

- A short summary of the problem.
- The environment you are using, including Go version, container/runtime details, and S3-compatible provider when relevant.
- Clear reproduction steps.
- Expected behavior and actual behavior.
- Relevant logs or error messages.

Because this project handles backup configuration and credentials, redact secrets, access keys, bucket names, passphrases, and healthcheck URLs before posting anything publicly.

## Development Setup

The repository uses Go. The version in [go.mod](go.mod) is the source of truth.

Typical setup:

```bash
go test ./...
```

For local container-based testing, the repository also includes `docker-compose.dev.yml`. It expects local development files such as:

- `.env.dev`
- `config.json`
- `age_passphrase.txt`

If you use the development compose setup, keep test credentials isolated from any production bucket or real Backrest instance.

## Code Style

- Keep changes focused on a single problem.
- Follow standard Go formatting with `gofmt`.
- Prefer simple, explicit code over additional abstraction.
- Add or update tests when behavior changes.
- New features must include tests.
- Update documentation when configuration, runtime behavior, or user-facing workflows change.
- New features must include the necessary documentation updates.

Useful commands:

```bash
gofmt -w .
go test ./...
```

Integration tests are opt-in:

```bash
go test -tags=integration ./...
```

Current integration coverage in this repository expects these environment variables when applicable:

- `INTEGRATION_S3_BUCKET`
- `INTEGRATION_AWS_REGION`
- `INTEGRATION_S3_ENDPOINT` (optional depending on provider)

## Pull Request Guidelines

Pull requests must start from an existing issue. Do not open a pull request before creating an issue that documents the bug being fixed or the feature being implemented.

When opening a pull request:

- Use a clear title and describe the problem being solved.
- Explain the change at a high level and call out any tradeoffs.
- Link the related issue.
- Bug fixes should include tests when practical.
- New features must include tests.
- New features must include the necessary documentation updates.
- Update `README.md` or other docs if usage, configuration, or operations changed.
- Keep the pull request scoped; separate unrelated changes into different pull requests.

Before submitting, make sure:

- `go test ./...` passes.
- Code is formatted with `gofmt`.
- No secrets, credentials, or environment-specific files are included in the diff.

## Security

Do not open a public issue containing a suspected security vulnerability with live exploit details or credentials. Report it privately to the maintainer first.

## License

By contributing, you agree that your contributions will be licensed under the terms of the [MIT License](LICENSE).
