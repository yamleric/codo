<template>
  <AuthPanel
    v-if="!authLoading && (!authenticated || setupRequired)"
    :setup-required="setupRequired"
    @authenticated="onAuthenticated"
  />

  <main v-else-if="authLoading" class="auth-shell">
    <div class="loading-row"><LoaderCircle :size="16" class="spinning" />检查登录状态</div>
  </main>

  <div v-else class="app-shell">
    <aside class="sidebar">
      <div class="brand">
        <span class="brand-mark"><Workflow :size="18" :stroke-width="2.2" /></span>
        <div>
          <strong>Codo</strong>
          <span>Personal agent</span>
        </div>
      </div>

      <nav class="primary-nav" aria-label="主导航">
        <button
          v-for="item in navigation"
          :key="item.id"
          type="button"
          :class="{ active: activeView === item.id }"
          @click="activeView = item.id"
        >
          <component :is="item.icon" :size="17" />
          <span>{{ item.label }}</span>
          <span v-if="item.count !== undefined" class="nav-count">{{ item.count }}</span>
        </button>
      </nav>

      <div class="sidebar-meta">
        <div class="agent-state">
          <span class="live-dot" :class="connectionState"></span>
          <div>
            <strong>{{ connectionLabel }}</strong>
            <span>Agent API · WebSocket</span>
          </div>
        </div>
        <p>确定性工作流，AI 只处理内容。</p>
      </div>
    </aside>

    <main class="main-content">
      <header class="topbar">
        <div class="mobile-brand">
          <span class="brand-mark"><Workflow :size="17" /></span>
          <strong>Codo</strong>
        </div>
        <div class="topbar-actions">
          <span class="connection-pill">
            <span class="live-dot" :class="connectionState"></span>
            {{ connectionLabel }}
          </span>
          <span class="connection-pill">{{ username }}</span>
          <button
            type="button"
            class="icon-button"
            title="刷新看板"
            :disabled="refreshing"
            @click="loadDashboardData"
          >
            <RefreshCw :size="16" :class="{ spinning: refreshing }" />
          </button>
          <button type="button" class="icon-button" title="退出登录" @click="logout">
            <LogOut :size="16" />
          </button>
        </div>
      </header>

      <div class="content-wrap">
        <header class="page-heading">
          <div>
            <span class="eyebrow">{{ viewMeta.eyebrow }}</span>
            <h1>{{ viewMeta.title }}</h1>
            <p>{{ viewMeta.description }}</p>
          </div>
          <span class="date-stamp">{{ todayLabel }}</span>
        </header>

        <div v-if="loadError" class="notice error-notice">
          <CircleAlert :size="16" />
          <span>{{ loadError }}</span>
          <button type="button" @click="loadDashboardData">重新加载</button>
        </div>

        <template v-if="activeView === 'overview'">
          <SubmitBar @submitted="onSubmitted" />

          <section class="stats-band" aria-label="今日统计">
            <article v-for="stat in stats" :key="stat.label">
              <component :is="stat.icon" :size="17" />
              <div>
                <strong>{{ stat.value }}</strong>
                <span>{{ stat.label }}</span>
              </div>
            </article>
          </section>

          <section class="source-board-panel">
            <header class="section-heading">
              <div>
                <span class="section-kicker">SOURCE RADAR</span>
                <h2>来源看板</h2>
              </div>
              <Database :size="17" />
            </header>
            <div class="source-card-grid">
              <button
                v-for="card in sourceCards"
                :key="card.id"
                type="button"
                class="source-card"
                :class="card.tone"
                @click="activeView = card.view"
              >
                <span class="source-card-icon">
                  <component :is="card.icon" :size="16" />
                </span>
                <span class="source-card-main">
                  <strong>{{ card.label }}</strong>
                  <small>{{ card.secondary }}</small>
                </span>
                <span class="source-card-count">
                  <b>{{ card.primary }}</b>
                  <small>{{ card.unit }}</small>
                </span>
              </button>
            </div>
          </section>

          <div class="dashboard-grid">
            <div class="dashboard-main-stack">
              <section class="source-activity-panel">
                <header class="section-heading">
                  <div>
                    <span class="section-kicker">LIVE INPUTS</span>
                    <h2>最近来源动态</h2>
                  </div>
                  <Clock3 :size="17" />
                </header>
                <div v-if="recentSourceEvents.length" class="source-event-list">
                  <button
                    v-for="event in recentSourceEvents"
                    :key="event.id"
                    type="button"
                    class="source-event-row"
                    @click="activeView = event.view"
                  >
                    <span class="source-event-icon" :class="event.tone">
                      <component :is="event.icon" :size="14" />
                    </span>
                    <span class="source-event-main">
                      <strong>{{ event.title }}</strong>
                      <small>{{ event.meta }}</small>
                    </span>
                    <time>{{ event.time }}</time>
                  </button>
                </div>
                <div v-else class="inline-empty compact-empty">
                  <Activity :size="17" />
                  <p>暂无来源动态</p>
                  <span>添加订阅源或提交链接后，这里会显示最近进入系统的数据。</span>
                </div>
              </section>

              <TaskList
                :tasks="tasks"
                title="最近任务"
                :compact="true"
                @updated="loadDashboardData"
              />
            </div>
            <div class="side-stack">
              <section class="source-insight-panel">
                <header class="section-heading">
                  <div>
                    <span class="section-kicker">KNOWLEDGE FLOW</span>
                    <h2>知识流向</h2>
                  </div>
                  <BookOpen :size="17" />
                </header>
                <div class="source-flow-list">
                  <div v-for="row in knowledgeSourceRows" :key="row.name" class="source-flow-row">
                    <span>{{ row.label }}</span>
                    <strong>{{ row.count }}</strong>
                    <i :style="{ width: row.width }"></i>
                  </div>
                </div>
              </section>
              <section class="process-panel">
                <header class="section-heading">
                  <div>
                    <span class="section-kicker">PIPELINE</span>
                    <h2>处理链路</h2>
                  </div>
                  <Activity :size="17" />
                </header>
                <div class="pipeline-list">
                  <div v-for="(stage, index) in pipelineStages" :key="stage.label" class="pipeline-stage">
                    <span>{{ String(index + 1).padStart(2, '0') }}</span>
                    <div>
                      <strong>{{ stage.label }}</strong>
                      <small>{{ stage.description }}</small>
                    </div>
                  </div>
                </div>
              </section>
              <SubscriptionManager :compact="true" />
            </div>
          </div>
        </template>

        <TaskList
          v-else-if="activeView === 'tasks'"
          :tasks="tasks"
          title="全部任务"
          @updated="loadDashboardData"
        />

        <KnowledgeBase v-else-if="activeView === 'knowledge'" />
        <BookmarkManager v-else-if="activeView === 'bookmarks'" />
        <EmailInbox v-else-if="activeView === 'email'" />
        <ChaoxingBoard v-else-if="activeView === 'chaoxing'" />
        <SettingsPanel v-else-if="activeView === 'settings'" />
        <SubscriptionManager v-else-if="activeView === 'sources'" />
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  Activity,
  AlertTriangle,
  Archive,
  BookOpen,
  Bookmark,
  CheckCircle2,
  CircleAlert,
  Clock3,
  Database,
  Filter,
  Globe2,
  GraduationCap,
  Inbox,
  LayoutDashboard,
  ListTodo,
  LoaderCircle,
  LogOut,
  Mail,
  Newspaper,
  RefreshCw,
  Rss,
  Send,
  SlidersHorizontal,
  Video,
  Workflow,
} from '@lucide/vue'
import SubmitBar from './components/SubmitBar.vue'
import AuthPanel from './components/AuthPanel.vue'
import SettingsPanel from './components/SettingsPanel.vue'
import BookmarkManager from './components/BookmarkManager.vue'
import ChaoxingBoard from './components/ChaoxingBoard.vue'
import EmailInbox from './components/EmailInbox.vue'
import KnowledgeBase from './components/KnowledgeBase.vue'
import SubscriptionManager from './components/SubscriptionManager.vue'
import TaskList from './components/TaskList.vue'
import { api } from './api'
import type { Article, AuthStatus, Bookmark as BookmarkRow, KnowledgeFacets, SourceItem, Subscription, Task } from './types'

