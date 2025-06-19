# tmiDB — Too-Much-Information DataBase

**MIT / Apache-2 / BSD 퍼미시브 라이선스 전용 설계서 (v0.1)**

---

## 1. 구성 요소 & 라이선스

| 계층 | 컴포넌트 | 라이선스 | 비고 |⸻

## 8. 초기 로드맵

| 주  | 목표                               |
| --- | ---------------------------------- |
| 1   | 레포·Compose 기동 / DB 초기 스키마 |
| 2   | pollingd → raw_bucket INSERT       |
| 3   | normsvc AI 매핑 → 정규 테이블      |
| 4   | Realtime WS / stream_def 적용      |
| 5   | alarm_engine 임계값 알람           |
| 6   | 콘솔 MVP·tmictl CLI 배포           |
| 7   | **A-방식 래퍼 패키징 & 브랜딩**    |

⸻

## 9. 라이선스 파일---|---------|------|

| **DB 엔진** | PostgreSQL 15 | PostgreSQL (BSD) | 기반 RDB |
| **시계열** | TimescaleDB 2.18 **코어** | Apache-2.0 | `compress_chunks` 까지만 사용 |
| **공간 / 거리** | `cube` + `earthdistance` | BSD | 반경·근접 질의 |
| **객체 스토리지** | SeaweedFS S3 Gateway | Apache-2.0 | MinIO 대체 |
| **스트림** | Supabase Realtime | MIT | logical decoding WS |
| **REST** | PostgREST | MIT | 스키마→REST |
| **메시지 버스** | NATS JetStream | Apache-2.0 | ETL·알람 |
| **ETL / AI** | pollingd · normsvc · alarm_engine (Go) | MIT | 내부 코드 |
| **ONNX Runtime** | MIT | AI 추론 |

> **GPL/AGPL 전혀 없음** — 상용/SaaS 재배포 100 % 안전.

---

## 2. 핵심 개념

tmiDB는 특정 **대상(Target)**에 대한 모든 정보를 중앙에서 관리하고, 실시간으로 스트리밍하기 위해 설계된 데이터베이스입니다.

- **대상 (Target)**: 정보를 수집하고 관리할 기본 단위입니다. 사람, 장비, 서비스 등 모든 것이 대상이 될 수 있습니다.
- **스피커 (Speaker)**: 대상에 대한 정보를 생성하고 데이터베이스에 쓰는 주체입니다. API 호출, 직접적인 DB 쿼리, 또는 tmiDB가 특정 URL에서 데이터를 주기적으로 스크랩하는 자동화된 에이전트 등 다양한 형태가 될 수 있습니다.
- **리스너 (Listener)**: 특정 대상의 정보를 구독하는 주체입니다. 대상에 대한 읽기 권한을 부여받으면, 자동 생성된 REST API 엔드포인트에 접근할 수 있으며, WebSocket을 통해 정보 업데이트를 실시간으로 스트리밍 받을 수 있습니다.

### 주요 특징

- **동적 스키마**: MongoDB처럼 대상에 대한 정보 항목(컬럼)을 미리 정의할 필요 없이 자유롭게 추가할 수 있습니다. 정보 스키마가 변경될 때마다 내부적으로 버전이 관리됩니다.
- **자동화된 API 생성 및 버전 관리**: 대상의 데이터 구조가 변경되면, 그에 맞춰 REST API가 자동으로 업데이트되고 버전이 부여됩니다. 이는 gRPC와 유사한 방식으로 API 호환성을 보장합니다.
- **유연한 데이터 수집**: Speaker는 DB 쿼리, REST API 등 다양한 방식으로 데이터를 입력할 수 있습니다. 또한, 특정 웹 페이지의 URL을 등록하면 tmiDB 내부의 코드가 주기적으로 해당 페이지의 정보를 수집하여 저장하는 기능도 제공합니다.

---

## 3. 핵심 테이블

