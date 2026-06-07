# Codo 架构设计 v2

> 核心问题：信息过载。
> 核心解法：确定性工作流做骨架，AI 只作为可控的能力节点。

---

## 设计原则

1. **工作流控制流程，模型只处理内容** — 代码决定"做什么"，模型决定"怎么写"
2. **模型要被注册、路由、限权、观测** — 不在业务代码里写死模型名
3. **过滤优先** — 推送给用户的每条内容都经过价值判断，不制造新的信息过载
4. **每次运行都可追踪、可重试、可恢复** — Run + Step + Trace 完整落库
5. **业务与基础设施隔离** — 换模型、换存储只动 infra，不污染业务层

---

## 项目目录结构

```
codo/
├── cmd/
│   ├── api/             # HTTP 服务入口
│   ├── worker/          # Pipeline Worker 入口
│   └── scheduler/       # 定时任务入口
│
├── internal/
│   ├── domain/          # 领域类型，不依赖任何外部库
│   │   └── task/        # Task（聚合根）, Step, Status, FilterDecision
│   │                    # SourceType, ContentType
│   │                    # 所有状态变更通过 Task 方法，并发安全
│   │
│   ├── application/     # 业务用例编排，依赖 domain + 基础设施接口
│   │   └── pipeline/    # Router + 4个 Pipeline 实现
│   │                    # 接口：Fetcher, Filterer, Summarizer,
│   │                    #       Classifier, Extractor, Store, Notifier
│   │                    # 每个 Pipeline 只注入自己需要的接口（ISP）
│   │
│   ├── infra/           # 具体实现，可替换，不影响业务层
│   │   ├── llm/         # OpenAI-compatible 中转站适配器
│   │   │                # 实现 Filterer / Summarizer / Classifier / Extractor
│   │   │                # 通过 /v1/chat/completions，BaseURL 指向中转站
│   │   ├── fetcher/     # HTTP / Playwright / Flaresolverr / yt-dlp
│   │   ├── sources/     # RSS(gofeed) / WechatMP / LinuxDo / Chaoxing / IMAP
│   │   ├── store/       # PostgreSQL + pgvector（实现 Store 接口）
│   │   ├── queue/       # riverqueue/river（Postgres 事务内入队）
│   │   └── notify/      # Telegram / Email / 微信
│   │
│   └── transport/
│       └── http/        # Webhook 接收、看板 API、WebSocket
│
├── web/                 # Vue 3 看板
├── docker/
├── docker-compose.yml
└── config.example.yaml
```

---

## 系统架构

