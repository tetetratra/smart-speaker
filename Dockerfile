FROM golang:1.25-bookworm AS base

WORKDIR /app

RUN apt-get update && apt-get install -y \
  pkg-config \
  libopus-dev \
  libopusfile-dev \
  ca-certificates \
  && rm -rf /var/lib/apt/lists/*

FROM node:20-bookworm AS frontend

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY web ./web
RUN npm run build

FROM base AS dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .
CMD ["go", "run", "./cmd/smart-speaker"]

FROM base AS build

COPY go.mod go.sum ./
RUN go mod download

# Go のビルドに要るものだけを取り込み、手元の作業用ファイルをビルドキャッシュに残さない。
COPY cmd ./cmd
COPY internal ./internal
RUN go build -o /app/bin/smart-speaker ./cmd/smart-speaker

FROM debian:bookworm-slim AS runtime

RUN apt-get update && apt-get install -y \
  libopus0 \
  libopusfile0 \
  ca-certificates \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=build /app/bin/smart-speaker /app/bin/smart-speaker
COPY --from=frontend /app/web/dist /app/web/dist
# config.yaml は任意。無い場合はアプリケーション側の既定値を使う
# （ワイルドカードにすると、他に一致するファイルがあればCOPYが失敗しないため、config.sample.yaml と一緒にコピーする）
COPY config.sample.yaml config.yam[l] /app/

EXPOSE 8081
CMD ["/app/bin/smart-speaker"]
