# tmiDB 개발 환경 가이드

## 📋 개요

tmiDB는 Supervisor 기반 아키텍처로 설계되어 있으며, 개발 환경에서는 Air를 사용한 핫 리로드를 지원합니다.

## 🏗️ 아키텍처

```
tmidb-supervisor (메인 프로세스)
├── External Services
│   ├── PostgreSQL (포트 5432)
│   ├── NATS (포트 4222)
│   └── SeaweedFS (포트 9333)
└── Internal Components
    ├── API Server (포트 8080) - Air 핫 리로드
    ├── Data Manager - Air 핫 리로드
    └── Data Consumer - Air 핫 리로드
```

## 🚀 개발 환경 시작

### 1. Docker Compose로 시작

```bash
# 프로젝트 루트에서
docker compose -f docker-compose.dev.yml up
```

### 2. 로컬에서 직접 실행

```bash
# 의존성 설치
go mod download

# Supervisor 실행 (개발 모드)
export TMIDB_DEV_MODE=true
export TMIDB_HOT_RELOAD=true
export TMIDB_LOG_LEVEL=debug
go run ./cmd/supervisor
```

## 📊 로그 모니터링

### CLI를 통한 로그 확인

```bash
# 전체 로그 확인
./bin/tmidb-cli logs

# 실시간 로그 팔로우
./bin/tmidb-cli logs -f

# 특정 컴포넌트 로그
./bin/tmidb-cli logs api
./bin/tmidb-cli logs api -f

# 로그 상태 확인
./bin/tmidb-cli logs status
```

### 사용 가능한 컴포넌트

- `api` - API 서버
- `data-manager` - 데이터 관리자
- `data-consumer` - 데이터 소비자
- `postgresql` - PostgreSQL 데이터베이스
- `nats` - NATS 메시지 브로커
- `seaweedfs` - SeaweedFS 파일 시스템

## 🔧 프로세스 관리

### 프로세스 상태 확인

```bash
# 모든 프로세스 목록
./bin/tmidb-cli process list

# 특정 프로세스 상태
./bin/tmidb-cli process status api
```

### 프로세스 제어

```bash
# 프로세스 시작
./bin/tmidb-cli process start api

# 프로세스 정지
./bin/tmidb-cli process stop api

# 프로세스 재시작
./bin/tmidb-cli process restart api
```

## 📈 시스템 모니터링

```bash
# 시스템 리소스 모니터링 (실시간)
./bin/tmidb-cli monitor system

# 서비스 상태 확인
./bin/tmidb-cli monitor services

# 헬스 체크
./bin/tmidb-cli monitor health
```

## 🔥 핫 리로드 개발

### 개발 모드 환경 변수

```bash
export TMIDB_DEV_MODE=true        # 개발 모드 활성화
export TMIDB_HOT_RELOAD=true      # Air 핫 리로드 활성화
export TMIDB_LOG_LEVEL=debug      # 디버그 로그 레벨
```

### Air 설정 파일

각 컴포넌트별로 개별 Air 설정 파일이 있습니다:

- `.air.api.toml` - API 서버용
- `.air.data-manager.toml` - Data Manager용
- `.air.data-consumer.toml` - Data Consumer용

### 코드 변경 시 자동 재시작

개발 모드에서는 Go 코드를 수정하면 해당 컴포넌트가 자동으로 재컴파일되고 재시작됩니다.

## 🐳 Docker 개발 환경

### 볼륨 마운트

```yaml
volumes:
  - ./tmidb-core:/app:cached # 소스 코드 마운트
  - tmidb-data:/data # 데이터 영구 저장
  - tmidb-logs:/app/logs # 로그 영구 저장
```

### 포트 매핑

- `8080` - API Server
- `5432` - PostgreSQL
- `4222` - NATS
- `9333` - SeaweedFS Master
- `8333` - SeaweedFS Volume

### 컨테이너 내에서 개발

```bash
# 컨테이너 접속
docker compose -f docker-compose.dev.yml exec tmidb-core bash

# 컨테이너 내에서 CLI 사용
./bin/tmidb-cli logs -f
```

## 🛠️ 개발 팁

### 1. 로그 레벨 조정

```bash
# 디버그 로그 활성화
./bin/tmidb-cli logs enable api DEBUG

# 특정 컴포넌트 로그 비활성화
./bin/tmidb-cli logs disable postgresql
```

### 2. 개발 중 자주 사용하는 명령어

```bash
# 개발 환경 시작
docker compose -f docker-compose.dev.yml up -d

# 실시간 로그 모니터링
./bin/tmidb-cli logs -f

# API 서버만 재시작
./bin/tmidb-cli process restart api

# 시스템 상태 확인
./bin/tmidb-cli monitor health
```

### 3. 문제 해결

```bash
# 모든 프로세스 상태 확인
./bin/tmidb-cli process list

# 특정 프로세스 상세 정보
./bin/tmidb-cli process status api

# 시스템 리소스 확인
./bin/tmidb-cli monitor system
```

## 📝 로그 파일 위치

- 개발 환경: `./logs/`
- Docker 환경: `/app/logs/` (볼륨 마운트됨)

각 컴포넌트별로 개별 로그 파일이 생성되며, 로그 로테이션과 압축이 자동으로 수행됩니다.

## 🔗 유용한 링크

- API 문서: http://localhost:8080/docs
- NATS 모니터링: http://localhost:8222
- SeaweedFS 마스터: http://localhost:9333

## 🐛 디버깅

### 로그 수준별 필터링

```bash
# ERROR 레벨 이상만 표시
./bin/tmidb-cli logs api --level ERROR

# 특정 시간대 로그 확인
./bin/tmidb-cli logs api --since "2024-01-01 10:00:00"
```

### 프로세스 디버깅

```bash
# 프로세스 메모리 사용량 확인
./bin/tmidb-cli process status api

# 프로세스 재시작 기록 확인
./bin/tmidb-cli monitor services
```
