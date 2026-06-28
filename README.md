# seedward-rehearsal

Consumers of the Seedward rehearsal engine
([`pkg/rehearse`](https://github.com/ny4rl4th0t3p/seedward-gentool) in `seedward-gentool`): a
standalone CLI, a GitHub Action, and a coordd-connected daemon.

The engine **pre-flights a chain launch**: from a launch's approved inputs it builds the candidate
genesis, boots an ephemeral chain on **substitute** validators (the real validators' consensus keys
aren't available), runs the on-chain assertion suite, and always tears the chain down.

A `PASS` certifies that the approved input set assembles and a representative chain initializes and
advances — it is **not** a certification that the real network produces blocks, and it emits no
publishable genesis.

## Components

| Path              | What it is                                                                                                                   |
|-------------------|------------------------------------------------------------------------------------------------------------------------------|
| `cmd/rehearse`    | Standalone one-shot CLI — runs a rehearsal over **local** inputs. No coordd.                                                 |
| `action.yml`      | GitHub Action wrapping `cmd/rehearse`, for chain teams to self-rehearse in CI.                                               |
| `cmd/rehearsald`  | Coordd-connected daemon — pulls a launch's approved input from coordd, runs, and posts a **signed** result fact back.        |
| `internal/bridge` | coordd bridge client + the signed result fact ([`bridge-contract.md`](https://github.com/ny4rl4th0t3p/seedward-chaincoord)). |
| `internal/daemon` | The daemon's config, HTTP server, and run orchestration.                                                                     |

The result-fact signature uses `canonicaljson` from
[`seedward-commons`](https://github.com/ny4rl4th0t3p/seedward-commons), the same implementation
coordd verifies against.

## `cmd/rehearse` (standalone)

```
rehearse --config gentool.yaml --binary /path/to/chaind [flags]
```

| flag              | default | meaning                                                                    |
|-------------------|---------|----------------------------------------------------------------------------|
| `--config`        | —       | gentool YAML config (names the allocation CSVs + gentx dir). **Required.** |
| `--binary`        | —       | chaind binary used to build and boot the genesis. **Required.**            |
| `--binary-sha256` | `""`    | expected sha256 of the binary (hex); empty skips the digest check          |
| `--validators`    | `2`     | substitute validators to boot                                              |
| `--boot-wait`     | `90s`   | max wait for the first block                                               |
| `--json`          | `false` | emit a machine-readable result instead of the human report                 |

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
        uses: ny4rl4th0t3p/seedward-rehearsal@v0.1.1
        with:
          config: ./gentool.yaml
          binary: ./chaind

      - run: echo "outcome=${{ steps.rehearsal.outputs.outcome }}"
```

The step fails (non-zero) on `FAIL`/`ERROR`. Outputs: `outcome` (`PASS`/`FAIL`/`ERROR`) and
`failed-step`. The action sets up Go and builds the runner from source, so a first run pulls and
compiles the cosmos-sdk dependency tree; `actions/setup-go` caching keeps subsequent runs fast.
`jq` is required on the runner (present on GitHub-hosted `ubuntu-*`).

## `cmd/rehearsald` (daemon)

The coordd-connected service. It **never initiates** toward coordd — a run happens only when
triggered (the bridge is rehearsal-initiated). On a trigger it pulls the approved input, runs the
engine, Ed25519-signs the result fact, and posts it back; coordd verifies the signature against the
launch's trusted service pubkey.

```
rehearsald --config rehearsald.yaml
```

**Endpoints:**

| method + path                  | auth             | behavior                                                                                                                     |
|--------------------------------|------------------|------------------------------------------------------------------------------------------------------------------------------|
| `POST /launches/{id}/rehearse` | ops bearer token | Pull input → run → sign → post the fact. Streams NDJSON progress. Serialized — returns `409` while another run is in flight. |
| `GET /healthz`                 | none             | Liveness check.                                                                                                              |

Auth is an ops credential (no wallet identity on this plane): `Authorization: Bearer <ops_token>`.

**Trigger response** is newline-delimited JSON, one event per line:

```jsonc
{"kind":"step","step":{"name":"build","status":"PASS","detail":""}}
{"kind":"step","step":{"name":"assert:supply_reconciles","status":"PASS","detail":"7000000"}}
{"kind":"fact","fact":{ /* the signed result fact posted to coordd */ }}
{"kind":"error","error":"..."}   // only on failure; ends the stream
```

**Config** (`rehearsald.yaml`; any key is overridable via env `REHEARSALD_<KEY>`, e.g.
`REHEARSALD_OPS_TOKEN`):

```yaml
listen_addr: ":8088"                       # default :8088
coordd_url: "https://coordd.example"      # required
ops_token: "…"                           # required (prefer REHEARSALD_OPS_TOKEN)
service_key_path: "/etc/rehearsald/service.key" # required — base64 Ed25519 (32-byte seed or 64-byte key)
binary_path: "/usr/local/bin/chaind"       # required — the pre-provisioned chaind
validators: 2                              # default 2
boot_wait: "90s"                          # default 90s
```

The result fact's `engine_version` is **not** configured — it's the binary's build version, injected
at build time (`make build` sets it from `git describe`; a plain `go build` reports `dev`). Both
binaries also accept `--version`.

## Build & test

```
make build   # → build/rehearse and build/rehearsald
make test    # unit tests
make lint    # golangci-lint
make check   # fmt + vet + tidy + lint + test
```

The full boot path (the engine actually running `chaind`) is integration-tested in `seedward-gentool`;
this repo's tests cover the runner wiring, the bridge mapping/signing, and the daemon flow with fakes.

## Versioning

Releases are tagged with semver (`vMAJOR.MINOR.PATCH`). Pin the action to an exact release tag
(e.g. `@v0.1.0`) or, for the strongest supply-chain guarantee, to a full commit SHA. From `v1.0.0`
onward a moving major alias (`@v1`, re-pointed to the latest `v1.x.x` on each release) will be
maintained for convenience; while the project is pre-`1.0`, pin exact tags.