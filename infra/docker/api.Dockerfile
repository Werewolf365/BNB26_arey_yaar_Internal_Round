# Quorum API image: builds ./apps/api from the repo root.
FROM golang:1.27.1-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/quorum-api ./apps/api

FROM debian:bookworm-slim
RUN useradd -m quorum && mkdir -p /data/evidence && chown quorum:quorum /data/evidence
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
  && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/quorum-api /usr/local/bin/quorum-api
USER quorum
ENV QUORUM_ADDR=:8080 QUORUM_BLOB_DIR=/data/evidence
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=10 \
  CMD curl -sf http://localhost:8080/api/v1/health || exit 1
ENTRYPOINT ["/usr/local/bin/quorum-api"]
