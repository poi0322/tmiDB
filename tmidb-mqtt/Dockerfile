# Dockerfile for tmidb-mqtt
FROM golang:1.24-alpine AS builder
LABEL version="0.1"
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/ingestor .

# mqtt:
  #   image: emqx:5
  #   restart: always
  #   ports: ["${TMIDB_MQTT_PORT:-1883}:1883", "${TMIDB_MQTT_WS_PORT:-8083}:8083"]
  #   volumes: ["emqx_data:/opt/emqx/data", "emqx_log:/opt/emqx/log"]
  #   healthcheck:
  #     test: ["CMD", "/bin/sh", "-c", "emqx_ctl status"]


FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /bin/ingestor /bin/
CMD ["ingestor"] 