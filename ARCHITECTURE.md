# Codo 架构设计

> 当前版本定位：单人自用的信息摄取、摘要、知识库和通知工作台。
> 核心解法：确定性工作流负责流程编排，AI 只作为过滤、摘要、分类、翻译和问答节点。

本文档描述当前代码库已经落地的架构。`FEATURES.md` 属于本地需求和路线图文档，不作为当前架构依据，也不再同步到远端仓库。

---

## 设计原则

1. **流程由代码控制**：抓取、过滤、摘要、入库、通知的顺序由 Pipeline 固定，模型不决定系统行为。
2. **内容类型保持稳定**：`content_type` 只表示处理方式，主题用 `category` 和 `tags` 表达。
3. **过滤不阻断用户主动收藏**：手动提交和收藏来源不走丢弃过滤，避免误杀用户明确想保存的内容。
4. **配置可在前台编辑**：LLM、Embedding、ASR、Telegram、SMTP 和用户通知策略可从工作台维护。
5. **密钥不回显**：API 只返回 `*_configured` 状态，不返回 API key、token、SMTP password、Cookie 等真实密钥。
6. **单人单用优先**：保留鉴权和会话，但不引入团队、角色、管理员体系。

---

## 当前服务拓扑

```text
浏览器工作台
  │
  │  HTTPS
  ▼
Nginx / Caddy
  │
  │  反向代理
  ▼
cmd/api
  ├─ Web UI 静态资源
  ├─ /api/* 工作台接口
  ├─ /ws 任务状态推送
  └─ 手动链接、收藏夹导入、设置、搜索、问答

cmd/scheduler
  ├─ RSS 巡检
  ├─ 学习通作业/考试巡检
  ├─ 邮箱收件箱巡检
  ├─ 日报/周报/月报发送
  └─ 知识库 embedding 回填

PostgreSQL
  ├─ 业务数据
  ├─ pg_trgm 关键词召回
  └─ pgvector 语义召回
```

当前发布版使用 `api + scheduler + postgres`。`cmd/worker` 仍保留为可扩展入口，但不是当前 Docker Compose 发布路径的主链路。当前实现没有 Redis、River 队列或 Railway 部署依赖。

---

## 目录结构

```text
codo/
├── cmd/
│   ├── api/                 # HTTP API、Web UI、鉴权、WebSocket
│   ├── scheduler/           # 订阅巡检、日报、embedding 回填
│   └── worker/              # 预留 worker 入口
│
├── internal/
│   ├── domain/task/         # Task 聚合、状态、来源、内容类型、分类、翻译元数据
│   ├── application/
│   │   ├── ingest/          # URL 规范化、内容类型识别
│   │   ├── knowledge/       # 搜索、问答、embedding 回填
│   │   ├── pipeline/        # WebPage / Video / Email / Message Pipeline
│   │   ├── report/          # 日报、周报、月报汇总
│   │   └── sourcecheck/     # 学习通、邮箱等来源巡检用例
│   └── infra/
│       ├── auth/            # 密码和 session token 处理
│       ├── db/              # PostgreSQL 连接和迁移
│       ├── fetcher/         # HTTP 抓取、Playwright 渲染、yt-dlp 视频文字稿
│       ├── llm/             # OpenAI-compatible LLM / Embedding / ASR 适配
│       ├── notify/          # Telegram、SMTP Email
│       ├── runtimeconfig/   # 数据库配置 + 环境变量兜底
│       ├── sources/         # RSS、学习通、IMAP 实现
│       └── store/           # PostgreSQL 存储实现
│
├── web/                     # Vue 3 工作台
├── docker/                  # Caddy、初始化 SQL
├── docker-compose.release.yml
├── docker-compose.prod.yml
└── Dockerfile
```

---

## 入口与任务流

手动链接入口在 `cmd/api`：

```text
POST /api/tasks
  -> NormalizeURL
  -> DetectContentType
  -> task.New(source=manual)
  -> 记录用户收藏意图（可选）
  -> goroutine 内运行 Pipeline Router
```

订阅入口在 `cmd/scheduler`，启动后立即执行一次，之后每 30 分钟执行：

```text
runRSS
runChaoxing
runEmail
runDailyReport
runEmbeddingBackfill
```

所有内容最终都落到统一的 Task / Article 模型。Pipeline 按 `content_type` 分发，来源只影响过滤策略、通知策略和元数据。

---

## Pipeline

