# Review Handoff: Go SDK Changelog

## Scope

This change adds SDK release notes for the current experimental v0 preview.

Changed files:

- `sdk/go/lore/CHANGELOG.md`
- `sdk/go/lore/README.md`
- `docs/review-handoff-codex53-sdk-changelog.md`

## Contents

`CHANGELOG.md` documents `v0.1.0-unreleased`:

- stdio transport;
- lifecycle methods;
- typed read-only methods;
- public DTOs;
- error types;
- opt-in E2E smoke;
- argument contract;
- read-only/governance boundaries;
- known limitations.

README status now points readers to the changelog.

## Review Focus

Please check:

- The changelog does not imply a frozen public module path.
- Boundaries remain read-only and stdio-only.
- Known limitations match the current implementation.
