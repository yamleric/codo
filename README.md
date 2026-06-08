# Codo

## 项目名

Codo

## 这是什么

Codo 是一个私有部署的个人信息助理。它把你收藏、订阅、同步进来的网页、RSS、视频、邮件、学习通任务和收藏夹内容，自动整理成可搜索、可问答、可回顾的个人知识库。

## 一句话描述

**收藏、订阅、同步资料后，Codo 自动抓取、总结、分类、入库，并按你的偏好推送。**

Codo 的目标不是做一个通用爬虫平台，而是把个人每天遇到的文章、RSS、视频、邮件、学习通任务、收藏夹内容沉淀成可搜索、可问答、可回顾的知识库。

> 重要说明：**知乎链接当前不可用**。项目里保留了 Playwright 渲染抓取能力，但知乎目前不能作为稳定支持来源，请不要把 README 中的“网页抓取”理解为已经支持知乎。
> 当前版本**暂时只支持首次初始化的 owner 单账号**，还没有多用户注册和数据隔离；后续会考虑加入多用户支持。

---

## 怎么跑

### Docker Compose 运行（推荐）

clone 仓库：

```bash
git clone https://github.com/yamleric/codo.git
cd codo
```

安装依赖：

```text
不需要 pip install -r requirements.txt。
本项目不是 Python 项目，推荐直接使用 Docker / Docker Compose 运行。
```

配置 Unity.ai API key：

```bash
cp .env.example .env
```

编辑 `.env`，至少填写：

```env
POSTGRES_PASSWORD=change-me
LLM_BASE_URL=https://你的-unity-ai-中转地址/v1
LLM_API_KEY=你的-unity-ai-api-key
LLM_MODEL=gpt-5
```

`LLM_BASE_URL` 和 `LLM_API_KEY` 可以填写 Unity.ai 赞助的 OpenAI-compatible API 中转服务地址和 Key；如果你使用其他兼容 OpenAI Chat Completions 的服务，也可以替换成对应地址和 Key。

运行：

```bash
docker compose -f docker-compose.release.yml up -d
```

默认访问：

```text
http://服务器IP:8090
```

第一次访问会进入 owner setup，创建工作台登录账号和密码。

> 如果使用 GitHub Release 附件部署，也可以只下载 `docker-compose.yml` 和 `env.example`。完整说明见 [docs/deploy-docker.md](docs/deploy-docker.md)。

### 本地开发

本地开发需要：

- Go 1.26+
- Node.js 22+
- PostgreSQL + pgvector

常用命令：

```bash
go mod download
npm --prefix web install
npm --prefix web run build
go run ./cmd/api
```

---

## 用了什么

| 类型 | 具体实现 |
| --- | --- |
| LLM API | Unity.ai 赞助的 OpenAI-compatible API 中转服务；模型由 `.env` 的 `LLM_MODEL` 指定 |
| Embedding API | OpenAI-compatible Embeddings API，可选，用于语义搜索 |
| ASR API | OpenAI-compatible `/audio/transcriptions`，可选，用于视频无字幕时转写 |
| 后端 | Go |
| 前端 | Vue 3 + Vite |
| 数据库 | PostgreSQL + pgvector |
| 网页解析 | go-readability、goquery、Playwright |
| RSS | gofeed |
| 视频 | yt-dlp + FFmpeg |
| 邮件 | go-imap + go-message |
| 通知 | Telegram Bot、SMTP Email |
| 部署 | Docker / Docker Compose |

> 配置名沿用项目里的通用 OpenAI-compatible 适配层：`LLM_BASE_URL`、`LLM_API_KEY`、`LLM_MODEL`。Unity.ai 是 API 中转和赞助来源；模板里的 `UNITY2_API_KEY` 在 Codo 中对应 `.env` 里的 `LLM_API_KEY`。

---

## 主要功能

- 网页链接抓取、摘要、分类、标签、入库
- RSS / Atom 订阅巡检和自动总结
- B站视频总结；抖音部分可用但依赖 cookies 和平台状态
- 学习通作业 / 考试提醒
- IMAP 邮件收件箱总结
- 收藏夹管理和 linux.do 书签导入
- 知识库搜索、语义搜索和问答
- 英文资料自动翻译
- 偏好记忆和反馈学习
- Telegram / Email 通知
- 日报 / 周报 / 月报推送

---

## 当前已完成

### 工作台与账号

