# Security Policy

## Core Policy

This project follows a Fail-Fast policy. If a security risk, unsafe dependency, or vulnerable workflow is found, we prefer to stop, expose the problem clearly, and fix it as soon as possible.

## Supported Versions

We keep dependency modules and packages up to date. We do not provide security support for older release lines. Please use the latest available version.

| Version | Supported |
| --- | --- |
| `@latest` | :white_check_mark: |
| Older versions | :x: |

## Reporting a Vulnerability

Please report vulnerabilities on the GitHub Issues page:

<https://github.com/KEINOS/go-verdict/issues>

Thank you for helping improve the security of this project. We review vulnerability reports as soon as possible and handle confirmed issues with high priority.

## Minimum Security Measures

As a minimum security measure, we:

- Run unit tests with coverage and the Go race detector.
- Run Go linting, Go modernization checks, and Markdown linting.
- Run platform CI on Linux, macOS, and Windows.
- Run Linux end-to-end checks for documented workflows.
- Run `govulncheck` locally and in GitHub Actions.
- Upload `govulncheck` SARIF results to GitHub Code Scanning when the workflow event can write security alerts.
- Run Dependabot for Go modules and GitHub Actions.
- Keep dependency modules and packages up to date, and fix reported vulnerabilities as soon as possible.
