# Builder image (Slice 1): pinned, non-root, network-restricted by default.
FROM golang:1.27.1-bookworm AS build
WORKDIR /src
# (Later phases copy the tiny-package build here; Slice 1 builds bytes locally for determinism.)
RUN useradd -m builder
USER builder
