# EmbyForge

Emby 媒体服务器辅助管理工具，帮助检测刮削异常、查找重复媒体、分析集数映射问题。

## 功能

- **仪表盘**：媒体库概览和统计信息
- **媒体扫描**：同步 Emby 媒体库数据到本地缓存
- **刮削异常检测**：检测缺少封面图片或外部 ID（TMDB/IMDB）的媒体条目
- **重复媒体查找**：按 TMDB ID 查找重复电影，按季集号查找重复剧集
- **集数映射分析**：对比本地剧集季集数与 TMDB 数据，发现集数不一致的问题
- **Emby 配置管理**：配置和测试 Emby 服务器连接
- **系统设置**：配置 TMDB API Key 等系统参数
- **个人资料**：修改密码和头像

## 技术栈

- 前端：Vue 3 + Vuetify（Materio 模板）
- 后端：Go + Gin + GORM + SQLite
- 部署：Docker（Nginx + Supervisor）
- 多架构支持：amd64 / arm64

## 快速开始

### Docker 部署（推荐）

```bash
docker run -d \
  --name embyforge \
  -p 8880:80 \
  -v embyforge-data:/data \
  --restart unless-stopped \
  shutu736/embyforge:latest
```

访问 `http://localhost:8880` 即可使用。

### 默认账户

首次启动自动创建管理员账户：

- 用户名：`admin`
- 密码：`admin123`

请及时修改密码。

### 配置说明

所有配置均通过 Web 界面完成：

1. 进入 **Emby 配置** 页面，填写 Emby 服务器地址、端口和 API Key
2. 进入 **系统设置** 页面，填写 TMDB API Key（用于集数映射分析）
3. 进入 **媒体扫描** 页面，同步媒体库数据

### 数据持久化

SQLite 数据库和上传文件存储在 `/data` 目录，通过 Docker 卷持久化。

## 从源码构建

### 前置条件

- Go 1.23+
- Node.js 20+
- Docker（含 buildx 插件）

### 构建并推送多架构镜像

```bash
chmod +x build.sh
./build.sh latest --push
```

### 构建并加载到本地

```bash
./build.sh latest --load
```

### 本地开发

后端：

```bash
cd backend
go mod download
go run ./cmd/server
```

前端：

```bash
cd frontend
npm install
npm run dev
```
