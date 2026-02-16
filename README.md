# 🎬 EmbyForge

> Emby 媒体服务器辅助管理工具，帮助你更好地管理和维护媒体库。

## ✨ 功能特性

- 📊 **仪表盘** — 媒体库概览和统计信息一目了然
- 🔄 **媒体扫描** — 同步 Emby 媒体库数据到本地缓存，支持增量更新
- 🔍 **刮削异常检测** — 检测缺少封面图片或外部 ID（TMDB/IMDB）的媒体条目
- 📑 **重复媒体查找** — 按 TMDB ID 查找重复电影，按季集号查找重复剧集
- 🗂️ **集数映射分析** — 对比本地剧集季集数与 TMDB 数据，发现集数不一致的问题
- ⚙️ **Emby 配置管理** — 配置和测试 Emby 服务器连接
- 🔧 **系统设置** — 配置 TMDB API Key 等系统参数
- 👤 **个人资料** — 修改密码和头像

## 🛠️ 技术栈

| 层级 | 技术 |
|------|------|
| 前端 | Vue 3 + Vuetify（Materio 模板）|
| 后端 | Go + Gin + GORM + SQLite |
| 部署 | Docker（Nginx + Supervisor）|
| 架构 | amd64 / arm64 |

## 🚀 快速开始

### Docker 部署（推荐）

```bash
docker run -d \
  --name embyforge \
  -p 8880:80 \
  -v ./data:/data \
  --restart unless-stopped \
  shutu736/embyforge:latest
```

或使用 Docker Compose：

```bash
curl -O https://raw.githubusercontent.com/shutu777/embyforge/main/docker-compose.yml
docker compose up -d
```

访问 `http://localhost:8880` 即可使用 🎉

### 🔑 默认账户

首次启动自动创建管理员账户：

| 项目 | 值 |
|------|------|
| 用户名 | `admin` |
| 密码 | `admin` |

> ⚠️ 请登录后及时修改密码

### 📝 配置说明

所有配置均通过 Web 界面完成，无需修改配置文件：

1. 进入 **Emby 配置** 页面，填写 Emby 服务器地址、端口和 API Key
2. 进入 **系统设置** 页面，填写 TMDB API Key（用于集数映射分析）
3. 进入 **媒体扫描** 页面，同步媒体库数据

### 💾 数据持久化

SQLite 数据库和上传文件存储在 `/data` 目录，通过挂载到宿主机持久化。

## 🏗️ 从源码构建

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

```bash
# 后端
cd backend
go mod download
go run ./cmd/server

# 前端
cd frontend
npm install
npm run dev
```

## 📄 License

MIT
