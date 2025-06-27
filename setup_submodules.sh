#!/bin/bash

# tmiDB 프로젝트를 Git Submodules로 분리하는 스크립트
# 
# 사용법:
# 1. GitHub에서 다음 저장소들을 미리 생성해주세요:
#    - tmidb-core
#    - tmidb-mqtt  
#    - tmidb-realtime
# 2. 이 스크립트를 실행하기 전에 현재 작업을 모두 커밋해주세요
# 3. ./setup_submodules.sh 실행

set -e

echo "🚀 tmiDB Git Submodules 설정을 시작합니다..."

# 현재 디렉토리 저장
ORIGINAL_DIR=$(pwd)
TEMP_DIR="${ORIGINAL_DIR}_temp"

# GitHub 사용자명 (수정 필요)
GITHUB_USER="poi0322"

# 1. 백업 생성
echo "📦 현재 프로젝트 백업 생성 중..."
cp -r "${ORIGINAL_DIR}" "${TEMP_DIR}"

# 2. 각 모듈별 독립 저장소 생성
modules=("tmidb-core" "tmidb-mqtt" "tmidb-realtime")

for module in "${modules[@]}"; do
    echo "🔧 ${module} 독립 저장소 생성 중..."
    
    # 모듈 디렉토리로 이동
    cd "${TEMP_DIR}/${module}"
    
    # 새로운 Git 저장소 초기화
    rm -rf .git
    git init
    git add .
    git commit -m "Initial commit for ${module}"
    
    # GitHub 저장소와 연결 (실제 URL로 수정 필요)
    echo "📡 GitHub 저장소 연결: https://github.com/${GITHUB_USER}/${module}.git"
    git remote add origin "https://github.com/${GITHUB_USER}/${module}.git"
    git branch -M main
    
    echo "⚠️  다음 명령어를 수동으로 실행해주세요:"
    echo "   cd ${TEMP_DIR}/${module}"
    echo "   git push -u origin main"
    echo ""
    
    cd "${ORIGINAL_DIR}"
done

echo "✅ 개별 저장소 준비 완료!"
echo ""
echo "📋 다음 단계:"
echo "1. GitHub에서 각 저장소를 생성하세요:"
for module in "${modules[@]}"; do
    echo "   - https://github.com/${GITHUB_USER}/${module}"
done
echo ""
echo "2. 각 모듈을 푸시하세요:"
for module in "${modules[@]}"; do
    echo "   cd ${TEMP_DIR}/${module} && git push -u origin main"
done
echo ""
echo "3. 개별 저장소 푸시 완료 후 convert_to_submodules.sh를 실행하세요" 