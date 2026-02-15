FROM golang:1.24-bookworm AS base

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

COPY . .
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

EXPOSE 8081
CMD ["/app/bin/smart-speaker"]
