#!/bin/bash
# EmbyForge 一键构建脚本
# 支持 amd64 和 arm64 多架构镜像

set -e

DOCKER_REPO="shutu736/embyforge"
IMAGE_TAG="${1:-latest}"
PLATFORMS="linux/amd64,linux/arm64"

echo "=== EmbyForge 构建脚本 ==="
echo "镜像仓库: ${DOCKER_REPO}:${IMAGE_TAG}"
echo "目标平台: ${PLATFORMS}"
echo ""

# 检查并创建 buildx builder
BUILDER_NAME="embyforge-builder"
if ! docker buildx inspect "${BUILDER_NAME}" > /dev/null 2>&1; then
    echo ">>> 创建 buildx builder..."
    docker buildx create --name "${BUILDER_NAME}" --use --driver docker-container
else
    docker buildx use "${BUILDER_NAME}"
fi

case "${2}" in
    --push)
        # 检查是否已登录 Docker Hub
        if ! docker info 2>/dev/null | grep -q "Username"; then
            echo ">>> 未检测到 Docker Hub 登录，请先登录..."
            docker login
        fi

        echo ">>> 构建并推送多架构镜像..."
        docker buildx build \
            --platform "${PLATFORMS}" \
            -t "${DOCKER_REPO}:${IMAGE_TAG}" \
            -t "${DOCKER_REPO}:latest" \
            --push \
            .

        echo ""
        echo ">>> 推送完成！"
        echo "拉取镜像: docker pull ${DOCKER_REPO}:${IMAGE_TAG}"
        ;;

    --load)
        echo ">>> 构建并加载到本地（仅当前架构）..."
        docker buildx build \
            -t "${DOCKER_REPO}:${IMAGE_TAG}" \
            --load \
            .

        echo ""
        echo ">>> 构建完成，镜像已加载到本地。"
        ;;

    *)
        echo ">>> 构建多架构镜像（不推送）..."
        docker buildx build \
            --platform "${PLATFORMS}" \
            -t "${DOCKER_REPO}:${IMAGE_TAG}" \
            .

        echo ""
        echo ">>> 构建完成（未推送）。"
        ;;
esac

echo ""
echo "用法："
echo "  ./build.sh [tag] [--load|--push]"
echo ""
echo "示例："
echo "  ./build.sh latest --load    # 构建并加载到本地（当前架构）"
echo "  ./build.sh latest --push    # 构建并推送到 Docker Hub"
echo "  ./build.sh v1.0 --push      # 构建 v1.0 并推送"
echo ""
echo "运行方式："
echo "  docker-compose up -d"
echo "  docker run -d -p 8880:80 -v embyforge-data:/data ${DOCKER_REPO}:${IMAGE_TAG}"
