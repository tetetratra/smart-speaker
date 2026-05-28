FROM node:20-bookworm-slim

ARG GH_VERSION=2.74.2
ARG GO_VERSION=1.25.0

RUN apt-get update && apt-get install -y \
  bash \
  ca-certificates \
  curl \
  g++ \
  gcc \
  git \
  jq \
  libopus-dev \
  libopusfile-dev \
  make \
  openssh-client \
  pkg-config \
  ripgrep \
  tar \
  unzip \
  && rm -rf /var/lib/apt/lists/*

RUN set -eux; \
  arch="$(dpkg --print-architecture)"; \
  case "$arch" in \
    amd64) go_arch="amd64" ;; \
    arm64) go_arch="arm64" ;; \
    *) echo "unsupported architecture: $arch" >&2; exit 1 ;; \
  esac; \
  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${go_arch}.tar.gz" -o /tmp/go.tgz; \
  rm -rf /usr/local/go; \
  tar -C /usr/local -xzf /tmp/go.tgz; \
  rm -f /tmp/go.tgz

ENV PATH="/usr/local/go/bin:${PATH}"

RUN set -eux; \
  ln -sf /usr/local/go/bin/go /usr/local/bin/go; \
  ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt

RUN set -eux; \
  arch="$(dpkg --print-architecture)"; \
  case "$arch" in \
    amd64) gh_arch="amd64" ;; \
    arm64) gh_arch="arm64" ;; \
    *) echo "unsupported architecture: $arch" >&2; exit 1 ;; \
  esac; \
  curl -fsSL "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${gh_arch}.tar.gz" -o /tmp/gh.tgz; \
  tar -xzf /tmp/gh.tgz -C /tmp; \
  install "/tmp/gh_${GH_VERSION}_linux_${gh_arch}/bin/gh" /usr/local/bin/gh; \
  rm -rf /tmp/gh.tgz "/tmp/gh_${GH_VERSION}_linux_${gh_arch}"

RUN npm install -g @openai/codex

RUN curl https://cursor.com/install -fsS | bash

ENV PATH="/root/.local/bin:/home/node/.local/bin:/root/.cursor/bin:${PATH}"

WORKDIR /workspace
