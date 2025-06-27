# tmiDB Git Submodules 설정 가이드

이 가이드는 tmiDB 프로젝트를 개별 Git 저장소로 분리하면서 전체를 하나의 메인 저장소로 관리하는 방법을 설명합니다.

## 🎯 목표 구조

```
tmidb (메인 저장소)
├── tmidb-core/ (submodule)
├── tmidb-mqtt/ (submodule)
├── tmidb-realtime/ (submodule)
├── docker-compose.yml
├── docker-compose.dev.yml
├── README.md
└── bluePrint.v1.md
```

## 📋 사전 준비

### 1. GitHub 저장소 생성

다음 저장소들을 GitHub에서 미리 생성해주세요:

- `tmidb-core`
- `tmidb-mqtt`
- `tmidb-realtime`
- `tmidb` (메인 저장소, 현재 저장소를 유지하거나 새로 생성)

### 2. 현재 작업 저장

```bash
# 현재 작업을 모두 커밋
git add .
git commit -m "Save current work before submodule conversion"
git push
```

## 🚀 변환 과정

### 단계 1: 스크립트 설정

1. `setup_submodules.sh` 파일에서 GitHub 사용자명을 수정:

```bash
# GitHub 사용자명 (수정 필요)
GITHUB_USER="your-actual-github-username"
```

2. `convert_to_submodules.sh` 파일에서도 동일하게 수정

### 단계 2: 개별 저장소 생성

```bash
./setup_submodules.sh
```

이 스크립트는 다음을 수행합니다:

- 프로젝트 백업 생성 (`tmiDB_temp/`)
- 각 모듈별로 독립적인 Git 저장소 초기화
- GitHub 원격 저장소 연결 설정

### 단계 3: 개별 저장소 푸시

스크립트 실행 후 각 모듈을 GitHub에 푸시:

```bash
# tmidb-core 푸시
cd tmiDB_temp/tmidb-core
git push -u origin main

# tmidb-mqtt 푸시
cd ../tmidb-mqtt
git push -u origin main

# tmidb-realtime 푸시
cd ../tmidb-realtime
git push -u origin main

# 원래 디렉토리로 복귀
cd ../../tmiDB
```

### 단계 4: Submodules로 변환

```bash
./convert_to_submodules.sh
```

이 스크립트는 다음을 수행합니다:

- 기존 모듈 디렉토리 제거
- Git Submodules로 각 저장소 추가
- 변경사항 커밋

### 단계 5: 메인 저장소 푸시

```bash
git push
```

## 🔧 일상적인 사용법

### 전체 프로젝트 클론

```bash
# 처음 클론할 때
git clone --recursive https://github.com/poi0322/tmidb.git

# 또는 일반 클론 후 submodules 초기화
git clone https://github.com/poi0322/tmidb.git
cd tmidb
git submodule update --init --recursive
```

### 개별 모듈에서 작업

```bash
# 특정 모듈로 이동
cd tmidb-core

# 작업 후 커밋 & 푸시
git add .
git commit -m "Update core functionality"
git push

# 메인 저장소에서 submodule 업데이트 반영
cd ..
git add tmidb-core
git commit -m "Update tmidb-core submodule"
git push
```

### 🚀 자동 업데이트 (권장)

**1. 스크립트를 사용한 간편 업데이트:**

```bash
./update_submodules.sh
```

이 스크립트는 다음을 자동으로 수행합니다:

- 메인 저장소 최신 상태로 업데이트
- 모든 submodules를 최신 버전으로 업데이트
- 변경사항 확인 및 커밋
- 원격 저장소에 푸시

**2. GitHub Actions 자동 업데이트:**

- 매일 한국 시간 오전 9시에 자동 실행
- GitHub의 Actions 탭에서 수동 실행 가능
- 개별 submodule 업데이트 시 자동 트리거 (webhook 설정 시)

### 수동 업데이트

**모든 Submodules 최신 버전으로 업데이트:**

```bash
# 모든 submodules를 최신 버전으로 업데이트
git submodule update --remote --merge

# 변경사항이 있으면 커밋
git add .
git commit -m "Update all submodules to latest"
git push
```

**특정 Submodule만 업데이트:**

```bash
# 특정 submodule만 업데이트
git submodule update --remote --merge tmidb-core

git add tmidb-core
git commit -m "Update tmidb-core to latest"
git push
```

## 🐳 Docker Compose 사용

Docker Compose는 변경 없이 그대로 사용 가능합니다:

```bash
# 개발 환경
docker compose -f docker-compose.dev.yml up

# 프로덕션 환경
docker compose up
```

## 🔍 장점

1. **독립적인 개발**: 각 모듈을 독립적으로 개발하고 배포 가능
2. **자동 버전 관리**: 각 모듈의 최신 버전을 자동으로 추적
3. **권한 관리**: 모듈별로 다른 접근 권한 설정 가능
4. **CI/CD**: 각 모듈별로 독립적인 CI/CD 파이프라인 구성 가능
5. **전체 관리**: 메인 저장소에서 전체 프로젝트 오케스트레이션
6. **자동화**: GitHub Actions를 통한 완전 자동 업데이트

## ⚠️ 주의사항

1. **Submodule 커밋**: 개별 모듈에서 작업 후 메인 저장소에서도 submodule 업데이트를 커밋해야 합니다
2. **클론 시 --recursive**: 새로운 개발자는 반드시 `--recursive` 옵션으로 클론해야 합니다
3. **브랜치 관리**: 각 submodule은 독립적인 브랜치를 가지므로 브랜치 전략을 명확히 해야 합니다

## 🛠️ 문제 해결

### Submodule이 비어있는 경우

```bash
git submodule update --init --recursive
```

### Submodule 삭제가 필요한 경우

```bash
git submodule deinit -f tmidb-core
git rm -f tmidb-core
rm -rf .git/modules/tmidb-core
```

### 모든 Submodules 상태 확인

```bash
git submodule status
```
