FROM docker.io/oven/bun:1-alpine AS web
WORKDIR /src/tools/ci-results/viewer/frontend
COPY tools/ci-results/viewer/frontend/package.json tools/ci-results/viewer/frontend/bun.lock ./
RUN bun install --frozen-lockfile
COPY tools/ci-results/viewer/frontend ./
RUN bun run build

FROM docker.io/library/golang:1.26-alpine AS build
RUN apk add --no-cache build-base libpcap-dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/tools/ci-results/viewer/frontend/build ./tools/ci-results/viewer/frontend/build
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/ci-results ./tools/ci-results

FROM docker.io/library/alpine:3.23
RUN apk add --no-cache ca-certificates libpcap && addgroup -S viewer && adduser -S -G viewer viewer
USER viewer
ENTRYPOINT ["/usr/local/bin/ci-results"]
COPY --from=build /out/ci-results /usr/local/bin/ci-results