### WebPagePipeline

```text
去重
  -> 抓取网页正文
  -> 过滤判断
  -> 英文资料自动翻译（可选）
  -> 生成摘要
  -> 内容分类
  -> 存入知识库
  -> 按策略通知
```

手动提交的网页直接 `pass`。收藏夹和 Linux.do 书签来源为 `silent`，跳过丢弃过滤并静默入库，避免把用户主动收藏的链接误判为无价值。

### VideoPipeline

```text
去重
  -> yt-dlp 获取字幕或音频
  -> 无字幕时通过 ASR 转写
  -> 过滤判断
  -> 生成摘要
  -> 内容分类
  -> 存入知识库
  -> 按策略通知
```

当前目标是用户授权或公开视频链接的文字稿提取和总结。B 站、抖音依赖 `yt-dlp`、`ffmpeg` 和可选 ASR。Cookie、浏览器 profile、UA 等运行时参数只通过本地配置或只读挂载提供，不写入仓库。

### EmailPipeline

```text
IMAP 读取邮件
  -> LLM 分类 important / notify / spam
  -> spam 丢弃
  -> notify 静默入库
  -> important 摘要、入库并通知
```

邮箱来源使用只读 IMAP 配置，适合做个人收件箱摘要和重要邮件提取。

### MessagePipeline

`MessagePipeline` 已有抽象和处理流程，但当前发布版还没有接入具体群聊平台。因此文档和展示页不把群消息总结描述为已可用能力。

---

## 来源管理

当前订阅源统一存在 `subscriptions` 表，来源特有字段存在 `config` JSON 中。稳定、需要去重和提醒状态的检测结果进入 `source_items`。

已落地来源：

- **RSS / Atom**：通过 `gofeed` 拉取，生成网页或视频任务。
- **学习通**：支持账号密码或 Cookie 配置，巡检作业和考试，按新任务和临近截止推送提醒。
- **个人邮箱**：通过 IMAP 读取收件箱，交给 EmailPipeline 分类和摘要。
- **收藏夹 / Linux.do 书签**：前台导入 URL、浏览器书签或 Linux.do 导出的 `bookmarks.csv` / zip，再同步到知识库。

微信公众号当前只通过 RSSHub / RSS 间接接入，不提供微信官方接口抓取能力。知乎当前不可用；Playwright 只是通用 JS 渲染能力，不代表知乎支持已经完成。

---

## 分类、标签与新类型

`content_type` 不随主题扩张，只保留稳定处理方式：

- `webpage`
- `video`
- `email`
- `message`
- `post`

如果用户提交一个政治新闻链接，入口仍识别为 `webpage`。Pipeline 抓取正文后，分类器把主题写入：

- `articles.category = 政治`
- `articles.tags = {国际关系, 政策, ...}`

前端知识库通过 `/api/knowledge/facets` 聚合已有分类和标签，动态生成筛选项。也就是说，新的主题不需要新增数据库字段或内容类型。

`articles.metadata` 用于保存站点、作者、封面、翻译信息等来源特有数据。只有稳定、高频、需要索引或排序的字段才单独加列，例如 `published_at`。

---

## 知识库搜索与问答

入库时 `SaveKnowledgeItem` 会生成 `article_chunks`。搜索接口 `/api/search` 使用两级召回：

1. `pg_trgm`：对切片正文、标题、摘要、分类、标签做关键词召回。
2. `pgvector`：如果配置了 Embedding，并且 scheduler 已回填向量，则做语义召回并合并排序。

问答接口 `/api/qa` 复用搜索结果构造 RAG 上下文，并要求模型只依据引用片段回答。未配置 Embedding 时仍可关键词搜索；未配置 LLM 时搜索可用，问答返回配置错误。

---

## 偏好记忆与反馈

过滤层不只依赖关键词。用户可以在提交链接时写收藏意图，也可以在知识库卡片或详情页反馈“有用 / 没用 / 类似通知 / 类似静默”。

数据流：

```text
content_feedback
  -> user_memories
  -> preference_profiles
  -> LLM Filter 读取偏好证据
```

记忆只作为偏好证据，不作为系统指令。用户可在前台查看、编辑、停用或删除记忆，也可以关闭“记忆参与过滤”。

---

## 翻译

英文网页可自动翻译后再摘要。翻译配置由用户设置控制，翻译结果存在任务 / 文章 `metadata.translation` 中。根据配置，系统可以只把翻译用于摘要，也可以把译文写入知识库内容。