```
┌─────────────────────────────────────────────────────────────┐
│                        信息入口层                            │
│                                                             │
│   主动触发                    被动订阅（Scheduler 定时拉）   │
│   Telegram Bot                RSS / 微信公众号（RSSHub）     │
│   Web 网页                    学习通 / linux.do             │
│                               邮件 IMAP / 群消息            │
└─────────────────────────┬───────────────────────────────────┘
                          │ 写入统一 Task 格式
┌─────────────────────────┬───────────────────────────────┐
│                    任务队列（riverqueue/river）            │
│              Postgres 事务内入队，与业务数据强一致          │
└─────────────────────────┬───────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────┐
│                      Pipeline Worker                         │
│                                                             │
│  按 ContentType 路由到对应 Pipeline：                        │
│                                                             │
│  WebPagePipeline   fetch → filter → summarize → save → notify│
│  VideoPipeline     subtitle → filter → summarize → save → notify│
│  EmailPipeline     parse → classify → digest → notify       │
│  MessagePipeline   collect → filter → extract → notify      │
│                                                             │
│  每个 Pipeline 内部：代码控制步骤顺序，LLM 只在节点内工作   │
└──────────────┬──────────────────────────────────────────────┘
               │
┌──────────────▼──────────────────────────────────────────────┐
│                      模型路由层                              │
│                                                             │
│  ModelRouter.Resolve(task) → 按任务类型选模型               │
│                                                             │
│  filter（过滤判断）   → 轻量模型（省成本）                   │
│  summarize（摘要）    → 主力模型（高质量）                   │
│  translate（翻译）    → 主力模型                             │
│  classify（邮件分类） → 轻量模型                             │
│  embedding（入库）    → embedding 模型                      │
└──────────────┬──────────────────────────────────────────────┘
               │
┌──────────────▼──────────────────────────────────────────────┐
│                     LLM 调用层（infra/llm）                  │
│                                                             │
│  统一使用 OpenAI-compatible /v1/chat/completions 接口        │
│  BaseURL 指向中转站，支持任意兼容模型（Claude / GPT / 等）   │
│  Filter / Classify 强制 JSON 输出，结果 normalize 防模型漂移 │
│  截断超长输入，防止 context window 溢出                      │
└──────────────┬──────────────────────────────────────────────┘
               │
┌──────────────▼──────────────────────────────────────────────┐
│                        数据层                                │
│                                                             │
│  PostgreSQL                         Redis（仅缓存）          │
│  ├─ tasks（任务 + Steps）            ├─ 去重 Set              │
│  ├─ task_steps（步骤记录）           └─ 会话缓存              │
│  ├─ river_jobs（任务队列，river）                             │
│  ├─ model_calls（LLM 调用 Trace）                            │
│  ├─ articles（知识库）                                        │
│  ├─ subscriptions（订阅源配置）                               │
│  └─ users（用户配置 + 模型策略）                              │
└──────────────┬──────────────────────────────────────────────┘
               │
┌──────────────▼──────────────────────────────────────────────┐
│               可观测层 / Agent 看板（WebSocket）              │
│                                                             │
│  ├─ 实时任务状态（每步进度）                                  │
│  ├─ 今日：处理 / 过滤丢弃 / 推送 统计                         │
│  ├─ 模型调用次数 / Token 用量                                 │
│  ├─ 失败任务 + 原因 + 一键重试                                │
│  └─ 知识库增长曲线                                           │
└─────────────────────────────────────────────────────────────┘
```

---

## 视频内容处理

VideoPipeline 用于处理用户粘贴的 B站 / 抖音公开链接。入口层先从用户输入中抽取第一个 `http(s)` URL，再根据域名识别为 `ContentVideo`：

- B站：`bilibili.com`、`b23.tv`、`bili2233.cn`
- 抖音：`douyin.com`、`v.douyin.com`、`iesdouyin.com`

执行流程：

```text
解析链接 → yt-dlp 获取元数据 → 优先提取字幕 → 无字幕则下载音频 → ffmpeg 切片 → ASR 转写 → LLM 总结 → 入库 / 通知
```

当前实现把视频抓取封装在 `internal/infra/fetcher.VideoFetcher`，主 Pipeline 只依赖 `Fetcher` 接口。这样后续可以把 `yt-dlp + ffmpeg + ASR` 迁移到独立 `video-fetcher` sidecar，而不用改业务编排。

运行时配置：

- `YTDLP_BIN`：yt-dlp 可执行文件，默认 `yt-dlp`
- `FFMPEG_BIN`：ffmpeg 可执行文件，默认 `ffmpeg`
- `VIDEO_SUB_LANGS`：字幕语言优先级，默认中文优先、英文兜底
- `VIDEO_MAX_DURATION_SECONDS`：视频最长处理时长，默认 2 小时
- `ASR_BASE_URL` / `ASR_API_KEY` / `ASR_MODEL`：OpenAI-compatible `/audio/transcriptions` 配置；仅显式配置 ASR endpoint 时启用，模型默认 `whisper-1`
- `PLAYWRIGHT_DRIVER_PATH` / `PLAYWRIGHT_NODEJS_PATH` / `PLAYWRIGHT_CHROMIUM_EXECUTABLE`：浏览器渲染抓取配置；知乎链接默认使用 Playwright + Chromium 渲染后提取正文，Alpine 镜像使用系统 Node 执行 Playwright driver
- `YTDLP_COOKIES_FILE`：可选 Cookie 文件路径，推荐指向容器内 `/run/codo-secrets/*.txt`
- `YTDLP_COOKIES_FROM_BROWSER`：可选浏览器 Cookie 来源，直接透传给 `yt-dlp --cookies-from-browser`；生产默认预留 `./browser-profile:/run/codo-browser-profile:ro`
- `YTDLP_USER_AGENT` / `YTDLP_REFERER`：可选请求头覆盖；默认按 B站 / 抖音平台选择浏览器 UA 和 referer

