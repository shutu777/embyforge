# 阶段1：构建前端
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm config set registry https://registry.npmmirror.com && \
    npm install --legacy-peer-deps --ignore-scripts
COPY frontend/ .
RUN npm run build:icons && npm run build

# 阶段2：构建后端（纯 Go SQLite，无需 CGO）
FROM golang:1.23-alpine AS backend-builder
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
ENV GOPROXY=https://goproxy.cn,direct
RUN go mod download
COPY backend/ .
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -o /app/embyforge ./cmd/server

# 阶段3：最终运行镜像
FROM alpine:3.19
RUN apk add --no-cache nginx supervisor ca-certificates tzdata \
    && mkdir -p /run/nginx /data /data/uploads/avatars \
    && chown -R nginx:nginx /data/uploads

# 复制 Nginx 配置
COPY nginx.conf /etc/nginx/http.d/default.conf

# 复制前端构建产物
COPY --from=frontend-builder /app/frontend/dist /usr/share/nginx/html

# 复制后端二进制
COPY --from=backend-builder /app/embyforge /usr/local/bin/embyforge

# 复制 supervisord 配置
COPY supervisord.conf /etc/supervisord.conf

# 环境变量默认值（JWT_SECRET 不再硬编码，由程序自动生成）
ENV EMBYFORGE_PORT=8080 \
    EMBYFORGE_DB_PATH=/data/embyforge.db

EXPOSE 80

VOLUME ["/data"]

CMD ["supervisord", "-c", "/etc/supervisord.conf"]