```sql
-- 대상 (Target): 정보를 수집할 주체. 사람, 사물, 서비스 등 모든 것이 될 수 있습니다.
CREATE TABLE target(
  target_id UUID PRIMARY KEY,
  name TEXT,
  meta JSONB, -- 동적 속성
  created_at TIMESTAMPTZ DEFAULT now()
);

-- 시계열 관측 데이터 (Hypertable)
CREATE TABLE ts_obs(
  obs_id BIGSERIAL PRIMARY KEY,
  target_id UUID NOT NULL REFERENCES target(target_id),
  ts TIMESTAMPTZ NOT NULL,
  payload JSONB -- 측정된 모든 데이터를 JSON으로 저장
);
SELECT create_hypertable('ts_obs', 'ts',
  partitioning_column => 'target_id', number_partitions => 8);

-- 위치 추적 데이터
CREATE EXTENSION IF NOT EXISTS cube;
CREATE EXTENSION IF NOT EXISTS earthdistance;
CREATE TABLE geo_trace(
  target_id UUID,
  ts TIMESTAMPTZ,
  lon DOUBLE PRECISION, lat DOUBLE PRECISION,
  PRIMARY KEY(target_id, ts),
  FOREIGN KEY (target_id) REFERENCES target(target_id)
);
CREATE INDEX idx_geo_gist
  ON geo_trace USING gist (ll_to_earth(lat, lon));

-- 원본 수집
CREATE TABLE raw_bucket(
  raw_id BIGSERIAL PRIMARY KEY,
  ts TIMESTAMPTZ DEFAULT now(),
  payload JSONB
);

-- 버전·커넥터·매핑 메타 (요약)
CREATE TABLE model_versions(...);
CREATE TABLE ingest_connector(...);
CREATE TABLE mapping_rule(...);
```

⸻

3. 아키텍처

graph TD
UI[🌐 tmiDB Console] --> PGRST
UI --> RT
CLI[tmictl] --> DB

subgraph Services
CONNECT[🔌 pollingd] --> NATS
NORM[🔧 normsvc] --> DB
ALARM[🚨 alarm_engine] --> DB
PGRST[PostgREST] --> DB
RT[Realtime] --> DB
end

subgraph Core
DB[(PostgreSQL + Timescale<br>+ cube/earthdistance)]
S3[(SeaweedFS S3)]
NATS[(NATS JetStream)]
end

S3 --> DB
DB --> NATS

⸻

4. Docker Compose(요약)

services:
db:
image: timescale/timescaledb:2.18-pg15
environment:
POSTGRES_PASSWORD: postgres
command: >
postgres -c timescaledb.telemetry_level=off
-c timescaledb.license='apache'
volumes: [db_data:/var/lib/postgresql/data]
ports: ["5432:5432"]

seaweed:
image: seaweedfs/seaweedfs:3.67
command: "server -s3 -dir=/data -volume.max=0"
volumes: [weed_data:/data]
ports: ["8333:8333","9333:9333"]

nats:
image: nats:2.10-alpine
command: "--jetstream --store_dir=/data"
volumes: [nats_data:/data]
ports: ["4222:4222"]

postgrest:
image: postgrest/postgrest:v12
environment:
PGRST_DB_URI: postgres://postgres:postgres@db:5432/postgres
PGRST_DB_ANON_ROLE: postgres
ports: ["3000:3000"]

realtime:
image: supabase/realtime:v2.30
environment:
DB_HOST: db
DB_USER: postgres
DB_PASSWORD: postgres
ports: ["4000:4000"]

volumes:
db_data:
weed_data:
nats_data:

⸻

5. 초기 로드맵

주 목표
1 레포·Compose 기동 / DB 초기 스키마
2 pollingd → raw_bucket INSERT
3 normsvc AI 매핑 → 정규 테이블
4 Realtime WS / stream_def 적용
5 alarm_engine 임계값 알람
6 콘솔 MVP·tmictl CLI 배포

⸻

6. 라이선스 파일

LICENSE → MIT
Each Go/TS file → SPDX-License-Identifier: MIT

tmiDB 전체 스택은 MIT/Apache/BSD 만을 사용하여,
코드 포크·상용 판매·SaaS 배포 모두 제한 없이 가능하다.

---

## 7. A-방식 래퍼 브랜딩 전략

**tmiDB 브랜드를 전면에 노출시키는 체크리스트**

