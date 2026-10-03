# Quorum worker image: runs ./apps/worker. Needs the Docker socket to
# perform network-isolated rebuilds (DockerArchiveDigest) — mount
# /var/run/docker.sock when running (see compose `worker` profile).
FROM golang:1.27.1-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/quorum-worker ./apps/worker

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
  git ca-certificates curl docker.io \
  && rm -rf /var/lib/apt/lists/* \
  && useradd -m quorum
COPY --from=build /out/quorum-worker /usr/local/bin/quorum-worker
USER quorum
ENTRYPOINT ["/usr/local/bin/quorum-worker"]
CMD ["--api", "http://api:8080", "--owner", "worker-1"]
