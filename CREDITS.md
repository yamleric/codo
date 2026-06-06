# 致谢 / Credits

Codo 是一个开源非盈利项目，在开发过程中参考和借鉴了以下优秀的开源项目。感谢这些项目的作者和贡献者。

---

## 架构参考

<table border="1" cellpadding="8" cellspacing="0">
  <tr><th>项目</th><th>仓库</th><th>参考内容</th></tr>
  <tr><td>DEEIX-Chat</td><td>—</td><td>分层架构（domain / application / infra / transport）、模型路由设计、配置分离思路</td></tr>
  <tr><td>LangGraph</td><td><a href="https://github.com/langchain-ai/langgraph">langchain-ai/langgraph</a></td><td>确定性工作流 + 状态持久化思路</td></tr>
  <tr><td>OpenAI Agents SDK</td><td><a href="https://github.com/openai/openai-agents-python">openai/openai-agents-python</a></td><td>最小 Agent 抽象：instructions + tools + guardrails</td></tr>
  <tr><td>OpenClaw</td><td><a href="https://github.com/openclaw/openclaw">openclaw/openclaw</a></td><td>多渠道消息接入架构、节点系统设计</td></tr>
  <tr><td>Hermes Agent</td><td><a href="https://github.com/NousResearch/hermes-agent">NousResearch/hermes-agent</a></td><td>技能沉淀系统、持久记忆设计</td></tr>
</table>

---

## 专项实现参考

<table border="1" cellpadding="8" cellspacing="0">
  <tr><th>项目</th><th>许可证</th><th>参考内容</th><th>接入方式</th></tr>
  <tr>
    <td><a href="https://github.com/huanjuedadehen/wechat-article-parser">wechat-article-parser</a></td>
    <td>MIT</td>
    <td>微信公众号文章类型识别、验证页检测、元数据与多种正文容器提取策略</td>
    <td>作为实现参考；Codo 使用 Go + goquery 独立实现，不引入 Python 运行时</td>
  </tr>
</table>

---

## 已接入的开源依赖

<table border="1" cellpadding="8" cellspacing="0">
  <tr><th>模块</th><th>项目 / 仓库</th><th>当前用途</th></tr>
  <tr><td>通知</td><td><a href="https://github.com/go-telegram-bot-api/telegram-bot-api">go-telegram-bot-api/telegram-bot-api</a></td><td>Telegram 消息推送</td></tr>
  <tr><td>订阅源</td><td><a href="https://github.com/mmcdole/gofeed">mmcdole/gofeed</a></td><td>RSS / Atom / JSON Feed 统一解析</td></tr>
  <tr><td>网页抓取</td><td><a href="https://codeberg.org/readeck/go-readability">readeck/go-readability</a></td><td>网页正文提取</td></tr>
  <tr><td>浏览器抓取</td><td><a href="https://github.com/playwright-community/playwright-go">playwright-community/playwright-go</a></td><td>知乎等 JS 渲染页面的浏览器自动化抓取</td></tr>
  <tr><td>安全清洗</td><td><a href="https://github.com/microcosm-cc/bluemonday">microcosm-cc/bluemonday</a></td><td>HTML 安全清洗，入库 / 展示前 sanitize</td></tr>
  <tr><td>网页解析</td><td><a href="https://github.com/PuerkitoBio/goquery">PuerkitoBio/goquery</a></td><td>HTML 选择器解析；当前用于微信公众号专用正文提取</td></tr>
  <tr><td>视频内容</td><td><a href="https://github.com/yt-dlp/yt-dlp">yt-dlp/yt-dlp</a></td><td>B站 / 抖音公开视频元数据、字幕和音频下载；cookies 文件和浏览器 cookies 来源配置</td></tr>
  <tr><td>视频授权态参考</td><td><a href="https://github.com/mikf/gallery-dl">gallery-dl/gallery-dl</a></td><td>参考其显式 cookies / browser cookies 配置模式，不作为运行依赖</td></tr>
  <tr><td>音频处理</td><td><a href="https://github.com/FFmpeg/FFmpeg">FFmpeg/FFmpeg</a></td><td>视频音频提取、转码和 ASR 切片</td></tr>
  <tr><td>数据存储</td><td><a href="https://github.com/jackc/pgx">jackc/pgx</a></td><td>PostgreSQL 驱动与连接池</td></tr>
  <tr><td>知识库</td><td><a href="https://github.com/pgvector/pgvector">pgvector/pgvector</a></td><td>PostgreSQL 向量字段和向量索引</td></tr>
  <tr><td>去重</td><td><a href="https://github.com/cespare/xxhash">cespare/xxhash</a></td><td>URL / 内容 hash，用于快速去重</td></tr>
  <tr><td>实时看板</td><td><a href="https://github.com/gorilla/websocket">gorilla/websocket</a></td><td>任务状态 WebSocket 推送</td></tr>
  <tr><td>前端</td><td><a href="https://github.com/vuejs/core">vuejs/core</a></td><td>Web 看板 UI 框架</td></tr>
  <tr><td>前端构建</td><td><a href="https://github.com/vitejs/vite">vitejs/vite</a></td><td>前端开发服务器与生产构建</td></tr>
  <tr><td>前端样式</td><td><a href="https://github.com/tailwindlabs/tailwindcss">tailwindlabs/tailwindcss</a></td><td>看板样式系统</td></tr>
  <tr><td>前端请求</td><td><a href="https://github.com/axios/axios">axios/axios</a></td><td>浏览器端 HTTP API 调用</td></tr>
