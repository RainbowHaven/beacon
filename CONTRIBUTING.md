# Contributing to Beacon

## Contributor License Agreement

Beacon uses a **copyright assignment** CLA so the project steward can hold a single copyright interest, transfer the project to an NGO, and relicense if needed.

- Full terms: [CLA.md](./CLA.md)
- Enforcement: GitHub Action [`rdkcentral/contributor-assistant_github-action`](https://github.com/rdkcentral/contributor-assistant_github-action) (maintained fork; Node 24). Not the archived `contributor-assistant/github-action` or the old cla-assistant.io service.

On your first pull request, the CLA Assistant will comment. Sign by posting exactly:

```text
I have read the CLA Document and I hereby sign the CLA
```

Signatures are stored in `signatures/version1/cla.json` on `main` (created/updated by the Action — do not hand-edit).

`magiconair`, `cursoragent`, and GitHub bots are allowlisted and do not need to sign through the workflow.

**Note:** The first pull request after enabling CLA may fail the CLA check once while the empty signatures file is bootstrapped on `main`. Comment `recheck` on the PR to clear it.


## Local checks (CLA)

From the repo root:

```bash
./scripts/verify-cla.sh
```

This validates documents and workflow shape offline (no GitHub API calls).

Optional richer local dry-run:

```bash
brew install act actionlint
./scripts/verify-cla.sh
```

`act` can dry-run the workflow with fixtures under `testdata/cla/`. A full signing round-trip still needs a real pull request from a **non-allowlisted** GitHub user after the workflow is on `main`.
