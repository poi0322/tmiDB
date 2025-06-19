FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod ./
COPY go.sum* ./
RUN go mod download

COPY . .
RUN go mod tidy
RUN go build -o tmidb-proxy main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/tmidb-proxy .

CMD ["./tmidb-proxy"]
