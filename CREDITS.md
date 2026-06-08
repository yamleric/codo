# 致谢与开源参考目录

Codo 是一个开源非盈利项目。开发过程中参考了许多优秀开源项目的架构、产品组织和实现方式，也直接使用了一批开源依赖。感谢这些项目的作者和贡献者。

本文件是项目统一的“致谢、依赖、参考项目”目录。

- **已接入依赖**：Codo 运行或构建时实际使用的库、工具或镜像。
- **参考项目**：没有复制源码、没有作为运行依赖引入，但在设计和实现方案上参考过。
- **候选依赖**：未来可能引入的项目，不代表当前已经使用。

---

## 目录

- [已接入的开源依赖](#已接入的开源依赖)
- [架构与 Agent 参考](#架构与-agent-参考)
- [订阅源与内容入口参考](#订阅源与内容入口参考)
- [收藏、阅读和知识管理参考](#收藏阅读和知识管理参考)
- [学习通作业考试提醒参考](#学习通作业考试提醒参考)
- [视频抓取和授权态参考](#视频抓取和授权态参考)
- [邮件助理参考](#邮件助理参考)
- [偏好记忆和个人知识库参考](#偏好记忆和个人知识库参考)
- [英文资料翻译参考](#英文资料翻译参考)
- [候选开源依赖](#候选开源依赖)

---

## 已接入的开源依赖

| 模块 | 项目 / 仓库 | 当前用途 |
| --- | --- | --- |
| 通知 | [go-telegram-bot-api/telegram-bot-api](https://github.com/go-telegram-bot-api/telegram-bot-api) | Telegram 消息推送 |
| RSS | [mmcdole/gofeed](https://github.com/mmcdole/gofeed) | RSS / Atom / JSON Feed 解析 |
| 网页正文 | [readeck/go-readability](https://codeberg.org/readeck/go-readability) | 普通网页正文提取 |
| HTML 解析 | [PuerkitoBio/goquery](https://github.com/PuerkitoBio/goquery) | HTML 选择器解析；用于网页、学习通、部分来源页面解析 |
| HTML 清洗 | [microcosm-cc/bluemonday](https://github.com/microcosm-cc/bluemonday) | HTML 内容安全清洗 |
| 浏览器渲染 | [playwright-community/playwright-go](https://github.com/playwright-community/playwright-go) | 部分 JS 渲染页面的浏览器自动化抓取；当前不代表支持知乎 |
| 视频获取 | [yt-dlp/yt-dlp](https://github.com/yt-dlp/yt-dlp) | B站 / 抖音公开视频元数据、字幕和音频获取 |
| 音频处理 | [FFmpeg/FFmpeg](https://github.com/FFmpeg/FFmpeg) | 视频音频提取、转码和 ASR 前处理 |
| 邮件读取 | [emersion/go-imap](https://github.com/emersion/go-imap) | IMAP 只读同步收件箱 |
| 邮件解析 | [emersion/go-message](https://github.com/emersion/go-message) | MIME 邮件正文解析 |
| PostgreSQL | [jackc/pgx](https://github.com/jackc/pgx) | PostgreSQL 驱动与连接池 |
| 向量检索 | [pgvector/pgvector](https://github.com/pgvector/pgvector) | PostgreSQL 向量字段和相似度检索 |
| 去重 | [cespare/xxhash](https://github.com/cespare/xxhash) | URL hash，用于快速去重 |
| 实时看板 | [gorilla/websocket](https://github.com/gorilla/websocket) | 任务状态 WebSocket 推送 |
| 前端框架 | [vuejs/core](https://github.com/vuejs/core) | Web 工作台 UI |
| 前端构建 | [vitejs/vite](https://github.com/vitejs/vite) | 前端开发服务器和生产构建 |
| 前端样式 | [tailwindlabs/tailwindcss](https://github.com/tailwindlabs/tailwindcss) | 工作台样式系统 |
| 前端请求 | [axios/axios](https://github.com/axios/axios) | 浏览器端 HTTP API 调用 |
| 前端图标 | [lucide-icons/lucide](https://github.com/lucide-icons/lucide) | 工作台图标 |

---

## 架构与 Agent 参考

| 项目 | 参考内容 | 接入方式 |
| --- | --- | --- |
| [LangGraph](https://github.com/langchain-ai/langgraph) | 确定性工作流、状态持久化、节点编排思路 | 参考架构，不作为运行依赖 |
| [OpenAI Agents SDK](https://github.com/openai/openai-agents-python) | Agent 抽象、工具调用、guardrails 组织方式 | 参考抽象，不作为运行依赖 |
| [OpenClaw](https://github.com/openclaw/openclaw) | 多渠道消息接入和节点系统思路 | 参考产品架构，不作为运行依赖 |
| [Hermes Agent](https://github.com/NousResearch/hermes-agent) | 技能沉淀、工具调用和持久记忆思路 | 参考设计，不作为运行依赖 |

---

## 订阅源与内容入口参考

| 项目 | 参考内容 | 接入方式 |
| --- | --- | --- |
| [RSSHub](https://github.com/DIYgod/RSSHub) | 把网站、公众号、社区内容转换成 RSS 的模式 | 推荐作为外部 RSS 来源，不内嵌 |
| [Miniflux](https://github.com/miniflux/v2) | feed 分类、禁用、健康状态、OPML 思路 | 参考订阅源管理 |
| [FreshRSS](https://github.com/FreshRSS/FreshRSS) | RSS 阅读器的分类、源管理和阅读状态组织 | 参考产品组织 |
| [NetNewsWire](https://github.com/Ranchero-Software/NetNewsWire) | folders、订阅列表和阅读体验 | 参考交互组织 |
| [Tiny Tiny RSS](https://git.tt-rss.org/fox/tt-rss) | categories、labels、过滤规则 | 参考后续订阅规则设计 |

---

## 收藏、阅读和知识管理参考

| 项目 | 参考内容 | 接入方式 |
| --- | --- | --- |
| [Linkwarden](https://github.com/linkwarden/linkwarden) | 书签管理、标签、集合和可视化列表 | 参考收藏夹和展示页，不作为运行依赖 |
| [Karakeep](https://github.com/karakeep-app/karakeep) | 个人信息收藏、AI 摘要、搜索和标签组织 | 参考产品形态，不作为运行依赖 |
| [wallabag](https://github.com/wallabag/wallabag) | 稍后读、文章保存和阅读视图 | 参考阅读体验，不作为运行依赖 |
| [Readeck](https://codeberg.org/readeck/readeck) | 个人网页归档、正文抽取和阅读管理 | 参考知识库内容展示；Codo 仅接入其 Go readability 库 |

---

## 学习通作业考试提醒参考

| 项目 | 参考内容 | 接入方式 |
| --- | --- | --- |
| [LuckyTain/SuperStarInfoFetch](https://github.com/LuckyTain/SuperStarInfoFetch) | 学习通课程、作业、考试字段抽象，以及剩余时间 / 有效性过滤思路 | 仅作实现参考，不复制源码 |
| [songhahaha66/chaoxing_qq_notification](https://github.com/songhahaha66/chaoxing_qq_notification) | 作业入库、状态更新、临近截止提醒和前后端分离流程 | 仅作流程参考 |
| [Gngzs/xxt-unwork-push](https://github.com/Gngzs/xxt-unwork-push) | 每日未完成作业汇总和 24 小时内截止提醒策略 | 仅作提醒策略参考 |
| [xsk666/xxt_work_notice](https://github.com/xsk666/xxt_work_notice) | 作业 / 考试列表接口和最小提醒脚本结构 | 仅作接口形态参考 |
| [Marshmellond/XuexitongJob](https://github.com/Marshmellond/XuexitongJob) | 学习通每日作业提醒和邮件通知思路 | 仅作通知流程参考 |

---

## 视频抓取和授权态参考

| 项目 | 参考内容 | 接入方式 |
| --- | --- | --- |
| [yt-dlp](https://github.com/yt-dlp/yt-dlp) | 多平台视频元数据、字幕、音频获取 | 已作为运行工具接入 |
| [gallery-dl](https://github.com/mikf/gallery-dl) | 显式 cookies 文件和 browser cookies 来源配置模式 | 仅作授权态配置参考 |
| [FFmpeg](https://github.com/FFmpeg/FFmpeg) | 音频提取、转码、切片 | 已作为运行工具接入 |

---

## 邮件助理参考

| 项目 | 参考内容 | 接入方式 |
| --- | --- | --- |
| [emersion/go-imap](https://github.com/emersion/go-imap) | IMAP 只读同步收件箱 | 已作为依赖接入 |
| [emersion/go-message](https://github.com/emersion/go-message) | MIME 邮件解析、纯文本 / HTML 正文提取 | 已作为依赖接入 |
| [jhillyerd/enmime](https://github.com/jhillyerd/enmime) | 邮件 MIME 解析和附件处理 API 设计 | 仅作备选参考 |

---

## 偏好记忆和个人知识库参考

| 项目 | 参考内容 | 接入方式 |
| --- | --- | --- |
| [Dify](https://github.com/langgenius/dify) | 反馈、标注日志和应用配置组织 | 参考产品机制，不作为运行依赖 |
| [Open WebUI](https://github.com/open-webui/open-webui) | 可见、可编辑、可删除的用户记忆 | 参考记忆管理体验 |
| [LangMem](https://github.com/langchain-ai/langmem) | semantic / episodic / procedural memory 拆分 | 参考记忆类型抽象 |
| [Mem0](https://github.com/mem0ai/mem0) | 长期记忆抽象和记忆更新流程 | 参考记忆服务设计 |
| [Khoj](https://github.com/khoj-ai/khoj) | 个人知识库、检索和问答体验 | 参考知识问答产品 |
| [Letta](https://github.com/letta-ai/letta) | 长期记忆 Agent 和工具使用 | 参考 Agent 记忆设计 |
| [Graphiti](https://github.com/getzep/graphiti) | 时序知识图谱和记忆演化 | 参考后续记忆演化方向 |

---

## 英文资料翻译参考

| 项目 | 参考内容 | 接入方式 |
| --- | --- | --- |
| [Zotero PDF Translate](https://github.com/windingwind/zotero-pdf-translate) | 文献阅读场景下的段落翻译、双语资料管理 | 参考产品体验 |
| [LibreTranslate](https://github.com/LibreTranslate/LibreTranslate) | 自部署翻译服务 API 形态 | 未来可作为可选翻译后端 |
| [Argos Translate](https://github.com/argosopentech/argos-translate) | 本地离线翻译模型和包管理 | 未来可作为离线翻译参考 |
| [PDFMathTranslate](https://github.com/Byaidu/PDFMathTranslate) | 保留文档结构的翻译处理 | 参考长文档翻译方向 |

---

## 候选开源依赖

> 下面项目不代表当前已经接入。真正引入前需要确认许可证、维护状态、部署成本和是否能被现有接口隔离。

| 模块 | 候选项目 | 适用场景 | 引入建议 |
| --- | --- | --- | --- |
| LLM SDK | [openai/openai-go](https://github.com/openai/openai-go) | 替代当前手写 HTTP 请求，统一调用 OpenAI-compatible 接口 | 可接入到 `internal/infra/llm`，保留 BaseURL 配置 |
| 任务队列 | [riverqueue/river](https://github.com/riverqueue/river) | Postgres 事务内入队，任务状态与业务数据保持一致 | 若继续以 Postgres 为主存储，优先考虑 |
| 任务队列 | [hibiken/asynq](https://github.com/hibiken/asynq) | Redis 后台任务队列、重试、延迟任务 | 若确定 Redis 是核心队列，再考虑 |
| 定时任务 | [robfig/cron](https://github.com/robfig/cron) | 替代固定 ticker，管理 RSS 拉取、日报、提醒等 cron 任务 | 适合轻量 scheduler |
| 浏览器抓取 | [chromedp/chromedp](https://github.com/chromedp/chromedp) | 基于 Chrome DevTools Protocol 的 Go 原生自动化 | 比 Playwright 轻，复杂站点兼容性需验证 |
| 爬虫框架 | [gocolly/colly](https://github.com/gocolly/colly) | 规则化抓取网站列表页、分页、详情页 | 适合结构化 source 插件 |
| 反爬兜底 | [FlareSolverr](https://github.com/FlareSolverr/FlareSolverr) | Cloudflare 防护页面兜底抓取 | 作为外部服务接入，不建议作为默认路径 |
| 本地 ASR | [SYSTRAN/faster-whisper](https://github.com/SYSTRAN/faster-whisper) | 不走外部 ASR API 时本地转写音频 | 需要模型文件和较重运行时，适合 sidecar 化 |
| URL 提取 | [mvdan/xurls](https://github.com/mvdan/xurls) | 从消息、邮件、纯文本中提取 URL | 适合后续聊天入口和群消息处理 |
| 配置 | [caarlos0/env](https://github.com/caarlos0/env) | 把环境变量解析到结构体 | 比散落 getenv 更适合后续配置增长 |
| 迁移 | [golang-migrate/migrate](https://github.com/golang-migrate/migrate) | 数据库 schema 版本化迁移 | 适合生产部署演进 |
| 测试 | [stretchr/testify](https://github.com/stretchr/testify) | 单元测试断言和 mock 辅助 | 适合补 pipeline/store 测试 |
| 集成测试 | [testcontainers-go](https://github.com/testcontainers/testcontainers-go) | 测试中启动 Postgres 等依赖 | 适合验证存储、迁移和端到端流程 |

---

## 致谢说明

Codo 尊重各依赖库和参考项目的原始许可证。本项目没有复制参考项目源码；如有遗漏、链接错误或授权信息不准确，欢迎提交 Issue 或 PR 更正。

> This project is non-commercial and open source. All referenced projects retain their original licenses.
