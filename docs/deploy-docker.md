# Docker Compose 部署 Codo

Codo 的发布版使用 Docker Compose 部署。默认包含三个服务：

- `postgres`：PostgreSQL + pgvector，保存知识库、任务、订阅源和用户配置。
- `api`：Web 工作台和 API，监听容器内 `8080`。
- `scheduler`：定时巡检 RSS、学习通、邮箱、日报等后台任务。

`api` 和 `scheduler` 使用同一个镜像：`ghcr.io/yamleric/codo`。

## 1. 准备目录

```bash
mkdir codo
cd codo
```

## 2. 下载发布文件

从最新 Release 下载：

```bash
curl -LO https://github.com/yamleric/codo/releases/latest/download/docker-compose.yml
curl -LO https://github.com/yamleric/codo/releases/latest/download/env.example
cp env.example .env
```

也可以指定版本，例如 `v0.1.0`：

```bash
curl -LO https://github.com/yamleric/codo/releases/download/v0.1.0/docker-compose.yml
curl -LO https://github.com/yamleric/codo/releases/download/v0.1.0/env.example
cp env.example .env
```

使用固定版本时，建议把 `.env` 中的镜像版本也固定：

```env
CODO_VERSION=v0.1.0
```

## 3. 编辑配置

```bash
nano .env
```

最少需要改：

```env
POSTGRES_PASSWORD=change-this-to-a-strong-password
LLM_BASE_URL=https://api.openai.com/v1
LLM_API_KEY=your-api-key
LLM_MODEL=gpt-5
```

默认只监听本机：

```env
CODO_BIND=127.0.0.1:8090
```

如果要直接暴露给局域网或公网，可以改成：

```env
CODO_BIND=0.0.0.0:8090
```

更推荐公网使用 Nginx / Caddy 反代，并保留 `127.0.0.1:8090`。

## 4. 启动

```bash
docker compose up -d
```

查看状态：

```bash
docker compose ps
docker compose logs -f api
```

访问：

```text
http://服务器IP:8090
```

第一次访问会进入 owner setup，创建工作台登录账号和密码。

## 5. 配置反向代理

如果使用 Nginx，反代到：

```text
http://127.0.0.1:8090
```

WebSocket 路径 `/ws` 也需要转发。

## 6. 可选目录

视频 Cookies、浏览器 profile 和其他授权文件不要写入 Git。需要时在部署目录创建：

```bash
mkdir -p secrets browser-profile
```

然后在 `.env` 里配置：

```env
YTDLP_COOKIES_FILE=/run/codo-secrets/yt-dlp.cookies.txt
YTDLP_COOKIES_FROM_BROWSER=chromium:/run/codo-browser-profile/chromium
```

Compose 会把本机目录只读挂载到容器：

```text
./secrets -> /run/codo-secrets
./browser-profile -> /run/codo-browser-profile
```

## 7. 更新

使用 latest：

```bash
docker compose pull
docker compose up -d
```

使用固定版本：

```bash
nano .env
```

修改：

```env
CODO_VERSION=v0.1.1
```

然后：

```bash
docker compose pull
docker compose up -d
```

## 8. 备份

数据在 Docker volume `pgdata` 中。升级前建议备份数据库：

```bash
docker compose exec postgres pg_dump -U codo codo > codo-backup.sql
```

恢复时：

```bash
docker compose exec -T postgres psql -U codo codo < codo-backup.sql
```