| 功能 | 状态 | 说明 |
| --- | --- | --- |
| Web 工作台 | 已完成 | 浏览器访问任务看板、知识库、订阅源、收藏夹、设置页 |
| 首次初始化账号 | 已完成 | 第一次访问进入 owner setup，创建单人自用账号 |
| 登录鉴权 | 已完成 | 工作台 API 默认需要登录访问 |
| 前台配置管理 | 已完成 | 可在网页端配置 LLM、Embedding、ASR、Telegram、SMTP 等密钥；密钥保存后不回显 |
| 运行能力检查 | 已完成 | 前台显示 LLM、Embedding、ASR、SMTP、Telegram、Playwright、yt-dlp、ffmpeg 等可用状态 |

### 内容抓取与总结

| 功能 | 状态 | 说明 |
| --- | --- | --- |
| 手动提交链接 | 已完成 | 粘贴 URL 后自动创建任务、抓取正文、生成摘要、入库 |
| 普通网页正文解析 | 已完成 | 使用 HTTP 抓取、readability/goquery 提取正文，适合博客、新闻、文档类网页 |
| 自动分类和标签 | 已完成 | LLM 输出动态分类和短标签，不需要预置固定分类；例如政治、财经、技术都可以自动生成 |
| 英文资料自动翻译 | 已完成 | 可在设置页开启；英文网页会先翻译为中文，再用于摘要或知识库分块 |
| B站视频总结 | 已完成 | 自动识别 B站链接，优先字幕，缺字幕时可走 ASR 转写 |
| 抖音视频总结 | 部分可用 | 依赖 yt-dlp、cookies、UA 和平台状态，失败概率高于 B站 |
| 知乎链接抓取 | 当前不可用 | 不要认为当前版本支持知乎；后续会单独做适配 |

### 订阅源与外部来源

| 功能 | 状态 | 说明 |
| --- | --- | --- |
| RSS / Atom 订阅 | 已完成 | 支持前台新增、启停、分组、手动刷新和后台定时巡检 |
| 微信公众号文章 | 间接支持 | 通过 RSSHub 或其他 RSS 源接入，不是直接对接微信官方接口 |
| 学习通作业 / 考试提醒 | 已完成 | 可在订阅源里配置账号密码或 Cookie，巡检新作业、新考试和临近截止项 |
| 邮件收件箱总结 | 已完成 | 通过 IMAP 只读同步邮件，解析正文后进入摘要、知识库和日报流程 |
| 收藏夹管理 | 已完成 | 支持手动导入链接、编辑收藏元数据、同步入知识库 |
| linux.do 书签导入 | 已完成 | 支持导入 Discourse 导出的 `bookmarks.csv` 或 zip，过滤 linux.do 链接后总结入库 |

### 知识库、搜索和问答

| 功能 | 状态 | 说明 |
| --- | --- | --- |
| 知识库列表和详情 | 已完成 | 可查看文章摘要、原文内容、来源、分类、标签和抓取元数据 |
| 关键词搜索 | 已完成 | 搜索标题、摘要、标签、分类和内容分块 |
| 语义搜索 | 已完成 | 配置 Embedding 后使用 pgvector 做语义检索 |
| 知识库问答 | 已完成 | 基于搜索结果生成回答，并返回引用来源 |
| 偏好记忆 | 已完成 | 通过收藏意图和“有用 / 没用 / 类似通知 / 类似静默”反馈学习过滤偏好 |

### 推送和日报

| 功能 | 状态 | 说明 |
| --- | --- | --- |
| Telegram 通知 | 已完成 | 可作为即时通知和日报渠道 |
| Email 通知 | 已完成 | 用户名是邮箱时可自动使用该邮箱作为通知地址，也可在前台显式配置 |
| SMTP 配置 | 已完成 | 可在前台配置 SMTP Host、Port、Username、Password、From、TLS |
| 日报 / 周报 / 月报 | 已完成 | 可配置频率、发送小时、时区、最大条数、来源范围、分类范围和推送渠道 |
| 按分类拆开发送 | 已完成 | 日报可按分类拆分，也可合并为一条总结 |

### 部署

| 功能 | 状态 | 说明 |
| --- | --- | --- |
| Docker 镜像 | 已完成 | API 和 scheduler 使用同一个镜像 |
| Docker Compose 发布版 | 已完成 | 包含 `postgres`、`api`、`scheduler` 三个服务 |
| GitHub Release 构建 | 已完成 | 发布时生成镜像和 compose/env 示例文件 |
| 私有部署 | 已完成 | 默认只绑定 `127.0.0.1:8090`，建议公网通过 Nginx / Caddy 反代 |

---

## 当前限制

- **知乎当前不可用**：不要把知乎当成已支持来源。后续会做专门适配，包括登录态、反爬处理、正文展开和失败降级。
- **Codo 不是通用反爬破解器**：普通网页和部分 JS 渲染页面可以抓取，但 Cloudflare、防机器人、强登录站点不保证成功。
- **抖音解析不稳定**：抖音经常要求新鲜 cookies 或登录态，yt-dlp 可用性会随平台策略变化。
- **微信公众号不是直接抓微信**：当前推荐通过 RSSHub 或其他 RSS 化服务接入。
- **群消息总结还未落地**：QQ / 微信群消息总结仍在规划中，README 不把它列为当前可用功能。
- **英文翻译默认关闭**：需要在前台设置里开启，避免无意增加 LLM 调用成本。

