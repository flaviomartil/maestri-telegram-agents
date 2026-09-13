# Credentials and publication

Local audit performed on September 13, 2026. No real credentials were found in the current code or inspected Git history. This is an inspection result, not a guarantee that every possible secret is absent.

## What was checked

- Gitleaks 8.30.1, using default rules and redacted reports: the working directory and history across all refs, covering 116 upstream commits.
- Credential filenames, personal paths and project identifiers in files intended for publication, including untracked files.
- Loading `TELEGRAM_BOT_TOKEN`, `MAESTRI_WIRE_TOKEN` and `MAESTRI_LLM_KEY` from the environment, without tokens embedded in Relay configuration.

The scanner found one case in both the working directory and history: `internal/domain/redact_test.go`, lines 34–35, introduced by upstream commit `1bcbfa18f00d5dfb4d942753d4c2a770079f4f55`. This is a redaction test fixture containing a private-key marker and truncated content, not a usable cryptographic key. The finding was inspected rather than suppressed with a broad rule. Other test tokens are synthetic values used with simulated servers.

The inspection did not authenticate tokens against providers or access credential vaults. Raw reports and internal audit artifacts are kept outside the files intended for publication.

## Preparation completed

Fork-specific examples use generic project names. The machine's personal path was removed from documentation. `.gitignore` now covers `.env`, `wire-token`, `relay.json`, local configuration, state/catalog directories, exported partituras, and local agent instructions/memory. Ignoring files does not remove tracked files; none of these credential files was tracked during this inspection.

Each user supplies their own Telegram bot, Wire pairing and optional AI provider. The defaults store configuration and pairing outside the repository directory. State contains user messages, plans and bindings, and must also remain private.

The upstream code and MIT license were preserved. The Maestri Guide is downloaded locally at a pinned revision: its content and generated partituras are not redistributed by this fork because no redistribution license was found for the guide.

## Repeat before publishing a new version

With Gitleaks installed, run from the repository root and keep reports outside the repository:

```sh
gitleaks git . --log-opts='--all' --redact --report-format json --report-path /tmp/relay-history.json
gitleaks dir . --redact --report-format json --report-path /tmp/relay-worktree.json
git status --short
git diff --check
```

The synthetic case above causes Gitleaks to exit with code 1; investigate any change in file, line or rule. Do not automatically classify every finding as a false positive. This document records the pre-publication audit; creating the remote repository and pushing to GitHub are separate operations.