| 화면·명령어                           | 기본 PostgreSQL 동작            | tmiDB 표시 방법                                        | 실제 구현 지점                                                |
| ------------------------------------- | ------------------------------- | ------------------------------------------------------ | ------------------------------------------------------------- |
| systemd 서비스                        | `postgresql.service`            | `tmidbd.service`<br>Description= tmiDB Database Engine | 패키지 post-install 스크립트에서<br>`systemctl enable tmidbd` |
| 프로세스 목록<br>`ps aux`             | `postgres … -D /var/lib/…`      | `tmidbd … -D /var/lib/tmidb/data`<br>(런처 이름)       | `/usr/bin/tmidbd` → `exec postgres`                           |
| psql 프롬프트                         | `postgres=#`                    | `tmiDB:postgres=#`                                     | `ALTER SYSTEM SET prompt1='tmiDB:%/%R%# ';`                   |
| pg_stat_activity.<br>application_name | 빈값 또는 psql                  | tmiDB-Core (서버)<br>tmiDB-CLI (클라이언트)            | 런처에서 PGAPPNAME ENV 주입                                   |
| 로그·배너                             | `LOG: database system is ready` | `=== tmiDB 0.1 ready ===`                              | entrypoint 첫 줄 echo                                         |
| REST Swagger 제목                     | "PostgREST API"                 | "tmiDB API"                                            | `PGRST_OPENAPI_TITLE=tmiDB API` ENV                           |
| Realtime WebSocket path               | `/realtime/v1`                  | `/tmi/ws` (reverse-proxy 리라이트)                     | Nginx rewrite or RT `WS_PATH` ENV                             |
| 콘솔 웹 UI 로고                       | Supabase 로고                   | tmiDB 로고 PNG/SVG                                     | React `src/assets/logo.svg` 교체                              |
| CLI                                   | 없음                            | `tmictl` (install / status / backup)                   | Go Cobra 바이너리                                             |

### 구현 순서

**1. 런처 바이너리 (Go 140 줄)**

```go
cmd := exec.Command(
    "/opt/tmidb/bin/postgres",
    "-D", "/opt/tmidb/data",
    "-c", "config_file=/etc/tmidb/tmi_db.conf",
)
cmd.Env = append(os.Environ(), "PGAPPNAME=tmiDB-Core")
cmd.Run()
```

**2. systemd 유닛 파일** `/usr/lib/systemd/system/tmidbd.service`

```ini
[Unit]
Description=tmiDB Database Engine
After=network.target

[Service]
Type=simple
ExecStart=/usr/bin/tmidbd start
ExecStop=/usr/bin/tmidbd stop
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

**3. psql 프롬프트 / 로그 배너**

```bash
echo "ALTER SYSTEM SET prompt1='tmiDB:%/%R%# ';" | psql -U postgres
echo "=== tmiDB $(tmictl version) ready ===" >> /opt/tmidb/log/startup.log
```

**4. PostgREST / Realtime 환경변수**

```bash
export PGRST_OPENAPI_TITLE="tmiDB API"
export PGRST_OPENAPI_SERVER_PROXY_URI="https://api.tmidb.io"
export WS_PATH="/tmi/ws"
```

**5. 웹 콘솔**

- `APP_TITLE = 'tmiDB Console'`
- favicon & logo 교체 → `public/favicon.ico`, `logo.svg`

**6. 패키지 네이밍**

- `.deb` → `tmidb-engine_0.1.0_amd64.deb`
- 패키지 설명: "tmiDB Core (PostgreSQL-based) Database Server"

### 결과 UX 스냅샷

```bash
$ sudo systemctl status tmidbd
● tmidbd.service - tmiDB Database Engine
   Active: active (running) since Fri 2025-06-20 10:12:05 KST

$ psql -h localhost -U postgres
tmiDB:postgres=# SHOW shared_buffers;
 shared_buffers
───────────────
 4GB
(1 row)
```

**웹 콘솔 상단** ⇒ tmiDB Console / **Swagger** ⇒ tmiDB API / **WS URL** ⇒ `wss://api.tmidb.io/tmi/ws`

### 요약

- **래퍼-패키지(A) 그대로 두면서도**, 서비스명·프롬프트·로그·UI를 일괄 덮어써 "진짜 tmiDB 전용 엔진" 인상을 완벽히 준다.
- **Postgres 업스트림 머지 0 줄, 라이선스 위험 0** — 유지보수 난도는 MySQL-식 패키지 수준.

⸻

## 8. 초기 로드맵

| 주  | 목표                               |
| --- | ---------------------------------- |
| 1   | 레포·Compose 기동 / DB 초기 스키마 |
| 2   | pollingd → raw_bucket INSERT       |
| 3   | normsvc AI 매핑 → 정규 테이블      |
| 4   | Realtime WS / stream_def 적용      |
| 5   | alarm_engine 임계값 알람           |
| 6   | 콘솔 MVP·tmictl CLI 배포           |
| 7   | **A-방식 래퍼 패키징 & 브랜딩**    |

⸻

## 10. 확장 로드맵 (v0.2+)

