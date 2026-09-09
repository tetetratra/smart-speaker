# smart-speaker の AI 検証で gofmt / go test を実行できるようにする。
# ai-driven-workflow の汎用 runner に含まれる基本ツールも維持する。
FROM node:20-bookworm-slim AS node

FROM golang:1.25-bookworm

ARG GH_VERSION=2.74.2

COPY --from=node /usr/local /usr/local

RUN apt-get update && apt-get install -y --no-install-recommends \
  bash \
  ca-certificates \
  curl \
  git \
  jq \
  openssh-client \
  ripgrep \
  tar \
  unzip \
  && rm -rf /var/lib/apt/lists/*

RUN ln -s /usr/local/go/bin/go /usr/local/bin/go \
  && ln -s /usr/local/go/bin/gofmt /usr/local/bin/gofmt

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

RUN ln -s /root/.local/bin/agent /usr/local/bin/agent

ENV PATH="/usr/local/go/bin:/root/.local/bin:/home/node/.local/bin:/root/.cursor/bin:${PATH}"

WORKDIR /workspace
