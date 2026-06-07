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
            title="刷新任务"
            :disabled="refreshing"
            @click="loadTasks"
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
          <button type="button" @click="loadTasks">重新加载</button>
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

          <div class="dashboard-grid">
            <TaskList
              :tasks="tasks"
              title="最近任务"
              :compact="true"
              @updated="loadTasks"
            />
            <div class="side-stack">
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
          @updated="loadTasks"
        />

        <KnowledgeBase v-else-if="activeView === 'knowledge'" />
        <BookmarkManager v-else-if="activeView === 'bookmarks'" />
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
  BookOpen,
  Bookmark,
  CheckCircle2,
  CircleAlert,
  Filter,
  GraduationCap,
  LayoutDashboard,
  ListTodo,
  LoaderCircle,
  LogOut,
  RefreshCw,
  Rss,
  Send,
  SlidersHorizontal,
  Workflow,
} from '@lucide/vue'
import SubmitBar from './components/SubmitBar.vue'
import AuthPanel from './components/AuthPanel.vue'
import SettingsPanel from './components/SettingsPanel.vue'
import BookmarkManager from './components/BookmarkManager.vue'
import ChaoxingBoard from './components/ChaoxingBoard.vue'
import KnowledgeBase from './components/KnowledgeBase.vue'
import SubscriptionManager from './components/SubscriptionManager.vue'
import TaskList from './components/TaskList.vue'
import { api } from './api'
import type { AuthStatus, Task } from './types'

type View = 'overview' | 'tasks' | 'knowledge' | 'bookmarks' | 'chaoxing' | 'sources' | 'settings'
type ConnectionState = 'connecting' | 'connected' | 'offline'

const tasks = ref<Task[]>([])
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
  return [
    { label: '今日处理', value: today.length, icon: Activity },
    { label: '完成', value: done, icon: CheckCircle2 },
    { label: '过滤', value: discarded, icon: Filter },
    { label: '推送', value: pushed, icon: Send },
  ]
})

const navigation = computed(() => [
  { id: 'overview' as View, label: '概览', icon: LayoutDashboard },
  { id: 'tasks' as View, label: '任务', icon: ListTodo, count: tasks.value.length },
  { id: 'knowledge' as View, label: '知识库', icon: BookOpen },
  { id: 'bookmarks' as View, label: '收藏', icon: Bookmark },
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

async function loadTasks() {
  if (!authenticated.value) return
  refreshing.value = true
  loadError.value = ''
  try {
    tasks.value = await api.getTasks()
  } catch {
    loadError.value = '暂时无法读取任务记录，请确认 API 服务已启动。'
  } finally {
    refreshing.value = false
  }
}

function onSubmitted(_id: string) {
  loadTasks()
}

async function loadAuthStatus() {
  authLoading.value = true
  try {
    const status = await api.authStatus()
    applyAuthStatus(status)
    if (authenticated.value) {
      await loadTasks()
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
  loadTasks()
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

onMounted(() => {
  loadAuthStatus()
})

onUnmounted(() => {
  stopped = true
  if (reconnectTimer) window.clearTimeout(reconnectTimer)
  ws?.close()
})
</script>
