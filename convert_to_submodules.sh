#!/bin/bash

# 개별 저장소들을 Git Submodules로 변환하는 스크립트
# setup_submodules.sh 실행 후에 사용하세요

set -e

echo "🔄 기존 모듈 디렉토리를 Git Submodules로 변환합니다..."

# GitHub 사용자명 (수정 필요)
GITHUB_USER="poi0322"

# 기존 모듈 디렉토리 제거
modules=("tmidb-core" "tmidb-mqtt" "tmidb-realtime")

echo "🗑️  기존 모듈 디렉토리 제거 중..."
for module in "${modules[@]}"; do
    if [ -d "${module}" ]; then
        echo "   - ${module} 제거"
        rm -rf "${module}"
    fi
done

# Git에서 제거된 디렉토리 커밋
git add .
git commit -m "Remove module directories before converting to submodules" || echo "커밋할 변경사항이 없습니다."

echo "📦 Git Submodules 추가 중..."
for module in "${modules[@]}"; do
    echo "   - ${module} submodule 추가"
    git submodule add "https://github.com/${GITHUB_USER}/${module}.git" "${module}"
done

# Submodules 초기화
echo "🔧 Submodules 초기화 중..."
git submodule update --init --recursive

# 변경사항 커밋
git add .
git commit -m "Convert to Git Submodules structure

- Add tmidb-core as submodule
- Add tmidb-mqtt as submodule  
- Add tmidb-realtime as submodule"

echo "✅ Git Submodules 변환 완료!"
echo ""
echo "📋 사용법:"
echo "1. 전체 프로젝트 클론:"
echo "   git clone --recursive https://github.com/${GITHUB_USER}/tmidb.git"
echo ""
echo "2. 개별 모듈 업데이트:"
echo "   git submodule update --remote"
echo ""
echo "3. 특정 모듈에서 작업:"
echo "   cd tmidb-core"
echo "   # 작업 후"
echo "   git add . && git commit -m 'Update' && git push"
echo "   cd .."
echo "   git add tmidb-core && git commit -m 'Update tmidb-core submodule'" 