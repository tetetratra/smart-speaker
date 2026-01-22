FROM --platform=linux/arm64 golang:1.24-bookworm

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
    https://alphacephei.com/vosk/models/vosk-model-small-ja-0.22.zip && \
  unzip /opt/vosk/model.zip -d /opt/vosk && \
  mv /opt/vosk/vosk-model-small-ja-0.22 /opt/vosk/model

ENV VOSK_PATH=/opt/vosk/runtime
ENV LD_LIBRARY_PATH=/opt/vosk/runtime
ENV CGO_CPPFLAGS="-I /opt/vosk/runtime"
ENV CGO_LDFLAGS="-L /opt/vosk/runtime -lvosk"
ENV VOSK_MODEL_PATH=/opt/vosk/model

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /app/bin/smart-speaker ./cmd/smart-speaker

EXPOSE 8081
CMD ["/app/bin/smart-speaker"]