</table>

---

## 候选开源依赖

> 下面是按模块推荐的可选依赖，不代表当前已经接入。真正引入前应确认许可证、维护状态、部署成本和是否能被现有接口隔离。

<table border="1" cellpadding="8" cellspacing="0">
  <tr><th>模块</th><th>候选项目</th><th>适用场景</th><th>引入建议</th></tr>
  <tr><td>LLM SDK</td><td><a href="https://github.com/openai/openai-go">openai/openai-go</a></td><td>替代当前手写 HTTP 请求，统一调用 OpenAI-compatible 接口</td><td>优先接入到 infra/llm，保留 BaseURL 配置</td></tr>
  <tr><td>LLM SDK</td><td><a href="https://github.com/anthropics/anthropic-sdk-go">anthropic-sdk-go</a></td><td>直连 Claude API，使用 Anthropic 原生能力</td><td>仅在不走 OpenAI-compatible 中转站时引入</td></tr>
  <tr><td>任务队列</td><td><a href="https://github.com/riverqueue/river">riverqueue/river</a></td><td>Postgres 事务内入队，任务状态与业务数据保持一致</td><td>若继续以 Postgres 为主存储，优先考虑</td></tr>
  <tr><td>任务队列</td><td><a href="https://github.com/hibiken/asynq">hibiken/asynq</a></td><td>Redis 后台任务队列、重试、延迟任务</td><td>若确定 Redis 是核心队列，再考虑</td></tr>
  <tr><td>定时任务</td><td><a href="https://github.com/robfig/cron">robfig/cron</a></td><td>替代固定 ticker，管理 RSS 拉取、日报、提醒等 cron 任务</td><td>适合轻量 scheduler</td></tr>
  <tr><td>浏览器抓取</td><td><a href="https://github.com/playwright-community/playwright-go">playwright-go</a></td><td>JS 渲染、登录态页面、滚动懒加载</td><td>功能完整但镜像更重，适合独立 fetcher 服务</td></tr>
  <tr><td>浏览器抓取</td><td><a href="https://github.com/chromedp/chromedp">chromedp/chromedp</a></td><td>基于 Chrome DevTools Protocol 的 Go 原生自动化</td><td>比 Playwright 轻，复杂站点兼容性需验证</td></tr>
  <tr><td>爬虫框架</td><td><a href="https://github.com/gocolly/colly">gocolly/colly</a></td><td>规则化抓取网站列表页、分页、详情页</td><td>适合 linux.do 等结构化 source 插件</td></tr>
  <tr><td>反爬兜底</td><td><a href="https://github.com/FlareSolverr/FlareSolverr">FlareSolverr/FlareSolverr</a></td><td>Cloudflare 防护页面兜底抓取</td><td>作为外部服务接入，不建议作为默认路径</td></tr>
  <tr><td>本地 ASR</td><td><a href="https://github.com/SYSTRAN/faster-whisper">SYSTRAN/faster-whisper</a></td><td>不走外部 ASR API 时，在独立 video-fetcher 服务内本地转写音频</td><td>需要模型文件和较重运行时，适合后续 sidecar 化</td></tr>
  <tr><td>订阅源</td><td><a href="https://github.com/DIYgod/RSSHub">DIYgod/RSSHub</a></td><td>把网站、公众号、社区内容转换成 RSS</td><td>建议作为外部 source，而不是耦合进主服务</td></tr>
  <tr><td>邮件</td><td><a href="https://github.com/emersion/go-imap">emersion/go-imap</a></td><td>IMAP 邮件拉取</td><td>接入 EmailPipeline 前再引入</td></tr>
  <tr><td>邮件</td><td><a href="https://github.com/jhillyerd/enmime">jhillyerd/enmime</a></td><td>邮件 MIME 解析、正文和附件提取</td><td>与 IMAP source 配套使用</td></tr>
  <tr><td>URL 提取</td><td><a href="https://github.com/mvdan/xurls">mvdan/xurls</a></td><td>从消息、邮件、纯文本中提取 URL</td><td>适合 Telegram Bot 输入和群消息处理</td></tr>
  <tr><td>配置</td><td><a href="https://github.com/caarlos0/env">caarlos0/env</a></td><td>把环境变量解析到结构体</td><td>比散落 getenv 更适合后续配置增长</td></tr>
  <tr><td>迁移</td><td><a href="https://github.com/golang-migrate/migrate">golang-migrate/migrate</a></td><td>数据库 schema 版本化迁移</td><td>替代单个 init.sql，适合生产部署</td></tr>
  <tr><td>向量存储</td><td><a href="https://github.com/pgvector/pgvector-go">pgvector/pgvector-go</a></td><td>Go 侧读写 pgvector 类型</td><td>接入 embedding 写入时使用</td></tr>
  <tr><td>可观测性</td><td><a href="https://github.com/open-telemetry/opentelemetry-go">open-telemetry/opentelemetry-go</a></td><td>HTTP、DB、LLM 调用链路追踪</td><td>先从 model_calls 表做轻量 trace，再按需接入</td></tr>
  <tr><td>指标</td><td><a href="https://github.com/prometheus/client_golang">prometheus/client_golang</a></td><td>任务量、失败率、LLM 延迟、token 用量指标</td><td>适合需要监控面板时引入</td></tr>
  <tr><td>测试</td><td><a href="https://github.com/stretchr/testify">stretchr/testify</a></td><td>单元测试断言和 mock 辅助</td><td>适合补 pipeline/store 测试</td></tr>
  <tr><td>集成测试</td><td><a href="https://github.com/testcontainers/testcontainers-go">testcontainers/testcontainers-go</a></td><td>测试中启动 Postgres、Redis 等依赖</td><td>适合验证存储、队列、迁移</td></tr>
  <tr><td>前端组件</td><td><a href="https://github.com/radix-vue/reka-ui">radix-vue/reka-ui</a></td><td>Vue 无样式可访问组件，适合菜单、弹窗、选择器</td><td>等看板控件复杂后再引入</td></tr>
  <tr><td>前端图标</td><td><a href="https://github.com/lucide-icons/lucide">lucide-icons/lucide</a></td><td>统一图标库</td><td>适合任务状态、操作按钮、来源类型图标</td></tr>
</table>

---

## 致谢说明

本项目遵循各依赖库的开源协议。如有遗漏或信息有误，欢迎提交 Issue 或 PR 更正。

> This project is non-commercial and open source. All referenced projects retain their original licenses.
