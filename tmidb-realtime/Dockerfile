# Dockerfile for tmidb-realtime
FROM golang:1.24-alpine AS builder
LABEL version="0.1"
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/realtime .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /bin/realtime /bin/
CMD ["realtime"] 