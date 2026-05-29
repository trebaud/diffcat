---
name: releaser
description: >
  Create a new diffcat release: build cross-platform artifacts with scripts/releaser.sh,
  generate a changelog since the previous tag, and publish a GitHub release via gh.
  Use when the user says "release", "cut a release", "publish v1.x", or "new version".
allowed-tools: Bash
---

# Release diffcat

Create a new GitHub release for the diffcat project.

## Arguments

`$ARGUMENTS` may be:
- A full version tag: `v1.2.0` — used as-is, skips version analysis
- A bump keyword: `patch`, `minor`, or `major` — overrides the inferred bump
- Empty — infer the bump level from the changes (see step 1)

## Steps

### 1. Determine the version bump

Get the latest tag and the commits since it:
```bash
git describe --tags --abbrev=0
git log <LATEST_TAG>..HEAD --oneline --no-merges
git diff <LATEST_TAG>..HEAD
```

If `$ARGUMENTS` is a full version tag (starts with `v`), skip this analysis and use it directly.

Otherwise, analyze the commits and diff to classify the bump:

**MAJOR** — any of:
- A commit message contains `BREAKING CHANGE:` or a type with `!` (e.g. `feat!:`, `fix!:`)
- A public API, CLI flag, config key, or command name was removed or its behavior changed in an incompatible way
- A public interface or struct field was removed or renamed in a breaking way

**MINOR** — none of the above, and any of:
- A commit type is `feat:` or similar (new feature added)
- A new CLI command, flag, or config option was introduced
- New functionality added without breaking existing behavior

**PATCH** — only backwards-compatible fixes, chores, or documentation:
- Commit types: `fix:`, `chore:`, `docs:`, `refactor:`, `perf:`, `test:`, `style:`
- No new features, no breaking changes

If `$ARGUMENTS` is a bump keyword (`patch`, `minor`, `major`), use that instead of the inferred level.

Compute the next version by incrementing the appropriate component of the latest tag:
- `major` → bump first number, reset minor and patch to 0
- `minor` → bump second number, reset patch to 0
- `patch` → bump third number

Always prefix with `v` (e.g. `v1.2.3`). If there are no tags yet, start at `v0.1.0`.

Present the inferred bump level, the reasoning, and the resolved version to the user, then ask for confirmation before proceeding.

### 2. Build release artifacts and tag

```bash
bash scripts/releaser.sh <VERSION>
```

This produces `./dist/*.tar.gz` and `./dist/*.sha256` for all supported platforms, then creates and pushes the git tag `<VERSION>`. Fail loudly if the script exits non-zero.

### 3. Generate changelog

Get the previous tag (the one before the latest):
```bash
git tag --sort=-version:refname | sed -n '2p'
```

Then collect commits between the previous tag and HEAD:
```bash
git log <PREV_TAG>..HEAD --oneline --no-merges
```

Filter and rewrite the commits into concise, user-facing release notes:

- **Include only** changes that affect end users: new features, bug fixes, UX improvements, performance gains visible to users.
- **Exclude** anything that is purely internal: CI/CD, devops, documentation, refactoring, dependency bumps, test changes, chores.
- Prefix each entry with a short type label: `feature:`, `bug:`, `perf:`, `ux:`.
- Keep each entry to one short sentence. Do not copy the raw commit message verbatim — rephrase it to describe what the user experiences.

Format the notes as:

```
## What's Changed

- feature: <what the user can now do>
- bug: <what was broken and is now fixed>
...

**Full changelog**: https://github.com/trebaud/diffcat/compare/<PREV_TAG>...<NEW_VERSION>
```

If no commits pass the user-facing filter, write "No user-facing changes in this release."

### 4. Create the GitHub release

```bash
gh release create <VERSION> ./dist/*.tar.gz ./dist/*.sha256 \
  --title "<VERSION>" \
  --notes "<NOTES>"
```

Print the release URL when done.

### 5. Warm the Go module proxy

```bash
bash scripts/deploy.sh
```

This confirms `go install github.com/trebaud/diffcat/cmd/diffcat@latest` resolves the new tag. Propagation can take a minute — re-run if it reports the proxy hasn't picked it up yet.
