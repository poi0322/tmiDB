#!/bin/bash

# tmiDB Submodules 자동 업데이트 스크립트
# 모든 submodule을 최신 버전으로 업데이트하고 메인 저장소에 커밋합니다.

set -e

echo "🔄 tmiDB Submodules 업데이트를 시작합니다..."

# 현재 브랜치 확인
CURRENT_BRANCH=$(git branch --show-current)
echo "📍 현재 브랜치: ${CURRENT_BRANCH}"

# 메인 저장소 최신 상태로 업데이트
echo "📥 메인 저장소 최신 상태로 업데이트 중..."
git pull origin ${CURRENT_BRANCH}

# 모든 submodules를 최신 버전으로 업데이트
echo "🔄 모든 submodules를 최신 버전으로 업데이트 중..."
git submodule update --remote --merge

# 변경사항이 있는지 확인
if git diff --quiet && git diff --staged --quiet; then
    echo "✅ 모든 submodules가 이미 최신 버전입니다."
    exit 0
fi

# Submodules 상태 표시
echo "📊 Submodules 상태:"
git submodule status

# 변경사항 확인
echo "📋 변경사항:"
git diff --name-only

# 사용자에게 확인 요청
echo ""
read -p "🤔 위 변경사항을 커밋하시겠습니까? (y/N): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "❌ 업데이트가 취소되었습니다."
    exit 1
fi

# 변경사항 커밋
TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
COMMIT_MESSAGE="Update submodules to latest versions (${TIMESTAMP})"

echo "💾 변경사항 커밋 중..."
git add .
git commit -m "${COMMIT_MESSAGE}"

# 원격 저장소에 푸시
echo "📤 원격 저장소에 푸시 중..."
git push origin ${CURRENT_BRANCH}

echo "✅ Submodules 업데이트 완료!"
echo ""
echo "🔗 업데이트된 submodules:"
git submodule foreach 'echo "  - $(basename $PWD): $(git log -1 --oneline)"' 