type View = 'overview' | 'tasks' | 'knowledge' | 'bookmarks' | 'email' | 'chaoxing' | 'sources' | 'settings'
type ConnectionState = 'connecting' | 'connected' | 'offline'

const tasks = ref<Task[]>([])
const subscriptions = ref<Subscription[]>([])
const sourceItems = ref<SourceItem[]>([])
const articles = ref<Article[]>([])
const bookmarks = ref<BookmarkRow[]>([])
const facets = ref<KnowledgeFacets | null>(null)
const activeView = ref<View>('overview')
const refreshing = ref(false)
const loadError = ref('')
const connectionState = ref<ConnectionState>('connecting')
const authLoading = ref(true)
const authenticated = ref(false)
const setupRequired = ref(false)
const username = ref('')

const stats = computed(() => {
  const today = tasks.value.filter(task => isToday(task.created_at))
  const done = today.filter(task => task.status === 'done').length
  const discarded = today.filter(task => task.status === 'discarded' || task.filter_decision === 'discard').length
  const pushed = today.filter(task => task.filter_decision === 'pass' && task.status === 'done').length
  const todayInputs = today.length + sourceItems.value.filter(item => isToday(item.last_seen_at)).length
  return [
    { label: '今日输入', value: todayInputs, icon: Inbox },
    { label: '完成', value: done, icon: CheckCircle2 },
    { label: '过滤', value: discarded, icon: Filter },
    { label: '推送', value: pushed, icon: Send },
  ]
})

