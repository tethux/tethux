# CI results viewer

The viewer is an internal archive browser and SQL explorer. It shows run
health, workflow steps, tests, exact artifact bytes, searchable logs, and
diagnostic charts. Saved SQL queries and timestamp-display preferences remain
in browser local storage.

Ingestion verifies every manifest entry by path, size, and SHA-256 before
storing its bytes in SQLite. Re-ingesting an existing run backfills legacy
metadata-only rows.

## Local use

```console
mise run build:viewer-frontend
go run ./tools/ci-results ingest --path ./results/archive --db ./data/ci/ci-res.db
go run ./tools/ci-results serve --db ./data/ci/ci-res.db
```

For continuous ingestion:

```console
go run ./tools/ci-results watch \
  --path ./results/archive \
  --db ./data/ci/ci-res.db
```

The watcher ingests only archives with a valid checksum-bearing `.done`
marker. It combines filesystem events with periodic reconciliation because
Docker bind mounts and network filesystems can miss events.

Validation:

```console
mise run check
```

## NAS deployment

Woodpecker updates the deployment after a successful push to `master`.
`compose.yaml` defines one dedicated Docker network and three containers:

- `tethux-ci-viewer` serves the UI and API;
- `tethux-ci-viewer-ingest` watches the archive bind mount read-only;
- `tethux-ci-viewer-tunnel` provides optional Cloudflare access.

SQLite lives at `/var/cache/tethux-ci/viewer`. Archives are mounted read-only
from `/var/cache/tethux-ci/archive`. No host port is published. Compose uses
explicit container names, so updates preserve `tethux-ci-viewer`,
`tethux-ci-viewer-ingest`, and `tethux-ci-viewer-tunnel`; the Cloudflare
service URL remains stable.

Create a remotely managed Cloudflare Tunnel and add a published application:

- Application type: `HTTP`
- Public hostname: the hostname you want, such as `ci.example.com`
- Service URL: `http://tethux-ci-viewer:8080`

The service name works because `cloudflared` and the viewer share the
`tethux-ci-viewer` Docker network. On the NAS, create:

```console
mkdir -p /Containers/homelab/tethux-ci-viewer
printf '%s\n' 'TUNNEL_TOKEN=replace-with-your-token' \
  > /Containers/homelab/tethux-ci-viewer/.env
chmod 600 /Containers/homelab/tethux-ci-viewer/.env
```

Then deploy or restart the tunnel:

```console
ssh nas
/Containers/homelab/tethux-ci-viewer/tethux-ci deploy viewer
```

Alternatively, `tethux-ci deploy tunnel-token` prompts without echoing and
writes the same `.env` file. The container receives `TUNNEL_TOKEN` through
Docker's `--env-file`; the token is not a command argument or Woodpecker
setting. After editing `.env`, rerun either deploy command to recreate the
tunnel container with the new token.

Canonical source:
https://codeberg.org/tethux/tethux/src/branch/master/tools/ci-results/viewer

Mirror:
https://github.com/tethux/tethux/tree/master/tools/ci-results/viewer
