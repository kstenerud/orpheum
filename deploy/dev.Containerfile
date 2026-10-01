# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: The development image (D235 §2-3): every tool the gates and the dev loop need, pinned by
# ABOUTME: version and checksum. `./dev` builds it and runs it; nothing here is meant to be run by hand.
#
# Stages:
#   fetch  downloads the pinned FFmpeg, MKVToolNix, golangci-lint and sqlc with the repository's
#          own fetch scripts, which verify each download's SHA-256 (one way to fetch them, AGENTS
#          rule 11).
#   base   Debian and the pinned FFmpeg: what the runtime image will share (D235 §2).
#   dev    base plus Go, Node, the gate tools, Playwright's system libraries and a user whose
#          uid and gid are the host's, so files written in the bind-mounted repository are the
#          developer's own.
#
# The build context is the repository root, or the subset of it `./dev` copies (the same relative
# paths), so `docker build -f deploy/dev.Containerfile .` from the root builds the same image.
#
# Every pin is written once. Go's version is go.mod's `toolchain` line, read at build time, and
# Node's major follows the Makefile's NODE_MAJOR; scripts/tests/test_dev_image.py fails when a
# checksum or Node drifts. golangci-lint's and sqlc's pins are scripts/fetch_tools.py's, so a
# Makefile edit rebuilds nothing here. Delve is not baked in: `make dev-debug` installs it on first
# use. Debian packages are verified by apt's signed archive, not pinned by version.

# debian:trixie-slim, the multi-arch index digest read from Docker Hub on 2026-09-30. Trixie is
# what devnas runs.
ARG DEBIAN_IMAGE=debian:trixie-slim@sha256:a99cfc517144bc59b1978475ec53b46ecabec7e43635402ee5b77cc54cd1b20a