---

## 未来计划

| 方向 | 计划 |
| --- | --- |
| 知乎适配 | 单独实现知乎链接解析、登录态管理、展开阅读全文、失败提示和测试用例 |
| 网页抓取增强 | 更清晰地区分静态网页、JS 渲染网页、需登录网页和不可抓取网页 |
| 视频稳定性 | 改进 B站 / 抖音 cookies 配置提示、失败诊断和 ASR 成本控制 |
| RSS 能力 | 支持 OPML 导入导出、订阅规则、源级过滤和更细的健康状态 |
| 邮件助理 | 支持更多邮箱模板、重要邮件规则、邮件标签和线程级总结 |
| 通知渠道 | 增加 Webhook、企业微信、飞书等渠道 |
| 收藏入口 | 浏览器扩展、移动端分享入口、Telegram 输入端 |
| 知识库 | 更好的引用视图、双语内容展示、相似内容聚类、重复内容合并 |
| 偏好学习 | 用更多显式反馈修正过滤策略，让系统更理解收藏意图 |

---

## 核心交互模型

你只需要做三类动作：

```text
收藏一个链接        -> 自动抓取、摘要、分类、入库
订阅一个来源        -> 定时巡检新内容，自动总结
提一个问题          -> 从个人知识库检索并回答
```

内容进入知识库后，可以继续被日报、搜索、问答、偏好记忆使用。

---

## 技术栈

| 层级 | 技术 | 作用 |
| --- | --- | --- |
| 后端服务 | Go | API、scheduler、pipeline、抓取和通知 |
| 前端 | Vue 3 + Vite | Web 工作台、设置页、知识库、订阅源和收藏夹管理 |
| 数据库 | PostgreSQL + pgvector | 任务、文章、订阅源、配置、知识库分块和向量检索 |
| LLM / Embedding / ASR | OpenAI-compatible API | 摘要、分类、过滤、翻译、问答、向量化和音频转写 |
| 网页解析 | go-readability、goquery、Playwright | 普通网页正文提取；Playwright 用于部分渲染场景，但当前不代表支持知乎 |
| RSS | gofeed | RSS / Atom 订阅解析 |
| 视频 | yt-dlp + FFmpeg | 视频元数据、字幕、音频抽取和转写前处理 |
| 邮件读取 | go-imap + go-message | IMAP 只读同步和 MIME 正文解析 |
| 通知 | Telegram Bot、SMTP Email | 即时通知和日报推送 |
| 部署 | Docker / Docker Compose | 私有部署和发布版运行 |
| 发布 | GitHub Actions / GHCR | 构建发布镜像和 Release 附件 |

---

## Release 部署

发布版可直接用 Docker Compose 运行，不需要本地构建源码：

```bash
mkdir codo
cd codo
curl -LO https://github.com/yamleric/codo/releases/latest/download/docker-compose.yml
curl -LO https://github.com/yamleric/codo/releases/latest/download/env.example
cp env.example .env
```

编辑 `.env`，至少配置：

```env
POSTGRES_PASSWORD=change-me
LLM_BASE_URL=https://你的-unity-ai-中转地址/v1
LLM_API_KEY=你的-unity-ai-api-key
LLM_MODEL=gpt-5
```

启动：

```bash
docker compose up -d
```

默认访问地址：

```text
http://服务器IP:8090
```

第一次访问会进入 owner setup，创建工作台登录账号和密码。

更完整的部署、反向代理、更新和备份说明见 [docs/deploy-docker.md](docs/deploy-docker.md)。

---

## 设计原则

- 单人单用，先把个人工作流跑通
- 私有部署，数据和密钥自己掌控
- 密钥只写入不回显，避免进入日志和仓库
- 先清楚标注可用边界，再扩展更多来源
- 收藏、订阅、同步之后，后续处理尽量自动化

---

## 致谢与参考

完整目录见 [CREDITS.md](CREDITS.md)。

这个目录集中记录两类内容：

- **已接入依赖**：项目实际使用的 Go / Node / Docker 依赖，例如 gofeed、go-imap、go-message、Playwright、yt-dlp、FFmpeg、pgvector、Vue、Vite 等。
- **参考过的开源项目**：没有复制源码、没有作为运行依赖引入，但在架构、产品组织、订阅源管理、学习通提醒、收藏管理、偏好记忆、翻译方案等设计上参考过的项目。
