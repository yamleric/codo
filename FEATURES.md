# Codo 功能需求文档

> 核心定位：你只管收藏，Codo 把信息消费的时间成本归零。

---

## 功能清单

### 1. 微信公众号爬取总结

<table border="1" cellpadding="8" cellspacing="0">
  <tr><th>项目</th><th>说明</th></tr>
  <tr><td>需求</td><td>自动获取用户星标公众号的新文章，AI 总结后推送</td></tr>
  <tr><td>数据来源</td><td>RSSHub（自建）定时拉取星标公众号 RSS</td></tr>
  <tr><td>处理方式</td><td>抓取正文 → Claude API 生成 300 字摘要 → 存知识库 → 推送</td></tr>
  <tr><td>通知渠道</td><td>Telegram / 微信公众号回复</td></tr>
  <tr><td>输出示例</td><td>📄 标题 / 来源 / 摘要 / 原文链接</td></tr>
</table>

---

### 2. B站 / 抖音视频总结

<table border="1" cellpadding="8" cellspacing="0">
  <tr><th>项目</th><th>说明</th></tr>
  <tr><td>需求</td><td>发送视频链接，自动生成内容摘要，无需观看全片</td></tr>
  <tr><td>B站</td><td>通过 API 获取字幕，无字幕则 AI 语音转文字</td></tr>
  <tr><td>抖音</td><td>获取视频文字稿或语音识别提取内容</td></tr>
  <tr><td>处理方式</td><td>获取文字稿 → 分段处理 → 合并摘要 → 存知识库 → 推送</td></tr>
  <tr><td>输出示例</td><td>🎬 标题 / 时长 / 内容摘要 / 关键要点列表</td></tr>
</table>

---

### 3. 学习通作业提醒

<table border="1" cellpadding="8" cellspacing="0">
  <tr><th>项目</th><th>说明</th></tr>
  <tr><td>需求</td><td>自动检测新作业和截止日期，邮件 + 推送双渠道提醒</td></tr>
  <tr><td>授权方式</td><td>Playwright 扫码登录，保存登录态</td></tr>
  <tr><td>检测频率</td><td>每天定时检测一次</td></tr>
  <tr><td>提醒时机</td><td>发现新作业立即提醒；截止前 24 小时再次提醒</td></tr>
  <tr><td>通知渠道</td><td>Telegram 推送 + 发送邮件</td></tr>
</table>

---

### 4. 邮件总结

<table border="1" cellpadding="8" cellspacing="0">
  <tr><th>项目</th><th>说明</th></tr>
  <tr><td>需求</td><td>自动拉取邮件，AI 分类总结，每日推送摘要</td></tr>
  <tr><td>支持协议</td><td>IMAP（Gmail、QQ邮箱、企业邮箱）</td></tr>
  <tr><td>分类</td><td>重要（需回复）/ 通知（账单、验证码）/ 垃圾（广告）</td></tr>
  <tr><td>推送策略</td><td>重要邮件立即推送；其余每日汇总一次</td></tr>
  <tr><td>输出示例</td><td>📬 邮件日报 / 重要邮件摘要 / 今日统计</td></tr>
</table>

---

### 5. linux.do 收藏内容总结

<table border="1" cellpadding="8" cellspacing="0">
  <tr><th>项目</th><th>说明</th></tr>
  <tr><td>需求</td><td>自动同步 linux.do 收藏帖子，AI 总结后入知识库</td></tr>
  <tr><td>授权方式</td><td>Playwright 授权登录，保存登录态</td></tr>
  <tr><td>同步频率</td><td>每小时同步一次收藏列表</td></tr>
  <tr><td>处理方式</td><td>抓取帖子正文及精华回复 → 生成摘要 → 存知识库</td></tr>
  <tr><td>推送策略</td><td>可选：静默入库 或 推送通知</td></tr>
</table>

---

### 6. QQ 群消息总结

<table border="1" cellpadding="8" cellspacing="0">
  <tr><th>项目</th><th>说明</th></tr>
  <tr><td>需求</td><td>自动获取 QQ 群消息，提取关键信息推送</td></tr>
  <tr><td>授权方式</td><td>用户自行授权接入指定群</td></tr>
  <tr><td>汇总频率</td><td>每 6 小时 或 每天一次</td></tr>
  <tr><td>过滤规则</td><td>过滤表情包、灌水、广告；保留通知、决策、文件分享</td></tr>
  <tr><td>输出示例</td><td>💬 群名 / 关键信息列表 / 今日消息统计</td></tr>
