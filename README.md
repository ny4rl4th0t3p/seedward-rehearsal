# seedward-rehearsal

Standalone runner and GitHub Action for the Seedward rehearsal engine
([`pkg/rehearse`](https://github.com/ny4rl4th0t3p/seedward-gentool) in `seedward-gentool`).

It **pre-flights a chain launch**: from a gentool config it builds the candidate genesis, boots an
ephemeral chain on **substitute** validators (the real validators' consensus keys aren't available),
runs the on-chain assertion suite, and always tears the chain down.

A `PASS` certifies that the approved input set assembles and a representative chain initializes and
advances — it is **not** a certification that the real network produces blocks, and it emits no
publishable genesis.

## `cmd/rehearse`

```
rehearse --config gentool.yaml --binary /path/to/chaind [flags]
```

| flag | default | meaning |
|------|---------|---------|
| `--config` | — | gentool YAML config (names the allocation CSVs + gentx dir). **Required.** |
| `--binary` | — | chaind binary used to build and boot the genesis. **Required.** |
| `--binary-sha256` | `""` | expected sha256 of the binary (hex); empty skips the digest check |
| `--validators` | `2` | substitute validators to boot |
| `--boot-wait` | `90s` | max wait for the first block |
| `--json` | `false` | emit a machine-readable result instead of the human report |

Exit codes mirror the engine's tri-state: **0 = PASS, 1 = FAIL, 2 = ERROR**. The chain config and
every input path come from the gentool config; only the binary is passed on the command line.

## GitHub Action

Drop it into a workflow to self-rehearse before submitting. Provision a `chaind` binary first
(`--binary` points at it), then call the action:

```yaml
jobs:
  rehearse:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Provision chaind
        run: |
          curl -fsSL -o chaind \
            "https://github.com/cosmos/gaia/releases/download/v27.2.0/gaiad-v27.2.0-linux-amd64"
          chmod +x chaind

      - id: rehearsal
        uses: ny4rl4th0t3p/seedward-rehearsal@v0.1.0
        with:
          config: ./gentool.yaml
          binary: ./chaind

      - run: echo "outcome=${{ steps.rehearsal.outputs.outcome }}"
```

The step fails (non-zero) on `FAIL`/`ERROR`. Outputs: `outcome` (`PASS`/`FAIL`/`ERROR`) and
`failed-step`. The action sets up Go and builds the runner from source, so a first run pulls and
compiles the cosmos-sdk dependency tree; `actions/setup-go` caching keeps subsequent runs fast.
`jq` is required on the runner (present on GitHub-hosted `ubuntu-*`).

### Versioning

Releases are tagged with semver (`vMAJOR.MINOR.PATCH`). Pin the action to an exact release tag
(e.g. `@v0.1.0`) or, for the strongest supply-chain guarantee, to a full commit SHA. From `v1.0.0`
onward a moving major alias (`@v1`, re-pointed to the latest `v1.x.x` on each release) will be
maintained for convenience; while the project is pre-`1.0`, pin exact tags.