const sourceCards = computed(() => {
  const rssSubs = subscriptions.value.filter(sub => sub.source_type === 'rss')
  const emailItems = sourceItems.value.filter(item => item.source_type === 'email')
  const chaoxingItems = sourceItems.value.filter(item => item.source_type === 'chaoxing')
  const videoTasks = tasks.value.filter(task => task.content_type === 'video')
  const pendingBookmarks = bookmarks.value.filter(bookmark => bookmark.status === 'pending' || bookmark.status === 'failed')
  return [
    {
      id: 'manual',
      label: '手动链接',
      icon: Globe2,
      view: 'tasks' as View,
      primary: countTasksBySource('manual'),
      unit: '任务',
      secondary: `今日 ${countTodayTasksBySource('manual')} 条`,
      tone: 'neutral',
    },
    {
      id: 'rss',
      label: 'RSS 订阅',
      icon: Rss,
      view: 'sources' as View,
      primary: rssSubs.length,
      unit: '订阅',
      secondary: `${rssSubs.filter(sub => sub.enabled).length} 个启用 · ${countArticlesBySource('rss')} 篇归档`,
      tone: 'green',
    },
    {
      id: 'bookmarks',
      label: '收藏夹',
      icon: Bookmark,
      view: 'bookmarks' as View,
      primary: bookmarks.value.length,
      unit: '网址',
      secondary: `${pendingBookmarks.length} 个待同步`,
      tone: 'green',
    },
    {
      id: 'email',
      label: '个人邮件',
      icon: Mail,
      view: 'email' as View,
      primary: emailItems.length,
      unit: '封',
      secondary: `${emailItems.filter(item => item.status === 'important').length} 封重要`,
      tone: 'orange',
    },
    {
      id: 'chaoxing',
      label: '学习通',
      icon: GraduationCap,
      view: 'chaoxing' as View,
      primary: chaoxingItems.length,
      unit: '事项',
      secondary: `${chaoxingItems.filter(isActionableSourceItem).length} 个待处理`,
      tone: 'orange',
    },
    {
      id: 'video',
      label: '视频链接',
      icon: Video,
      view: 'tasks' as View,
      primary: videoTasks.length,
      unit: '任务',
      secondary: `${videoTasks.filter(task => task.status === 'done').length} 个完成`,
      tone: 'neutral',
    },
    {
      id: 'knowledge',
      label: '知识库',
      icon: Archive,
      view: 'knowledge' as View,
      primary: facets.value?.total ?? articles.value.length,
      unit: '条',
      secondary: `${facets.value?.categories.length ?? 0} 个分类`,
      tone: 'green',
    },
  ]
})

