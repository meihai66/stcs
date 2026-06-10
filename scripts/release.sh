#!/usr/bin/env bash
# 一键发布:用 VERSION 文件里的版本号构建并推送 docker 镜像(同时打 latest),
# 推送成功后把补丁号 +1 写回 VERSION,供下次发布。
set -euo pipefail
cd "$(dirname "$0")/.."

IMAGE="meihai0211/stcs"
VERSION="$(tr -d '[:space:]' < VERSION)"

echo "==> 构建 ${IMAGE}:${VERSION} 与 ${IMAGE}:latest"
docker build -t "${IMAGE}:${VERSION}" -t "${IMAGE}:latest" .

echo "==> 推送 ${IMAGE}:${VERSION}"
docker push "${IMAGE}:${VERSION}"
echo "==> 推送 ${IMAGE}:latest"
docker push "${IMAGE}:latest"

# 补丁号 +1
IFS=. read -r a b c <<< "${VERSION}"
NEXT="${a}.${b}.$((c + 1))"
echo "${NEXT}" > VERSION
echo "==> 完成。下次版本号 -> ${NEXT}"