# ---------------------------------------------------------------------------------------------
FROM ${DEBIAN_IMAGE} AS fetch
RUN apt-get update -q \
 && apt-get install -y -q --no-install-recommends ca-certificates python3 \
 && rm -rf /var/lib/apt/lists/*
# The fetch scripts keep what they fetch under ORPHEUM_TOOLS_DIR instead of the repository's
# .tools/, so the tools baked in here are the ones `make ffmpeg` and the dev loop find later.
ENV ORPHEUM_TOOLS_DIR=/opt/orpheum/tools
COPY scripts/fetch_ffmpeg.py scripts/fetch_mkvtoolnix.py /opt/orpheum/scripts/
# MKVToolNix is pinned for amd64 only; on arm64 its script prints a notice and fetches nothing.
RUN python3 /opt/orpheum/scripts/fetch_ffmpeg.py \
 && python3 /opt/orpheum/scripts/fetch_mkvtoolnix.py \
 && mkdir -p /opt/orpheum/tools/mkvtoolnix
# The gate tools, linked from /opt/orpheum/tools/bin, which the Makefile puts on PATH. A layer of
# its own, so a changed pin fetches only them again.
COPY scripts/fetch_tools.py /opt/orpheum/scripts/
RUN python3 /opt/orpheum/scripts/fetch_tools.py

# ---------------------------------------------------------------------------------------------
FROM ${DEBIAN_IMAGE} AS base
RUN apt-get update -q \
 && apt-get install -y -q --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/*
ENV ORPHEUM_TOOLS_DIR=/opt/orpheum/tools
COPY --from=fetch /opt/orpheum/tools/ffmpeg /opt/orpheum/tools/ffmpeg

# ---------------------------------------------------------------------------------------------
FROM base AS dev

# gcc and libc6-dev: cgo, which the race detector needs. xz-utils: Node's archive. shellcheck:
# `make check-scripts`. python3: the gate scripts.
RUN apt-get update -q \
 && apt-get install -y -q --no-install-recommends \
      bash curl gcc git libc6-dev make procps python3 shellcheck xz-utils \
 && rm -rf /var/lib/apt/lists/*

COPY --from=fetch /opt/orpheum/tools/mkvtoolnix /opt/orpheum/tools/mkvtoolnix

# Go: the version go.mod's `toolchain` line names, read from go.mod here so it is written nowhere
# else. Each checksum is a sha256sum line naming the archive it is for, with the SHA-256 go.dev/dl
# publishes; a toolchain bump without new checksums stops the build here and fails
# test_dev_image.py. GOTOOLCHAIN=local keeps Go from fetching another toolchain behind the pin.
ARG GO_SHA256_AMD64="990e6b4bbba816dc3ee129eaeaf4b42f17c2800b88a2166c265ac1a200262282  go1.26.2.linux-amd64.tar.gz"
ARG GO_SHA256_ARM64="c958a1fe1b361391db163a485e21f5f228142d6f8b584f6bef89b26f66dc5b23  go1.26.2.linux-arm64.tar.gz"
COPY go.mod /opt/orpheum/src/go.mod
RUN set -eu; arch="$(dpkg --print-architecture)"; \
    case "$arch" in \
      amd64) sum="$GO_SHA256_AMD64" ;; arm64) sum="$GO_SHA256_ARM64" ;; \
      *) echo "no pinned Go for $arch" >&2; exit 1 ;; \
    esac; \
    version="$(sed -n 's/^toolchain go//p' /opt/orpheum/src/go.mod)"; \
    [ -n "$version" ] || { echo "go.mod has no toolchain line to take Go's version from" >&2; exit 1; }; \
    archive="go${version}.linux-${arch}.tar.gz"; \
    [ "${sum##* }" = "$archive" ] || { \
      echo "go.mod wants $archive; the Containerfile's checksum is for ${sum##* }" >&2; exit 1; }; \
    curl -fsSL -o "/tmp/$archive" "https://go.dev/dl/$archive"; \
    (cd /tmp && echo "$sum" | sha256sum -c -); \
    tar -C /usr/local -xzf "/tmp/$archive"; rm "/tmp/$archive"
ENV PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local

# Node: the newest 22.x on 2026-09-30 (the Makefile pins the major only), with the SHA-256 from
# nodejs.org's SHASUMS256.txt.
ARG NODE_VERSION=22.23.3
ARG NODE_SHA256_AMD64=df450af89261115ef9f9e3830c3eeb2cc9213b63c720b1af623cb5dcbe2e02de
ARG NODE_SHA256_ARM64=a44aeb94849a299b22df10b9e622ec2f605c2183501bc40590705131de7c740f
RUN set -eu; arch="$(dpkg --print-architecture)"; \
    case "$arch" in \
      amd64) node_arch=x64; sum="$NODE_SHA256_AMD64" ;; arm64) node_arch=arm64; sum="$NODE_SHA256_ARM64" ;; \
      *) echo "no pinned Node for $arch" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /tmp/node.tar.xz \
      "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${node_arch}.tar.xz"; \
    echo "$sum  /tmp/node.tar.xz" | sha256sum -c -; \
    mkdir -p /opt/node; tar -C /opt/node --strip-components=1 -xJf /tmp/node.tar.xz; rm /tmp/node.tar.xz
ENV PATH=/opt/node/bin:$PATH

# Playwright's system libraries, installed by the pinned Playwright itself: the playwright-core
# tarball web/package-lock.json resolves, checked against the lockfile's sha512 integrity. The
# browsers are not baked in: `make browsers` fetches them into a volume (Playwright fetches them
# for its own version over HTTPS; nothing here checks their hashes, as nothing does today).
COPY web/package-lock.json /opt/orpheum/src/web/package-lock.json
RUN set -eu; \
    python3 -c 'import json; p = json.load(open("/opt/orpheum/src/web/package-lock.json"))["packages"]["node_modules/playwright-core"]; print(p["resolved"], p["integrity"])' > /tmp/playwright.pin; \
    read -r url integrity < /tmp/playwright.pin; \
    curl -fsSL -o /tmp/playwright-core.tgz "$url"; \
    got="$(python3 -c 'import base64, hashlib, sys; print("sha512-" + base64.b64encode(hashlib.sha512(open(sys.argv[1], "rb").read()).digest()).decode())' /tmp/playwright-core.tgz)"; \
    [ "$got" = "$integrity" ] || { echo "playwright-core: integrity is $got, the lockfile says $integrity" >&2; exit 1; }; \
    mkdir /tmp/playwright-core; tar -C /tmp/playwright-core -xzf /tmp/playwright-core.tgz; \
    node /tmp/playwright-core/package/cli.js install-deps chromium firefox; \
    rm -rf /tmp/playwright* /var/lib/apt/lists/*

# The gate tools, last of the tools, so a changed pin rebuilds nothing above.
COPY --from=fetch /opt/orpheum/tools/golangci-lint /opt/orpheum/tools/golangci-lint
COPY --from=fetch /opt/orpheum/tools/sqlc /opt/orpheum/tools/sqlc
COPY --from=fetch /opt/orpheum/tools/bin /opt/orpheum/tools/bin

# The developer: the host's uid and gid (`./dev` passes them), so the repository's files stay
# theirs. Their caches live in named volumes mounted over these directories; a new volume takes
# the directory's owner from here.
ARG DEV_UID=1000
ARG DEV_GID=1000
RUN set -eu; \
    getent group "$DEV_GID" >/dev/null || groupadd --gid "$DEV_GID" dev; \
    useradd --uid "$DEV_UID" --gid "$DEV_GID" --create-home --home-dir /home/dev --shell /bin/bash dev; \
    mkdir -p /home/dev/go /home/dev/.cache /home/dev/.npm; \
    chown -R "$DEV_UID:$DEV_GID" /home/dev
USER dev
# ORPHEUM_IN_DEV_IMAGE tells the git and Claude Code hooks they are inside this image, where the
# gate runs directly rather than through `./dev` (scripts/run_gate.py).
ENV HOME=/home/dev GOPATH=/home/dev/go ORPHEUM_IN_DEV_IMAGE=1
WORKDIR /home/dev