const recentSourceEvents = computed(() => {
  const taskEvents = tasks.value.slice(0, 40).map(task => ({
    id: `task:${task.id}`,
    title: taskTitle(task),
    meta: `${sourceLabel(task.source)} · ${statusLabel(task.status)}`,
    at: new Date(task.created_at).getTime(),
    time: timeAgo(task.created_at),
    icon: iconForSource(task.source, task.content_type),
    view: 'tasks' as View,
    tone: task.status === 'failed' ? 'danger' : 'neutral',
  }))
  const sourceEvents = sourceItems.value.slice(0, 80).map(item => ({
    id: `source:${item.id}`,
    title: item.title || sourceLabel(item.source_type),
    meta: `${sourceLabel(item.source_type)} · ${item.status || '已同步'}`,
    at: new Date(item.last_seen_at).getTime(),
    time: timeAgo(item.last_seen_at),
    icon: iconForSource(item.source_type, item.item_type),
    view: sourceView(item.source_type),
    tone: item.status === 'failed' ? 'danger' : item.status === 'important' ? 'orange' : 'green',
  }))
  return [...taskEvents, ...sourceEvents]
    .filter(item => Number.isFinite(item.at))
    .sort((a, b) => b.at - a.at)
    .slice(0, 8)
})

const knowledgeSourceRows = computed(() => {
  const rows = (facets.value?.sources.length ? facets.value.sources : sourceRowsFromArticles())
    .map(row => ({ ...row, label: sourceLabel(row.name) }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 5)
  const max = Math.max(1, ...rows.map(row => row.count))
  if (rows.length === 0) {
    return [{ name: 'empty', label: '暂无归档', count: 0, width: '0%' }]
  }
  return rows.map(row => ({
    ...row,
    width: `${Math.max(8, Math.round((row.count / max) * 100))}%`,
  }))
})

const navigation = computed(() => [
  { id: 'overview' as View, label: '概览', icon: LayoutDashboard },
  { id: 'tasks' as View, label: '任务', icon: ListTodo, count: tasks.value.length },
  { id: 'knowledge' as View, label: '知识库', icon: BookOpen },
  { id: 'bookmarks' as View, label: '收藏', icon: Bookmark },
  { id: 'email' as View, label: '邮件', icon: Mail },
  { id: 'chaoxing' as View, label: '学习通', icon: GraduationCap },
  { id: 'sources' as View, label: '订阅源', icon: Rss },
  { id: 'settings' as View, label: '设置', icon: SlidersHorizontal },
])

const viewMeta = computed(() => {
  if (activeView.value === 'tasks') {
    return { eyebrow: 'TASK OPERATIONS', title: '任务运行记录', description: '查看每一次内容处理的步骤、结果与失败原因。' }
  }
  if (activeView.value === 'sources') {
    return { eyebrow: 'INPUT SOURCES', title: '订阅源管理', description: '管理 Codo 自动巡检并抓取的新内容来源。' }
  }
  if (activeView.value === 'bookmarks') {
    return { eyebrow: 'BOOKMARK SYNC', title: '收藏夹', description: '导入收藏网址，并把待读链接同步到 Codo 的抓取总结流程。' }
  }
  if (activeView.value === 'chaoxing') {
    return { eyebrow: 'CHAOXING WATCH', title: '学习通', description: '查看自动巡检到的作业、考试状态和临近截止事项。' }
  }
  if (activeView.value === 'email') {
    return { eyebrow: 'MAIL DIGEST', title: '邮件助理', description: '汇总个人收件箱，提取重要邮件、待处理事项和每日摘要。' }
  }
  if (activeView.value === 'knowledge') {
    return { eyebrow: 'KNOWLEDGE BASE', title: '知识库', description: '按分类、标签和来源查看已经归档的内容摘要。' }
  }
  if (activeView.value === 'settings') {
    return { eyebrow: 'SYSTEM SETTINGS', title: '配置项设置', description: '调整通知、摘要与过滤偏好，查看运行能力状态。' }
  }
  return { eyebrow: 'AGENT OVERVIEW', title: '工作台', description: '将链接交给 Codo，后台会完成抓取、判断、总结与归档。' }
})

const connectionLabel = computed(() => {
  if (connectionState.value === 'connected') return 'Agent 在线'
  if (connectionState.value === 'connecting') return '正在连接'
  return 'Agent 离线'
})

const todayLabel = new Intl.DateTimeFormat('zh-CN', {
  month: 'long',
  day: 'numeric',
  weekday: 'short',
}).format(new Date())

const pipelineStages = [
  { label: '获取正文', description: 'HTTP / Browser fallback' },
  { label: '价值过滤', description: '去重、质量与兴趣判断' },
  { label: '内容分析', description: '生成结构化摘要' },
  { label: '归档推送', description: '知识库存储与通知' },
]

async function loadDashboardData() {
  if (!authenticated.value) return
  refreshing.value = true
  loadError.value = ''
  const [taskResult, subResult, sourceResult, articleResult, bookmarkResult, facetResult] = await Promise.allSettled([
    api.getTasks(),
    api.getSubscriptions(),
    api.getSourceItems({ limit: 200 }),
    api.getArticles({ limit: 120 }),
    api.getBookmarks(),
    api.getKnowledgeFacets(),
  ])
  if (taskResult.status === 'fulfilled') tasks.value = taskResult.value
  if (subResult.status === 'fulfilled') subscriptions.value = subResult.value
  if (sourceResult.status === 'fulfilled') sourceItems.value = sourceResult.value
  if (articleResult.status === 'fulfilled') articles.value = articleResult.value
  if (bookmarkResult.status === 'fulfilled') bookmarks.value = bookmarkResult.value
  if (facetResult.status === 'fulfilled') facets.value = facetResult.value

  if (taskResult.status === 'rejected') {
    loadError.value = '暂时无法读取任务记录，请确认 API 服务已启动。'
  } else if ([subResult, sourceResult, articleResult, bookmarkResult, facetResult].some(result => result.status === 'rejected')) {
    loadError.value = '部分来源数据暂时无法读取，任务数据已更新。'
  }
  refreshing.value = false
}

function onSubmitted(_id: string) {
  loadDashboardData()
}

async function loadAuthStatus() {
  authLoading.value = true
  try {
    const status = await api.authStatus()
    applyAuthStatus(status)
    if (authenticated.value) {
      await loadDashboardData()
      connectWS()
    }
  } catch {
    authenticated.value = false
    setupRequired.value = false
  } finally {
    authLoading.value = false
  }
}

function onAuthenticated(status: AuthStatus) {
  applyAuthStatus(status)
  loadDashboardData()
  connectWS()
}

function applyAuthStatus(status: AuthStatus) {
  setupRequired.value = status.setup_required
  authenticated.value = status.authenticated
  username.value = status.username || 'owner'
}

async function logout() {
  await api.logout()
  stopped = true
  if (reconnectTimer) window.clearTimeout(reconnectTimer)
  ws?.close()
  ws = null
  tasks.value = []
  subscriptions.value = []
  sourceItems.value = []
  articles.value = []
  bookmarks.value = []
  facets.value = null
  authenticated.value = false
  connectionState.value = 'offline'
  stopped = false
}

let ws: WebSocket | null = null
let reconnectTimer: number | undefined
let stopped = false

function connectWS() {
  if (!authenticated.value) return
  connectionState.value = 'connecting'
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  ws = new WebSocket(`${proto}://${location.host}/ws`)
  ws.onopen = () => {
    connectionState.value = 'connected'
  }
  ws.onmessage = (event) => {
    try {
      const snap: Task = JSON.parse(event.data)
      const index = tasks.value.findIndex(task => task.id === snap.id)
      if (index >= 0) tasks.value[index] = snap
      else tasks.value.unshift(snap)
    } catch {}
  }
  ws.onerror = () => {
    connectionState.value = 'offline'
  }
  ws.onclose = () => {
    connectionState.value = 'offline'
    if (!stopped) reconnectTimer = window.setTimeout(connectWS, 3000)
  }
}

function isToday(value: string) {
  const date = new Date(value)
  const today = new Date()
  return date.toDateString() === today.toDateString()
}

function countTasksBySource(source: string) {
  return tasks.value.filter(task => task.source === source).length
}

function countTodayTasksBySource(source: string) {
  return tasks.value.filter(task => task.source === source && isToday(task.created_at)).length
}

function countArticlesBySource(source: string) {
  return articles.value.filter(article => article.source === source).length
}

function isActionableSourceItem(item: SourceItem) {
  const status = (item.status || '').trim()
  if (item.source_type === 'email') return item.status === 'important' || item.status === 'failed'
  if (status.includes('已完成') || status.includes('已提交') || status.includes('已交')) return false
  return status === '' || status.includes('未') || status.includes('待') || status.includes('进行') || status === 'pending'
}

function sourceRowsFromArticles() {
  const counts = new Map<string, number>()
  for (const article of articles.value) {
    const source = article.source || 'manual'
    counts.set(source, (counts.get(source) || 0) + 1)
  }
  return Array.from(counts.entries()).map(([name, count]) => ({ name, count }))
}

function sourceLabel(source: string) {
  switch (source) {
    case 'manual':
      return '手动链接'
    case 'rss':
      return 'RSS'
    case 'bookmark':
      return '收藏'
    case 'email':
      return '邮件'
    case 'chaoxing':
      return '学习通'
    case 'wechat_mp':
      return '公众号'
    case 'linux_do':
      return 'linux.do'
    case 'video':
      return '视频'
    default:
      return source || '未知来源'
  }
}

function sourceView(source: string): View {
  if (source === 'email') return 'email'
  if (source === 'chaoxing') return 'chaoxing'
  if (source === 'rss') return 'sources'
  return 'tasks'
}

function iconForSource(source: string, contentType = '') {
  if (contentType === 'video') return Video
  if (source === 'rss') return Rss
  if (source === 'bookmark') return Bookmark
  if (source === 'email') return Mail
  if (source === 'chaoxing') return GraduationCap
  if (source === 'wechat_mp') return Newspaper
  if (source === 'failed') return AlertTriangle
  return Globe2
}

function taskTitle(task: Task) {
  const summary = firstLine(task.summary)
  if (summary) return summary
  if (task.url) return hostLabel(task.url)
  return `${sourceLabel(task.source)}任务`
}

function firstLine(value: string) {
  return (value || '').split(/\r?\n/).map(line => line.trim()).find(Boolean) || ''
}

function hostLabel(value: string) {
  try {
    return new URL(value).hostname.replace(/^www\./, '')
  } catch {
    return value || '未命名链接'
  }
}

function statusLabel(status: string) {
  switch (status) {
    case 'done':
      return '完成'
    case 'failed':
      return '失败'
    case 'discarded':
      return '已过滤'
    case 'pending':
      return '排队'
    default:
      return status || '处理中'
  }
}

function timeAgo(value: string) {
  const date = new Date(value)
  const diff = Date.now() - date.getTime()
  if (Number.isNaN(diff)) return '刚刚'
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)} 分钟前`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)} 小时前`
  return `${Math.floor(diff / 86400000)} 天前`
}

onMounted(() => {
  loadAuthStatus()
})

onUnmounted(() => {
  stopped = true
  if (reconnectTimer) window.clearTimeout(reconnectTimer)
  ws?.close()
})
</script>
