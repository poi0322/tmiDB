# `tmiDB-Core` 기능 청사진

이 문서는 `tmiDB-Core` 프로젝트의 목적, 구성 요소, 핵심 기능 및 데이터 흐름을 정의하는 기술 청사진입니다.

## 1. 개요

`tmiDB-Core`는 프로세스 관리 기반의 올인원 데이터베이스 시스템입니다. 단일 supervisor가 모든 구성요소를 자식 프로세스로 관리하며, CLI를 통해 각 구성요소의 로그를 통합 제어할 수 있습니다.

## 2. 핵심 아키텍처

### 2.1. 프로세스 관리 구조

```
tmidb-supervisor (메인 프로세스)
├── PostgreSQL + TimescaleDB (자식 프로세스)
├── NATS Server (자식 프로세스)
├── SeaweedFS (자식 프로세스)
├── api (자식 프로세스)
├── data-manager (자식 프로세스)
└── data-consumer (자식 프로세스)
```

### 2.2. 로그 관리 시스템

- **통합 로그**: 모든 구성요소의 로그가 supervisor를 통해 통합 출력
- **CLI 제어**: `tmidb-cli`를 통해 각 구성요소별 로그 on/off 제어
- **실시간 모니터링**: 각 프로세스의 상태와 로그를 실시간으로 관리

## 3. 핵심 구성 요소

### 3.1. tmidb-supervisor

**역할**: 전체 시스템의 프로세스 관리자

**기능**:

- 모든 외부 서비스(DB, NATS, SeaweedFS) 자식 프로세스로 시작/관리
- 핵심 구성요소(api, data-manager, data-consumer) 프로세스 관리
- 통합 로그 수집 및 출력
- 프로세스 헬스체크 및 자동 재시작
- CLI 명령 수신 및 처리

### 3.2. tmidb-cli

**역할**: 시스템 제어 CLI 도구

**기능**:

- 각 구성요소별 로그 켜기/끄기 제어
- 프로세스 상태 확인
- 시스템 설정 변경
- 실시간 로그 모니터링

**사용 예시**:

```bash
tmidb-cli logs enable api
tmidb-cli logs disable postgresql
tmidb-cli logs show data-manager
tmidb-cli status
```

### 3.3. api

**역할**: REST API 서버 및 웹 관리 콘솔

**기능**:

- 웹 관리 콘솔 제공 (Go html/template 기반)
- REST API 엔드포인트 노출
- 사용자 인증 및 인가 (세션 + Bearer 토큰)
- 데이터베이스 직접 연동

### 3.4. data-manager

**역할**: 데이터 수집 및 관리

**기능**:

- 외부 데이터 소스로부터 데이터 수집
- 데이터 검증 및 변환
- NATS를 통한 데이터 발행
- 카테고리 스키마 기반 데이터 처리
- 실시간 데이터 스트리밍

### 3.5. data-consumer

**역할**: 데이터 소비 및 처리

**기능**:

- NATS 메시지 구독 및 처리
- 데이터베이스 쓰기 작업
- 배치 처리 및 집계
- 데이터 정합성 검증
- 실시간 알림 처리

## 4. 데이터 흐름

### 4.1. 데이터 수집 흐름

```
외부 데이터 소스 → data-manager → NATS → data-consumer → PostgreSQL/TimescaleDB
```

### 4.2. API 요청 흐름

```
클라이언트 → api → PostgreSQL (직접 연동)
```

### 4.3. 파일 저장 흐름

```
클라이언트 → api → SeaweedFS
```

## 5. 기술 스택

### 5.1. 외부 서비스

- **PostgreSQL 15 + TimescaleDB**: 시계열 데이터 저장
- **NATS JetStream**: 메시지 큐 및 스트리밍
- **SeaweedFS**: 분산 파일 저장

### 5.2. Go 구성요소

- **Fiber**: 웹 프레임워크 (api)
- **html/template**: 웹 템플릿 엔진
- **NATS Go Client**: 메시지 처리
- **pgx**: PostgreSQL 드라이버

## 6. 개발 환경

### 6.1. 컨테이너 구성

- **프로덕션**: 올인원 컨테이너 (모든 서비스 포함)
- **개발**: 라이브 리로딩 지원 (air 사용)
- **볼륨 마운트**: 소스 코드 실시간 반영

### 6.2. 로그 관리

- **통합 로그**: supervisor를 통한 중앙집중식 로그 관리
- **선택적 출력**: CLI를 통한 구성요소별 로그 제어
- **구조화된 로그**: JSON 형태의 구조화된 로그 출력

## 7. 구현 우선순위

### Phase 1: 기본 프로세스 관리

1. tmidb-supervisor 구현
2. 외부 서비스 프로세스 관리
3. 기본 로그 통합

### Phase 2: 핵심 구성요소

1. api 서버 구현
2. data-manager 구현
3. data-consumer 구현

### Phase 3: CLI 및 고급 기능

1. tmidb-cli 구현
2. 로그 제어 시스템
3. 프로세스 모니터링

### Phase 4: 웹 관리 콘솔

1. 사용자 관리
2. 데이터 탐색기
3. 시스템 모니터링 대시보드

## 8. 향후 확장성

- **마이크로서비스 분리**: 필요시 각 구성요소를 독립 서비스로 분리 가능
- **수평 확장**: data-manager, data-consumer의 다중 인스턴스 지원
- **플러그인 시스템**: 외부 데이터 소스 연동을 위한 플러그인 아키텍처
- **클러스터링**: 다중 노드 환경에서의 분산 처리 지원