</table>

---

### 7. 微信群消息总结

<table border="1" cellpadding="8" cellspacing="0">
  <tr><th>项目</th><th>说明</th></tr>
  <tr><td>需求</td><td>自动获取微信群消息，提取关键内容推送</td></tr>
  <tr><td>接入方式</td><td>用户授权，仅个人自用</td></tr>
  <tr><td>监听范围</td><td>用户指定的群</td></tr>
  <tr><td>过滤规则</td><td>过滤表情包、语音、广告、灌水；保留通知、决策、文件</td></tr>
  <tr><td>输出示例</td><td>💬 群名 / 关键信息列表 / 今日消息统计</td></tr>
</table>

---

### 8. 收藏网页自动爬取总结

<table border="1" cellpadding="8" cellspacing="0">
  <tr><th>项目</th><th>说明</th></tr>
  <tr><td>需求</td><td>发送任意网页链接，自动抓取正文并生成摘要</td></tr>
  <tr><td>策略一</td><td>静态页面 → 直接 HTTP 请求，速度快</td></tr>
  <tr><td>策略二</td><td>JS 渲染 / 懒加载 → Playwright 无头浏览器，模拟滚动</td></tr>
  <tr><td>策略三</td><td>CF 防护页面 → Flaresolverr 处理后获取内容</td></tr>
  <tr><td>策略四</td><td>需登录页面 → 用户授权，云端浏览器持登录态访问</td></tr>
  <tr><td>输出示例</td><td>📄 标题 / 来源 / 摘要 / 原文链接</td></tr>
</table>

---

### 9. 英文资料自动翻译

<table border="1" cellpadding="8" cellspacing="0">
  <tr><th>项目</th><th>说明</th></tr>
  <tr><td>需求</td><td>发送英文网页或文档链接，自动翻译成中文</td></tr>
  <tr><td>触发方式</td><td>发 URL → 抓正文 → 翻译；发文字 → 直接翻译</td></tr>
  <tr><td>处理方式</td><td>长文档分段翻译后合并，保留原文结构</td></tr>
  <tr><td>存储</td><td>翻译结果存入知识库（中文版）</td></tr>
  <tr><td>输出示例</td><td>🌐 原文标题 / 来源 / 中文译文</td></tr>
</table>

---

### 10. Agent 工作面板

<table border="1" cellpadding="8" cellspacing="0">
  <tr><th>项目</th><th>说明</th></tr>
  <tr><td>需求</td><td>可视化查看 Agent 工作状态、任务进度、历史记录</td></tr>
  <tr><td>访问方式</td><td>Web 网页，手机和电脑浏览器均可访问</td></tr>
  <tr><td>实时状态</td><td>WebSocket 推送当前执行任务及每步进度</td></tr>
  <tr><td>历史记录</td><td>已完成 / 失败任务列表，失败任务支持一键重试</td></tr>
  <tr><td>统计看板</td><td>今日处理条数 / 知识库总量 / 各来源分布</td></tr>
</table>

---

## 优先级

<table border="1" cellpadding="8" cellspacing="0">
  <tr><th>优先级</th><th>功能</th></tr>
  <tr><td>P0</td><td>收藏网页自动爬取总结</td></tr>
  <tr><td>P0</td><td>Agent 工作面板</td></tr>
  <tr><td>P0</td><td>Telegram Bot 接入（输入 + 通知渠道）</td></tr>
  <tr><td>P1</td><td>微信公众号爬取总结</td></tr>
  <tr><td>P1</td><td>英文资料自动翻译</td></tr>
  <tr><td>P1</td><td>学习通作业提醒</td></tr>
  <tr><td>P2</td><td>B站 / 抖音视频总结</td></tr>
  <tr><td>P2</td><td>邮件总结</td></tr>
  <tr><td>P2</td><td>linux.do 收藏同步</td></tr>
  <tr><td>P3</td><td>QQ 群消息总结</td></tr>
  <tr><td>P3</td><td>微信群消息总结</td></tr>
</table>