| 기능                   | 설명                                                                                                                    | 핵심 기술                           |
| ---------------------- | ----------------------------------------------------------------------------------------------------------------------- | ----------------------------------- |
| **웹 기반 대시보드**   | 수집된 시계열(`ts_obs`), 위치(`geo_trace`) 데이터를 그래프와 지도로 시각화하고, 특정 Target의 정보를 모니터링하는 웹 UI | React/SvelteKit, ECharts/Leaflet.js |
| **고급 인증/인가**     | 대상(Target)별, 리스너(Listener)별로 세분화된 접근 제어(RBAC/ABAC)                                                      | Open Policy Agent, Casbin           |
| **서버리스 코드 실행** | 특정 이벤트(예: 데이터 수신) 발생 시, 사용자가 등록한 코드를 실행하여 데이터를 변환, 가공, 알림 전송                    | Deno/Wasm, NATS Functions           |
| **분산 쿼리**          | 여러 tmiDB 인스턴스를 클러스터로 묶어 대규모 데이터셋에 대한 통합 쿼리 실행                                             | Citus, Trino                        |
| **백업 및 복구 CLI**   | `tmictl backup`, `tmictl restore` 명령어를 통한 손쉬운 데이터 백업 및 복구                                              | pg_dump, pg_restore, Restic         |

⸻

## 11. 고급 아키텍처 패턴

tmiDB의 기본 구성 요소 위에 더 정교한 기능을 구현하기 위한 아키텍처 패턴입니다.

### 리스너별 동적 API 및 실시간 필터링

**문제:** 모든 리스너가 동일한 API 엔드포인트를 사용하는 것이 아니라, `api/리스너A`처럼 자신만의 엔드포인트를 갖고 구독한 데이터만 필터링해서 받고 싶다.

**해결책:** tmiDB 스택 앞에 **API 게이트웨이** 또는 **인증/프록시 서비스**를 배치합니다.

**동작 흐름:**

1.  **구독 정보 관리**: 리스너의 구독 정보(대상 ID, 필터링할 필드 등)를 `listener_subscriptions`와 같은 별도의 메타데이터 테이블에 저장합니다.
2.  **API 요청 처리 (Gateway)**:
    - 클라이언트가 `GET /api/listener_a`로 요청을 보냅니다.
    - API 게이트웨이는 `listener_a`의 구독 정보를 DB에서 조회합니다.
    - 게이트웨이는 조회된 정보를 바탕으로 실제 PostgREST에 보낼 쿼리(예: `GET /target?id=eq.X&select=meta->>field1,meta->>field2`)를 동적으로 생성하여 요청합니다.
    - PostgREST의 응답을 받아 클라이언트에게 최종 전달합니다.
3.  **실시간 메시지 필터링 (WebSocket Proxy)**:
    - 클라이언트가 `ws/listener_a`로 연결합니다.
    - 프록시 서비스는 해당 리스너가 구독 중인 대상의 Realtime 채널에 대신 연결합니다.
    - Realtime에서 오는 모든 변경사항 중, 리스너가 구독하기로 한 필드에 해당하는 내용만 선별하여 클라이언트에게 전달합니다.

### 대용량 파일 처리 (이벤트 알림 방식)

**문제:** 이미지, 동영상 등 대용량 파일을 WebSocket으로 직접 전송하는 것은 비효율적이다.

**해결책:** 파일은 S3 호환 스토리지(SeaweedFS)를 통해 전달하고, WebSocket으로는 파일이 변경되었다는 **이벤트 알림**만 전달합니다.

**동작 흐름:**

1.  **파일 업로드**: 스피커가 파일을 SeaweedFS(S3)에 업로드합니다.
2.  **메타데이터 저장**: 업로드가 완료되면, 파일의 위치, 이름, 크기 등의 메타데이터를 `ts_obs`나 별도의 `file_attachments` 테이블에 JSON 형태로 저장합니다.
3.  **이벤트 알림**:
    - 테이블에 새로운 레코드가 INSERT되면, Supabase Realtime이 이 변경 이벤트를 감지합니다.
    - Realtime은 파일 자체가 아닌, 파일이 업로드되었다는 사실과 그 메타데이터(JSON)를 리스너에게 실시간으로 전송합니다.
4.  **파일 다운로드**:
    - 리스너의 클라이언트는 WebSocket으로 이벤트 알림을 받습니다.
    - 클라이언트는 알림에 포함된 파일 경로(메타데이터)를 이용해 SeaweedFS(S3)에 직접 HTTP 요청을 보내 파일을 다운로드합니다.

이 패턴들을 통해 tmiDB의 핵심은 단순하게 유지하면서, 필요에 따라 확장성 있는 고급 기능을 구현할 수 있습니다.
