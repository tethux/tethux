# CI results viewer

This is an internal tool for inspecting the SQLite databases produced by the
Tethux CI-results pipeline. It is intended for local development and debugging,
not as a public or hosted service.

The Go server exposes a read-only results API and embeds the static Svelte
frontend. The viewer includes run, test, and artifact pages plus a SQL explorer
with schema completion, configurable result summaries, row details, nested JSON
inspection, and persistent light/dark themes.

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

Once this change is pushed, the viewer source will be available at:

https://github.com/tethux/tethux/tree/master/tools/ci-results/viewer
