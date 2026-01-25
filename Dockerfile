FROM --platform=linux/arm64 golang:1.24-bookworm AS base

WORKDIR /app

RUN apt-get update && apt-get install -y \
  pkg-config \
  libopus-dev \
  libopusfile-dev \
  unzip \
  curl \
  ca-certificates \
  && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/vosk && \
  curl -L -o /opt/vosk/vosk-linux.zip \
    https://github.com/alphacep/vosk-api/releases/download/v0.3.45/vosk-linux-aarch64-0.3.45.zip && \
  unzip /opt/vosk/vosk-linux.zip -d /opt/vosk && \
  mv /opt/vosk/vosk-linux-aarch64-0.3.45 /opt/vosk/runtime

RUN curl -L -o /opt/vosk/model.zip \
    https://alphacephei.com/vosk/models/vosk-model-ja-0.22.zip && \
  unzip /opt/vosk/model.zip -d /opt/vosk && \
  mv /opt/vosk/vosk-model-ja-0.22 /opt/vosk/model

ENV VOSK_PATH=/opt/vosk/runtime
ENV LD_LIBRARY_PATH=/opt/vosk/runtime
ENV CGO_CPPFLAGS="-I /opt/vosk/runtime"
ENV CGO_LDFLAGS="-L /opt/vosk/runtime -lvosk"
ENV VOSK_MODEL_PATH=/opt/vosk/model

FROM --platform=linux/arm64 node:20-bookworm AS frontend

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

FROM gcr.io/distroless/base-debian12 AS runtime

WORKDIR /app

COPY --from=base /opt/vosk /opt/vosk
COPY --from=build /app/bin/smart-speaker /app/bin/smart-speaker
COPY --from=frontend /app/web/dist /app/web/dist

ENV VOSK_PATH=/opt/vosk/runtime
ENV LD_LIBRARY_PATH=/opt/vosk/runtime
ENV VOSK_MODEL_PATH=/opt/vosk/model

EXPOSE 8081
CMD ["/app/bin/smart-speaker"]
