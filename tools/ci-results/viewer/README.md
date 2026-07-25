# CI results viewer

This is an internal tool for inspecting the SQLite databases produced by the
Tethux CI-results pipeline. It is intended for local development and debugging,
not as a public or hosted service.

The Go server exposes a read-only results API and embeds the static Svelte
frontend. The viewer includes run, test, and artifact pages plus a SQL explorer
with schema completion, configurable result summaries, row details, nested JSON
inspection, browser-local saved queries, relative/calendar timestamp modes, and
persistent light/dark themes.

Ingestion verifies every manifest file by size and SHA-256 and stores its exact
bytes in SQLite. Re-running ingestion against an existing archive backfills
legacy metadata-only rows without duplicating the run. The artifact workbench
can filter and preview textual files and images; binary files and packet
captures are available as exact-byte downloads. Private entries remain visibly
marked and the service binds only to loopback.

## Run locally

Build the frontend before compiling or running the Go viewer:

```sh
cd tools/ci-results/viewer/frontend
npm install
npm run build

cd ../../../..
go run ./tools/ci-results serve -db /path/to/ci-results.sqlite
```

The server listens on `127.0.0.1:8080` by default. Run
`go run ./tools/ci-results serve -h` to see the available flags.

## Frontend checks

```sh
cd tools/ci-results/viewer/frontend
npm run check
npm run lint
npm run test:theme
npm run build
```

## Source

Saved SQL queries are versioned in browser local storage and never synchronized
to the results database. To populate bytes for an older database, run the same
`ingest` command again with the original archive path.

The canonical viewer source is:

https://codeberg.org/tethux/tethux/src/branch/master/tools/ci-results/viewer

The GitHub repository is a secondary mirror:

https://github.com/tethux/tethux/tree/master/tools/ci-results/viewer

## NAS deployment

The NAS workflow builds the viewer image on the NAS runner, keeps its SQLite
database under `/var/cache/tethux-ci/viewer`, and attaches the viewer and tunnel
connector to the dedicated `tethux-ci-viewer` Docker network. The viewer
publishes no host port; the connector reaches it at
`http://tethux-ci-viewer:8080`.

Run the `deploy-viewer` Woodpecker workflow manually, or push `master` after its
CI dependency succeeds. The first deployment intentionally starts without a
tunnel. Create a remotely managed Cloudflare Tunnel and configure its public
hostname service as `http://tethux-ci-viewer:8080`, then install the token
interactively on the NAS:

```console
ssh nas
/Containers/homelab/tethux-ci-viewer/tethux-ci deploy tunnel-token
```

Input is hidden. The token is written with mode `0600` below
`/Containers/homelab/tethux-ci-viewer/secrets`; it is never placed in Git, an
environment variable, a command argument, or a Woodpecker secret. Running the
deployment workflow again updates the viewer while preserving the database and
tunnel token.
