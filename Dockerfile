# 阶段1：构建前端
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend

# 先复制依赖文件，利用 Docker 层缓存（依赖不变时跳过 npm install）
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm config set registry https://registry.npmmirror.com && \
    npm install --legacy-peer-deps --ignore-scripts

# 再复制源码并构建
COPY frontend/ .
RUN npm run build:icons && npm run build

# 阶段2：构建后端
FROM golang:1.23-alpine AS backend-builder
WORKDIR /app/backend

# 先复制依赖文件，利用 Docker 层缓存
COPY backend/go.mod backend/go.sum ./
ENV GOPROXY=https://goproxy.cn,direct
RUN go mod download

# 再复制源码并构建（-ldflags 减小二进制体积，-trimpath 去除本地路径）
COPY backend/ .
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -trimpath -o /app/embyforge ./cmd/server

# 阶段3：最终运行镜像
FROM alpine:3.19

# 单层 RUN 减少镜像层数
RUN apk add --no-cache nginx supervisor ca-certificates tzdata \
    && mkdir -p /run/nginx /data /data/uploads/avatars \
    && chown -R nginx:nginx /data/uploads \
    && rm -rf /var/cache/apk/*

# 复制配置和构建产物
COPY nginx.conf /etc/nginx/http.d/default.conf
COPY supervisord.conf /etc/supervisord.conf
COPY --from=frontend-builder /app/frontend/dist /usr/share/nginx/html
COPY --from=backend-builder /app/embyforge /usr/local/bin/embyforge

ENV EMBYFORGE_PORT=8080 \
    EMBYFORGE_DB_PATH=/data/embyforge.db

EXPOSE 80
VOLUME ["/data"]

CMD ["supervisord", "-c", "/etc/supervisord.conf"]
