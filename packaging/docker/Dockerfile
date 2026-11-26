# syntax=docker/dockerfile:1

# Image of pcsm built from this checkout.
#
# The binary is built with the project's own `make build` and replaces /usr/bin/pcsm in the
# official image of the release this branch is based on. Nothing else in that image changes:
# same entrypoint, same user, same licence files.
#
# Build from the repository root:
#   docker buildx build -f packaging/docker/Dockerfile \
#     --build-arg GIT_COMMIT="$(git rev-parse --short HEAD)" \
#     --build-arg GIT_BRANCH="$(git rev-parse --abbrev-ref HEAD)" .

ARG BASE_IMAGE=percona/percona-clustersync-mongodb:1.1.0
ARG GO_VERSION=1.27.0

# Compiled on the build host and cross-compiled for the target, so no emulation is needed.
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS build

ARG TARGETOS
ARG TARGETARCH
ARG GIT_COMMIT=unknown
ARG GIT_BRANCH=unknown

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOARCH="$TARGETARCH" \
    make build GOOS="$TARGETOS" GITCOMMIT="$GIT_COMMIT" GITBRANCH="$GIT_BRANCH"

FROM ${BASE_IMAGE}

COPY --from=build --chmod=755 /src/bin/pcsm /usr/bin/pcsm
