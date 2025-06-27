# tmidb-core/Dockerfile - All-in-One Production Build
# 이 파일은 Go 애플리케이션과 모든 종속 서비스(PostgreSQL, NATS, SeaweedFS)를
# 포함하는 단일 프로덕션 이미지를 빌드합니다.

# 1. Builder Stage: Go 애플리케이션 컴파일
FROM golang:1.24-bullseye AS builder
LABEL maintainer="poi"
LABEL version="0.1-all-in-one"

WORKDIR /app

# 의존성 파일을 먼저 복사하고 다운로드하여 Docker 캐시를 활용합니다.
COPY go.mod go.sum ./
RUN go mod download

# 전체 소스 코드를 복사합니다.
COPY . .

# API 서버 하나만 빌드합니다. 이 서버가 다른 프로세스를 관리합니다.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/tmidb-core ./cmd/api


# 2. Final Stage: 모든 서비스가 포함된 프로덕션 이미지 생성
FROM debian:bullseye-slim

# 필요한 패키지 설치: sudo(권한 관리), postgresql, curl(다운로드) 등
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    gnupg \
    locales \
    lsb-release \
    sudo \
    && rm -rf /var/lib/apt/lists/*

# 로케일 설정
RUN echo "en_US.UTF-8 UTF-8" > /etc/locale.gen && \
    locale-gen en_US.UTF-8 && \
    update-locale LANG=en_US.UTF-8
ENV LANG en_US.UTF-8
ENV LANGUAGE en_US:en
ENV LC_ALL en_US.UTF-8

# PostgreSQL 15 리포지토리 추가 및 설치
RUN curl -s https://www.postgresql.org/media/keys/ACCC4CF8.asc | gpg --dearmor | tee /etc/apt/trusted.gpg.d/postgresql.gpg >/dev/null && \
    echo "deb http://apt.postgresql.org/pub/repos/apt/ $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list && \
    apt-get update && \
    apt-get install -y --no-install-recommends postgresql-15 && \
    rm -rf /var/lib/apt/lists/*

# PostgreSQL 데이터 디렉토리 초기화
RUN rm -rf /var/lib/postgresql/15/main/* && \
    mkdir -p /var/lib/postgresql/15/main && \
    chown -R postgres:postgres /var/lib/postgresql/15/main && \
    sudo -u postgres /usr/lib/postgresql/15/bin/initdb -D /var/lib/postgresql/15/main --encoding=UTF8 --locale=en_US.UTF-8

# TimescaleDB 설치 (PostgreSQL 15용) - packagecloud.io 공식 절차
RUN mkdir -p /etc/apt/keyrings && \
    curl -fsSL https://packagecloud.io/timescale/timescaledb/gpgkey | gpg --dearmor -o /etc/apt/keyrings/timescaledb.gpg && \
    echo "deb [signed-by=/etc/apt/keyrings/timescaledb.gpg] https://packagecloud.io/timescale/timescaledb/debian/ $(lsb_release -cs) main" > /etc/apt/sources.list.d/timescaledb.list && \
    apt-get update && \
    apt-get install -y --no-install-recommends timescaledb-2-postgresql-15 && \
    rm -rf /var/lib/apt/lists/*

# NATS 서버 설치
RUN curl -L https://github.com/nats-io/nats-server/releases/download/v2.10.21/nats-server-v2.10.21-linux-amd64.tar.gz | tar -xz && \
    mv nats-server-v2.10.21-linux-amd64/nats-server /usr/local/bin/

# SeaweedFS 설치
RUN curl -L https://github.com/seaweedfs/seaweedfs/releases/download/3.68/linux_amd64.tar.gz | tar -xz && \
    mv weed /usr/local/bin/

# 보안을 위해 non-root 사용자 생성 및 sudo 권한 부여
RUN adduser --system --group appuser && \
    echo "appuser ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers

WORKDIR /app

# Go 애플리케이션 및 관련 파일 복사
COPY --from=builder /app/tmidb-core .
COPY views ./views
# COPY public ./public # public 디렉토리가 있다면 주석 해제

# 데이터 디렉토리 생성 및 권한 설정
RUN mkdir -p /data/seaweedfs /data/nats /var/run/postgresql && \
    chown -R appuser:appuser /app /data && \
    chown -R postgres:postgres /var/lib/postgresql /var/run/postgresql

USER appuser

# 컨테이너 시작 시 실행될 Go 애플리케이션
# 이 Go 애플리케이션은 내부적으로 DB, NATS, SeaweedFS를 자식 프로세스로 실행해야 합니다.
CMD ["/app/tmidb-core"]