授权态接入参考 `yt-dlp` / `gallery-dl` 等成熟下载器的通用模式：优先使用显式导出的 cookies 文件，必要时才读取用户授权的浏览器 profile。生产部署通过 `./secrets:/run/codo-secrets:ro` 只读挂载 Cookie 文件，通过 `./browser-profile:/run/codo-browser-profile:ro` 只读挂载浏览器 profile；`secrets/`、`browser-profile/`、`.env`、构建产物都被 Git / Docker 忽略，不写入日志、数据库或仓库。

合规边界：只处理用户提交的公开或已授权内容，不绕过 DRM、会员限制、私密内容或平台访问控制。

---

## 订阅源管理

订阅源采用轻量的管理模型：结构化字段仍保存在 `subscriptions.config` JSON 中，避免为不同来源的配置字段频繁迁移。稳定、高频、需要去重和提醒状态的检测结果写入 `source_items`。

当前支持：

- 添加 RSS / Atom Feed
- 添加学习通巡检源，使用学习通账号密码或 Cookie 授权
- 展示全部订阅源，包括已暂停订阅
- 标题和分组，用于类似 RSS 阅读器里的 folder/category 管理
- 启用 / 暂停自动巡检
- 手动刷新单个订阅源
- 删除订阅源
- 记录最近一次拉取错误，前端按健康状态展示

这个设计参考了成熟 RSS 阅读器的常见能力：Miniflux 的 categories / disabled feeds / OPML 思路、FreshRSS 的按分类查看、NetNewsWire 的 folders 和后台刷新、Tiny Tiny RSS 的 categories / labels。Codo 当前先实现“分组 + 健康状态 + 启停 + 手动刷新”，OPML 导入导出和复杂过滤规则留到订阅源规模变大后再加。

学习通源参考了 SuperStarInfoFetch 的课程 / 作业 / 考试字段抽象、chaoxing_qq_notification 的“抓取后入库并按截止时间提醒”流程，以及 xxt-unwork-push / xxt_work_notice 的未完成作业、24 小时内截止提醒思路。Codo 未复制这些项目代码，也未把它们作为运行依赖；当前使用 Go + goquery 独立实现登录、列表解析、去重入库和通知。

学习通配置在前台订阅源管理里完成：选择“学习通”后填写账号、密码或 Cookie、提前提醒小时数，并选择是否开启新作业 / 新考试提醒与临近截止提醒。后端只向前端返回 `password_configured` / `cookie_configured`，不回显密码或 Cookie。

