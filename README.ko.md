# tmiDB: 모든 정보를 실시간으로 연결하는 데이터베이스

**tmiDB(Too-Much-Information Database)**는 세상의 모든 **대상(Target)**에 대한 정보를 실시간으로 수집, 관리, 스트리밍하기 위해 태어난 새로운 개념의 데이터베이스입니다.

PostgreSQL, TimescaleDB, PostgREST 등 검증된 오픈소스를 기반으로 구축되어 안정성과 확장성을 모두 갖추었으며, MIT/Apache-2.0 등 퍼미시브 라이선스만으로 구성되어 어떤 상용 서비스에도 제약 없이 사용할 수 있습니다.

## 핵심 컨셉

tmiDB는 세 가지 핵심 요소로 동작합니다.

- **대상 (Target)**: 정보 수집의 중심이 되는 모든 개체입니다. 사람이 될 수도, IoT 장비, 특정 서비스, 혹은 추상적인 개념일 수도 있습니다. `target_id`를 통해 고유하게 식별됩니다.

- **스피커 (Speaker)**: '대상'에 대한 정보를 생성하고 tmiDB에 기록하는 모든 주체입니다. REST API를 통한 직접적인 데이터 전송, DB 쿼리 실행, 심지어 특정 URL을 등록해 tmiDB가 주기적으로 정보를 가져오는 '자동화된 스피커'까지 가능합니다.

- **리스너 (Listener)**: '대상'의 정보를 구독하고 실시간으로 업데이트를 받는 주체입니다. 특정 대상에 대한 '리스너'로 등록되면, 해당 대상의 데이터에 접근할 수 있는 API 엔드포인트가 자동으로 생성되며, WebSocket을 통해 모든 변경사항을 실시간으로 스트리밍 받을 수 있습니다.

## 주요 특징

- **유연한 동적 스키마**: MongoDB처럼, 대상에 대한 속성(예: 컬럼)을 미리 정의할 필요가 없습니다. `meta` 필드를 통해 어떤 형태의 정보든 자유롭게 추가, 변경할 수 있으며, 데이터 구조가 변경될 때마다 내부적으로 버전이 관리됩니다.

- **자동화된 API 생성 및 버전 관리**: 데이터 스키마가 변경되면, PostgREST를 통해 REST API가 즉시 자동으로 업데이트되고 버전이 부여됩니다. 이를 통해 데이터 모델과 API가 항상 일관성을 유지하며, gRPC처럼 안정적인 서비스 운영이 가능합니다.

- **강력한 시계열 및 위치 기반 데이터 처리**: TimescaleDB를 하이퍼테이블로 활용하여 대용량 시계열 데이터를 효율적으로 처리하고, PostGIS의 `cube`와 `earthdistance`를 통해 복잡한 위치 기반 질의(반경 검색, 근접 분석 등)를 손쉽게 수행할 수 있습니다.

- **실시간 스트리밍**: Supabase Realtime 엔진을 통해 데이터베이스의 모든 변경사항(INSERT, UPDATE, DELETE)을 리스너에게 즉시 전달합니다.

## 시작하기

```bash
# Docker Compose로 모든 서비스를 한번에 실행합니다.
docker-compose up -d

# 새로운 '대상'을 생성합니다.
curl -X POST http://localhost:3000/target \
  -H "Content-Type: application/json" \
  -d '{ "name": "server-01", "meta": { "ip": "192.168.0.10", "status": "online" } }'

# 'server-01'의 CPU 사용률 데이터를 추가합니다. (Speaker 역할)
curl -X POST http://localhost:3000/ts_obs \
    -H "Content-Type: application/json" \
    -d '{ "target_id": "<server-01의 target_id>", "payload": { "cpu_usage": 15.5 } }'

# WebSocket 클라이언트로 'server-01'의 모든 변경사항을 실시간으로 구독합니다. (Listener 역할)
# (자세한 방법은 문서의 Realtime 섹션을 참고하세요.)
```

tmiDB는 복잡하게 얽힌 데이터를 명확하게 연결하고, 살아있는 정보의 흐름을 만드는 가장 강력한 도구가 될 것입니다.