翻译和摘要共用 OpenAI-compatible LLM 客户端，不引入单独翻译服务。

---

## 通知与报告

即时通知由 Pipeline 的 `saveAndNotify` 触发，当前支持：

- `telegram`
- `email`
- `none`

如果用户首次注册时用户名是邮箱，系统会把初始通知渠道设为 `email`。用户之后可以在设置页改为 Telegram 或关闭通知。

日报、周报、月报由 `internal/application/report` 负责。用户可配置：

- 是否启用报告
- 推送渠道：Email、Telegram
- 频率：daily / weekly / monthly
- 发送小时和时区
- 最大条目数
- 来源和分类过滤
- 是否按分类拆分发送

`daily_reports` 使用 `(user_id, report_date)` 唯一约束记录 `sent` / `skipped` / `failed`，避免同一周期重复发送。

---

## 鉴权与配置管理

Codo 按单人工作台设计。首次访问时，如果默认用户还没有密码，会进入 owner setup。之后 `/api/*` 和 `/ws` 都需要 `codo_session` HttpOnly Cookie。

运行配置有两层：

```text
app_settings（前台保存）
  + 环境变量兜底
  -> runtimeconfig.Resolved
```

前台可编辑：

- LLM：`LLM_BASE_URL`、`LLM_API_KEY`、`LLM_MODEL`
- Embedding：`EMBEDDING_BASE_URL`、`EMBEDDING_API_KEY`、`EMBEDDING_MODEL`
- ASR：`ASR_BASE_URL`、`ASR_API_KEY`、`ASR_MODEL`
- Telegram：token、chat id
- SMTP：host、port、username、password、from、TLS

Unity.ai 在本项目中作为 OpenAI-compatible API 中转和赞助方使用。代码层面只依赖标准兼容接口，不绑定 Unity.ai 专有 SDK。

---

## 数据模型

核心表：

```text
users
  单人用户、用户名、密码哈希、通知渠道、过滤关键词、模型策略

auth_sessions
  登录 session，存 token hash，不存明文 token

app_settings
  实例级运行配置，保存为 JSONB

tasks
  每次处理任务的来源、内容类型、状态、过滤结果、摘要、分类、标签

task_steps
  Pipeline 每一步的状态、耗时和简短详情，用于看板展示

articles
  知识库主表，保存正文、摘要、分类、标签、metadata、published_at

article_chunks
  搜索和问答切片，支持 pg_trgm 和 pgvector

content_feedback
  用户反馈和收藏意图

user_memories
  可编辑、可停用的偏好记忆

preference_profiles
  聚合后的偏好画像

bookmarks
  收藏夹导入、同步状态和错误

subscriptions
  RSS、学习通、邮箱等订阅源配置

source_items
  学习通作业 / 考试等来源检测结果和提醒状态

daily_reports
  报告发送记录，避免重复发送
```

数据库扩展：

- `pg_trgm`：关键词搜索和模糊召回。
- `vector`：可选语义搜索和 embedding 召回。

---

## 部署

发布版面向 Docker Compose：

```text
docker-compose.release.yml
  ├─ postgres
  ├─ api
  ├─ scheduler
  └─ caddy
```

生产环境可以使用 `docker-compose.prod.yml`，通过 Nginx 或 Caddy 反向代理到 API。构建产物、`.env`、密钥、Cookie 文件、浏览器 profile 不进入 Git，也不进入镜像构建上下文。

---

## 当前限制

- 知乎不可用，不应在 README、展示页或架构中宣称支持。
- 群消息总结尚未接入具体平台。
- 微信公众号只支持 RSSHub / RSS 间接订阅。
- 视频解析依赖平台公开能力、用户授权状态、Cookie 和 `yt-dlp` 支持情况，不承诺绕过访问控制。
- 当前没有独立队列服务；API 手动任务使用 goroutine，订阅任务由 scheduler 周期执行。

---

## 后续扩展点

- 将手动任务和订阅任务统一接入持久化队列，提升重试、限流和水平扩展能力。
- 抽象通知服务，合并即时通知和报告发送的渠道选择逻辑。
- 增加 OPML 导入导出、订阅规则、来源级过滤策略。
- 为邮箱和学习通增加更细粒度的错误诊断和重新授权流程。
- 为知识库问答增加引用跳转、答案反馈和记忆更新闭环。