学习通展示页借鉴 [Linkwarden](https://github.com/linkwarden/linkwarden) / [Karakeep](https://github.com/karakeep-app/karakeep) / [wallabag](https://github.com/wallabag/wallabag) 这类成熟个人信息管理项目的“概览统计 + 搜索 + 标签/状态筛选 + 紧凑列表”组织方式，但不引入它们作为运行依赖。Codo 的展示页只读取 `source_items` 中的作业 / 考试结果，负责查看待处理、临近截止、课程和状态，不暴露学习通密码或 Cookie。

---

## 核心表设计

```sql
-- 任务运行记录
tasks (id, user_id, source, content_type, url, raw_content,
       status, filter_decision, summary, error,
       created_at, updated_at)

-- 每步执行记录（看板数据来源）
task_steps (id, task_id, label, status, detail, duration_ms, created_at)

-- LLM 调用 Trace（可观测 + 成本追踪）
model_calls (id, task_id, step, model, input_tokens, output_tokens,
             latency_ms, error, created_at)

-- 知识库
articles (id, user_id, task_id, url, title, source, content_type,
          content, summary, category, tags, metadata jsonb,
          published_at, embedding vector(1536), created_at)

article_chunks (id, article_id, user_id, chunk_index, content,
                token_estimate, embedding vector(1536), created_at,
                updated_at)

-- 订阅源配置
subscriptions (id, user_id, source_type, config jsonb,
               last_fetched_at, enabled, created_at)

-- 来源检测结果（学习通作业 / 考试等）
source_items (id, user_id, subscription_id, source_type, item_type,
              external_id, course, title, status, url, due_at,
              payload jsonb, first_seen_at, last_seen_at,
              new_notified_at, due_notified_at,
              created_at, updated_at)

-- 用户配置（含过滤规则）
users (id, username, password_hash, auth_enabled, telegram_id,
       filter_keywords, notify_channel, model_policy jsonb,
       created_at, updated_at)

-- 单人工作台 session
auth_sessions (id, user_id, token_hash, expires_at,
               created_at, last_seen_at)

-- 实例级运行配置（LLM / Embedding / ASR / Telegram / SMTP）
app_settings (key, value jsonb, updated_at)

-- 日报发送记录（避免同一天重复发送）
daily_reports (id, user_id, report_date, status, item_count,
               last_error, sent_at, created_at, updated_at)
```

---

## 单人鉴权与配置管理

Codo 按单人单用设计，不维护角色、团队或复杂管理员系统。首次访问工作台时，如果 `DEFAULT_USER_ID` 对应用户还没有密码，会进入 owner setup，创建一个用户名和密码；之后所有 `/api/*` 与 `/ws` 都需要 `codo_session` HttpOnly Cookie。

密钥类配置可以在前台设置页编辑，包括 LLM、Embedding、ASR、Telegram 和 SMTP。API 响应只返回 `*_configured` 状态，不回显真实 token、password 或 API key；前端密钥输入框保存后清空，后续只能替换。服务器环境变量仍作为兜底配置，数据库中的显式配置优先。

---

## 知识库分类与标签

`content_type` 只表示处理方式，例如 `webpage` / `video` / `email`；主题归属不再增加新的内容类型，而是写入 `articles.category` 和 `articles.tags`。

例如用户提交政治新闻链接时，入口仍识别为 `webpage`，Pipeline 抓取正文并总结，然后分类器输出 `category=政治` 和若干短标签。前端知识库页通过 `/api/knowledge/facets` 聚合已有 `category/tags`，动态生成分类和标签筛选页，不需要提前写死“政治”“财经”“法律”等分类。

`articles.metadata` 用于承载平台、作者、站点、封面等来源特有信息；只有稳定、高频、需要索引或排序的字段才单独加列，例如 `published_at`。

---

## 知识库搜索与问答

入库内容会在 `SaveKnowledgeItem` 后同步生成 `article_chunks`。搜索接口 `/api/search` 先使用 PostgreSQL 的 `pg_trgm` 在切片正文、标题、摘要、分类、标签中做关键词召回；如果配置了 embedding 服务，并且 scheduler 已经为切片补齐向量，则再用 `pgvector` 做语义召回并合并排序。

问答接口 `/api/qa` 使用同一套召回结果构造 RAG 上下文，要求模型只依据引用片段回答。Embedding 配置来自前台运行配置或服务器环境变量；为避免隐式请求和费用，只有显式设置 Embedding key 时才启用向量生成。`EMBEDDING_BASE_URL` 为空时可复用 `LLM_BASE_URL`：

- `EMBEDDING_BASE_URL`
- `EMBEDDING_API_KEY`
- `EMBEDDING_MODEL`

未配置 embedding 时，搜索和问答仍可使用关键词召回；未配置 LLM 时，搜索可用，问答返回服务未配置错误。

---

## 日报邮件推送

日报由 `cmd/scheduler` 定时检查，使用 `internal/application/report` 聚合当天已入库的 `articles` 摘要，按分类生成纯文本邮件，并通过 `internal/infra/notify.Email` 发送。单条内容处理仍走 Pipeline 的即时通知策略，日报是独立的汇总用例。

用户可在前台设置：

- 是否启用日报
- 收件邮箱
- 本地发送小时
- 时区
- 最大条目数

SMTP 连接配置可在前台设置页维护，也可由服务器环境变量兜底。密码只允许写入或替换，接口不回显真实值。

`daily_reports` 表使用 `(user_id, report_date)` 唯一约束记录 `sent` / `skipped` / `failed` 状态，确保同一天不会重复发送；失败状态允许后续 scheduler 周期重试。

---

## LLM 接入配置

统一通过 OpenAI-compatible 中转站接入，支持任意兼容模型：

```go
// infra/llm/client.go
cfg := llm.Config{
    BaseURL: "https://api.your-relay.com/v1", // 中转站地址
    APIKey:  "your-key",
    Model:   "claude-opus-4-7",               // 中转站映射的模型名
}
client := llm.NewClient(cfg)

// 同一个 client 实现所有 pipeline 所需接口
// Filterer / Summarizer / Classifier / Extractor
```

配置项通过环境变量注入，不写死在代码里：

```yaml
# config.example.yaml
llm:
  base_url: ${LLM_BASE_URL}
  api_key:  ${LLM_API_KEY}
  model:    ${LLM_MODEL:-claude-opus-4-7}
  # 轻量任务（filter/classify）可指定更便宜的模型
  light_model: ${LLM_LIGHT_MODEL:-claude-haiku-4-5}
```

---

## 模型路由策略

```go
// 不同任务使用不同模型，成本与质量平衡
type ModelPolicy struct {
    FilterModel    string // 轻量：haiku / gpt-4o-mini
    SummarizeModel string // 主力：opus / gpt-4.1
    TranslateModel string // 主力：opus / gpt-4.1
    EmbedModel     string // text-embedding-3-small
}

var DefaultPolicy = ModelPolicy{
    FilterModel:    "claude-haiku-4-5",
    SummarizeModel: "claude-opus-4-7",
    TranslateModel: "claude-opus-4-7",
    EmbedModel:     "text-embedding-3-small",
}
```

---

## Pipeline 接口设计

```go
// 每种内容类型对应一个 Pipeline 实现
type Pipeline interface {
    ContentType() domain.ContentType
    Run(ctx context.Context, task *domain.Task) error
}

// 路由器根据内容类型分发
type Router struct {
    pipelines map[domain.ContentType]Pipeline
}

func (r *Router) Run(ctx context.Context, task *domain.Task) error {
    p, ok := r.pipelines[task.ContentType]
    if !ok {
        return fmt.Errorf("no pipeline for %s", task.ContentType)
    }
    return p.Run(ctx, task)
}
```

---

## 过滤层设计

```
内容进入过滤层
      │
      ├─ 快速路径（无需 LLM）
      │   ├─ URL 已在知识库？→ 丢弃
      │   ├─ 正文 < 200 字？→ 丢弃
      │   └─ 用户关键词黑名单命中？→ 丢弃
      │
      └─ LLM 判断（轻量模型，控制成本）
          ├─ 内容质量评估 → 低质量 → 丢弃
          ├─ 与用户兴趣匹配度 → 不匹配 → 降级（存库不推送）
          └─ 通过 → 进入摘要生成
```

过滤三种结果：
- **discard**：完全丢弃，不写库，节省存储和推送成本
- **silent**：存入知识库，不推送，用户主动查询时能找到
- **pass**：存库 + 推送摘要

---

## 可扩展路径

```
v1  WebPagePipeline + Telegram Bot + 看板（主链路跑通）
v2  + ModelRouter + Trace 落库（可观测）
v3  + RSS / 公众号订阅（Scheduler + Source 插件）
v4  + VideoPipeline（B站/抖音）
v5  + EmailPipeline + MessagePipeline
```

每个版本新增一个 Pipeline 或 Source 实现，不改已有结构。

---

## 部署

```
本地：docker-compose（api + worker + scheduler + postgres + redis + playwright）
生产：Railway，每个服务独立容器，worker 可多实例水平扩展
CI/CD：GitHub Actions → 构建镜像 → Railway 滚动部署
```
