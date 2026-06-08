<template>
  <section class="subscription-panel" :class="{ compact }">
    <header class="section-heading">
      <div>
        <span class="section-kicker">SOURCES</span>
        <h2>订阅源管理</h2>
      </div>
      <div class="source-heading-actions">
        <button
          type="button"
          class="icon-button"
          title="刷新订阅源列表"
          :disabled="loading"
          @click="load"
        >
          <RefreshCw :size="16" :class="{ spinning: loading }" />
        </button>
        <button
          type="button"
          class="icon-button"
          :title="showAdd ? '关闭添加表单' : '添加订阅源'"
          @click="toggleAdd"
        >
          <X v-if="showAdd" :size="16" />
          <Plus v-else :size="16" />
        </button>
      </div>
    </header>

    <div v-if="!compact" class="source-summary-strip" aria-label="订阅源概览">
      <article>
        <strong>{{ subs.length }}</strong>
        <span>全部订阅</span>
      </article>
      <article>
        <strong>{{ enabledCount }}</strong>
        <span>启用</span>
      </article>
      <article>
        <strong>{{ errorCount }}</strong>
        <span>异常</span>
      </article>
      <article>
        <strong>{{ categoryCount }}</strong>
        <span>分组</span>
      </article>
    </div>

    <form v-if="showAdd" class="source-form source-form-wide" @submit.prevent="add">
      <div class="source-type-tabs" aria-label="订阅源类型">
        <button type="button" :class="{ active: draft.source_type === 'rss' }" @click="setDraftType('rss')">
          <Rss :size="14" />RSS
        </button>
        <button type="button" :class="{ active: draft.source_type === 'chaoxing' }" @click="setDraftType('chaoxing')">
          <GraduationCap :size="14" />学习通
        </button>
        <button type="button" :class="{ active: draft.source_type === 'email' }" @click="setDraftType('email')">
          <Mail :size="14" />邮箱
        </button>
      </div>

      <template v-if="draft.source_type === 'rss'">
        <label for="feed-url">Feed 地址</label>
        <div class="source-input-row">
          <input
            id="feed-url"
            v-model="draft.feed_url"
            type="url"
            placeholder="https://example.com/feed.xml"
            @input="error = ''"
          />
          <button type="submit" title="添加订阅源" :disabled="saving || !canSubmitDraft">
            <LoaderCircle v-if="saving" :size="16" class="spinning" />
            <ArrowRight v-else :size="16" />
          </button>
        </div>
      </template>

      <template v-else-if="draft.source_type === 'chaoxing'">
        <label for="chaoxing-account">学习通登录授权</label>
        <div class="source-input-row">
          <input
            id="chaoxing-account"
            v-model="draft.account"
            type="text"
            autocomplete="username"
            placeholder="手机号 / 学习通账号"
            @input="error = ''"
          />
          <button type="submit" title="添加学习通巡检" :disabled="saving || !canSubmitDraft">
            <LoaderCircle v-if="saving" :size="16" class="spinning" />
            <ArrowRight v-else :size="16" />
          </button>
        </div>
        <div class="source-form-grid">
          <input v-model="draft.password" type="password" autocomplete="new-password" placeholder="密码，保存后不回显" @input="error = ''" />
          <input v-model.number="draft.alert_hours" type="number" min="1" max="168" placeholder="提前提醒小时数" @input="error = ''" />
        </div>
        <textarea
          v-model="draft.cookie"
          class="source-cookie-input"
          rows="2"
          placeholder="Cookie，可选；账号密码不可用时作为兜底"
          @input="error = ''"
        />
        <div class="source-form-options">
          <label><input v-model="draft.notify_new" type="checkbox" />新作业/考试提醒</label>
          <label><input v-model="draft.notify_due" type="checkbox" />临近截止提醒</label>
        </div>
      </template>

      <template v-else>
        <label for="email-account">邮箱收件箱授权</label>
        <div class="source-input-row">
          <input
            id="email-account"
            v-model="draft.account"
            type="email"
            autocomplete="username"
            placeholder="name@example.com"
            @input="error = ''"
          />
          <button type="submit" title="添加邮箱同步" :disabled="saving || !canSubmitDraft">
            <LoaderCircle v-if="saving" :size="16" class="spinning" />
            <ArrowRight v-else :size="16" />
          </button>
        </div>
        <div class="source-form-grid">
          <input v-model="draft.password" type="password" autocomplete="new-password" placeholder="IMAP 授权码 / 应用专用密码" @input="error = ''" />
          <input v-model="draft.host" type="text" placeholder="IMAP 主机，可自动识别" @input="error = ''" />
        </div>
        <div class="source-form-grid">
          <input v-model.number="draft.port" type="number" min="1" max="65535" placeholder="端口 993" @input="error = ''" />
          <input v-model="draft.mailbox" type="text" placeholder="文件夹 INBOX" @input="error = ''" />
        </div>
        <div class="source-form-grid">
          <input v-model.number="draft.since_days" type="number" min="1" max="30" placeholder="同步最近天数" @input="error = ''" />
          <input v-model.number="draft.max_messages" type="number" min="1" max="100" placeholder="每次最多封数" @input="error = ''" />
        </div>
        <div class="source-form-options">
          <label><input v-model="draft.notify_important" type="checkbox" />重要邮件单独提取</label>
          <label><input v-model="draft.sync_unread_only" type="checkbox" />只同步未读</label>
        </div>
      </template>

      <div v-if="!compact" class="source-form-meta">
        <input v-model="draft.title" type="text" placeholder="显示名称，可选" @input="error = ''" />
        <input v-model="draft.category" type="text" placeholder="分组，例如 课程 / 学习" @input="error = ''" />
      </div>
      <p v-if="error" class="field-error"><CircleAlert :size="14" />{{ error }}</p>
    </form>

    <div v-if="loadError" class="source-alert">
      <CircleAlert :size="15" />
      <span>{{ loadError }}</span>
      <button type="button" @click="load">重试</button>
    </div>

    <div v-if="subs.length && !compact" class="source-toolbar">
      <label class="search-field">
        <Search :size="15" />
        <input v-model="query" type="search" placeholder="搜索订阅名、分组或 URL" />
      </label>
      <div class="filter-tabs" aria-label="订阅源状态筛选">
        <button
          v-for="item in filters"
          :key="item.id"
          type="button"
          :class="{ active: activeFilter === item.id }"
          @click="activeFilter = item.id"
        >
          {{ item.label }}
        </button>
      </div>
    </div>

    <div v-if="visibleSubs.length" class="source-list">
      <article v-for="sub in visibleSubs" :key="sub.id" class="source-row rich" :class="{ disabled: !sub.enabled }">
        <span class="source-icon" :class="{ danger: !!sub.last_error, muted: !sub.enabled }">
          <CircleAlert v-if="sub.last_error" :size="15" />
          <PauseCircle v-else-if="!sub.enabled" :size="15" />
          <GraduationCap v-else-if="sub.source_type === 'chaoxing'" :size="15" />
          <Mail v-else-if="sub.source_type === 'email'" :size="15" />
          <Rss v-else :size="15" />
        </span>
        <div class="source-body">
          <div class="source-title-line">
            <strong>{{ displayName(sub) }}</strong>
            <span class="source-badge" :class="{ paused: !sub.enabled, danger: !!sub.last_error }">
              {{ statusLabel(sub) }}
            </span>
            <span class="source-badge">{{ typeLabel(sub) }}</span>
            <span v-if="sub.category" class="source-badge">{{ sub.category }}</span>
          </div>
          <span class="source-url">{{ sourceDescription(sub) }}</span>
          <small v-if="sub.last_error" class="source-error">{{ sub.last_error }}</small>
        </div>
        <span class="source-time">
          <Clock3 :size="12" />
          {{ sub.last_fetched_at ? timeAgo(sub.last_fetched_at) : '等待首次拉取' }}
        </span>
        <div class="source-actions">
          <button type="button" :title="sub.source_type === 'chaoxing' ? '测试并刷新' : '立即刷新'" :disabled="busyID === sub.id" @click="refresh(sub)">
            <RefreshCw :size="14" :class="{ spinning: busyID === sub.id }" />
          </button>
          <button type="button" :title="sub.enabled ? '暂停订阅' : '启用订阅'" @click="toggleEnabled(sub)">
            <Pause v-if="sub.enabled" :size="14" />
            <Play v-else :size="14" />
          </button>
          <button type="button" title="编辑订阅" @click="startEdit(sub)">
            <Pencil :size="14" />
          </button>
          <button type="button" title="删除订阅" class="danger" @click="remove(sub)">
            <Trash2 :size="14" />
          </button>
        </div>
      </article>
    </div>

    <div v-else-if="loaded" class="inline-empty">
      <Rss :size="18" />
      <p>{{ subs.length ? '没有匹配的订阅源' : '还没有自动订阅源' }}</p>
      <span>{{ subs.length ? '调整搜索词或状态筛选。' : '添加 RSS 或 Atom Feed 后，Codo 会定时检查新内容。' }}</span>
      <button v-if="!subs.length" type="button" @click="showAdd = true">
        <Plus :size="14" />添加订阅源
      </button>
    </div>
    <div v-else class="loading-row"><LoaderCircle :size="16" class="spinning" />读取订阅源</div>

    <div v-if="editing" class="source-edit-backdrop" @click.self="editing = null">
      <form class="source-edit-dialog" @submit.prevent="saveEdit">
        <header>
          <div>
            <span class="section-kicker">EDIT SOURCE</span>
            <h3>编辑订阅源</h3>
          </div>
          <button type="button" class="icon-button" title="关闭" @click="editing = null">
            <X :size="16" />
          </button>
        </header>
        <label v-if="editing.source_type === 'rss'">
          <span>Feed 地址</span>
          <input v-model="editDraft.feed_url" type="url" required />
        </label>
        <template v-else-if="editing.source_type === 'chaoxing'">
          <label>
            <span>学习通账号</span>
            <input v-model="editDraft.account" type="text" autocomplete="username" required />
          </label>
          <div class="source-edit-grid">
            <label>
              <span>更新密码</span>
              <input v-model="editDraft.password" type="password" autocomplete="new-password" :placeholder="editing.password_configured ? '已配置，留空不修改' : '未配置'" />
            </label>
            <label>
              <span>提前提醒</span>
              <input v-model.number="editDraft.alert_hours" type="number" min="1" max="168" />
            </label>
          </div>
          <label>
            <span>Cookie 兜底</span>
            <textarea v-model="editDraft.cookie" rows="2" :placeholder="editing.cookie_configured ? '已配置，留空不修改' : '可选'" />
          </label>
          <div class="source-credential-state">
            <span>{{ editing.password_configured ? '密码已配置' : '密码未配置' }}</span>
            <span>{{ editing.cookie_configured ? 'Cookie 已配置' : 'Cookie 未配置' }}</span>
          </div>
          <div class="source-form-options">
            <label><input v-model="editDraft.notify_new" type="checkbox" />新作业/考试提醒</label>
            <label><input v-model="editDraft.notify_due" type="checkbox" />临近截止提醒</label>
          </div>
        </template>
        <template v-else-if="editing.source_type === 'email'">
          <label>
            <span>邮箱账号</span>
            <input v-model="editDraft.account" type="email" autocomplete="username" required />
          </label>
          <div class="source-edit-grid">
            <label>
              <span>更新授权码</span>
              <input v-model="editDraft.password" type="password" autocomplete="new-password" :placeholder="editing.password_configured ? '已配置，留空不修改' : 'IMAP 授权码 / 应用专用密码'" />
            </label>
            <label>
              <span>IMAP 主机</span>
              <input v-model="editDraft.host" type="text" placeholder="imap.example.com" />
            </label>
          </div>
          <div class="source-edit-grid">
            <label>
              <span>端口</span>
              <input v-model.number="editDraft.port" type="number" min="1" max="65535" />
            </label>
            <label>
              <span>文件夹</span>
              <input v-model="editDraft.mailbox" type="text" placeholder="INBOX" />
            </label>
          </div>
          <div class="source-edit-grid">
            <label>
              <span>最近天数</span>
              <input v-model.number="editDraft.since_days" type="number" min="1" max="30" />
            </label>
            <label>
              <span>最多封数</span>
              <input v-model.number="editDraft.max_messages" type="number" min="1" max="100" />
            </label>
          </div>
          <div class="source-credential-state">
            <span>{{ editing.password_configured ? '授权码已配置' : '授权码未配置' }}</span>
            <span>{{ editDraft.sync_unread_only ? '仅同步未读' : '同步最近邮件' }}</span>
          </div>
          <div class="source-form-options">
            <label><input v-model="editDraft.notify_important" type="checkbox" />重要邮件单独提取</label>
            <label><input v-model="editDraft.sync_unread_only" type="checkbox" />只同步未读</label>
          </div>
        </template>
        <label>
          <span>显示名称</span>
          <input v-model="editDraft.title" type="text" placeholder="默认显示域名" />
        </label>
        <label>
          <span>分组</span>
          <input v-model="editDraft.category" type="text" placeholder="例如 技术 / 新闻" />
        </label>
        <label class="source-check">
          <input v-model="editDraft.enabled" type="checkbox" />
          <span>启用自动巡检</span>
        </label>
        <p v-if="editError" class="field-error"><CircleAlert :size="14" />{{ editError }}</p>
        <footer>
          <button type="button" @click="editing = null">取消</button>
          <button type="submit" :disabled="saving">
            <LoaderCircle v-if="saving" :size="15" class="spinning" />
            保存
          </button>
        </footer>
      </form>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import {
  ArrowRight,
  CircleAlert,
  Clock3,
  GraduationCap,
  LoaderCircle,
  Mail,
  Pause,
  PauseCircle,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  Rss,
  Search,
  Trash2,
  X,
} from '@lucide/vue'
import { api } from '../api'
import type { Subscription } from '../types'

const props = defineProps<{ compact?: boolean }>()

type FilterID = 'all' | 'enabled' | 'paused' | 'error'
type SourceType = 'rss' | 'chaoxing' | 'email'

const subs = ref<Subscription[]>([])
const showAdd = ref(false)
const saving = ref(false)
const loading = ref(false)
const loaded = ref(false)
const error = ref('')
const loadError = ref('')
const editError = ref('')
const query = ref('')
const activeFilter = ref<FilterID>('all')
const busyID = ref('')
const editing = ref<Subscription | null>(null)
const draft = reactive({
  source_type: 'rss' as SourceType,
  feed_url: '',
  title: '',
  category: '',
  account: '',
  password: '',
  cookie: '',
  alert_hours: 24,
  notify_new: true,
  notify_due: true,
  provider: 'imap',
  host: '',
  port: 993,
  mailbox: 'INBOX',
  since_days: 1,
  max_messages: 20,
  notify_important: true,
  sync_unread_only: false,
})
const editDraft = reactive({
  feed_url: '',
  title: '',
  category: '',
  enabled: true,
  account: '',
  password: '',
  cookie: '',
  alert_hours: 24,
  notify_new: true,
  notify_due: true,
  provider: 'imap',
  host: '',
  port: 993,
  mailbox: 'INBOX',
  since_days: 1,
  max_messages: 20,
  notify_important: true,
  sync_unread_only: false,
})

const filters: { id: FilterID; label: string }[] = [
  { id: 'all', label: '全部' },
  { id: 'enabled', label: '启用' },
  { id: 'paused', label: '暂停' },
  { id: 'error', label: '异常' },
]

const enabledCount = computed(() => subs.value.filter(sub => sub.enabled).length)
const errorCount = computed(() => subs.value.filter(sub => !!sub.last_error).length)
const categoryCount = computed(() => new Set(subs.value.map(sub => sub.category).filter(Boolean)).size)
const canSubmitDraft = computed(() => {
  if (draft.source_type === 'rss') return !!draft.feed_url.trim()
  if (draft.source_type === 'chaoxing') return !!draft.account.trim() && (!!draft.password || !!draft.cookie.trim())
  return !!draft.account.trim() && !!draft.password
})

const filteredSubs = computed(() => {
  const term = query.value.trim().toLowerCase()
  return subs.value.filter((sub) => {
    const haystack = `${displayName(sub)} ${sub.feed_url} ${sub.category} ${sub.account} ${sub.host} ${sub.mailbox}`.toLowerCase()
    if (term && !haystack.includes(term)) return false
    if (activeFilter.value === 'enabled') return sub.enabled
    if (activeFilter.value === 'paused') return !sub.enabled
    if (activeFilter.value === 'error') return !!sub.last_error
    return true
  })
})

const visibleSubs = computed(() => {
  const list = filteredSubs.value
  return props.compact ? list.slice(0, 4) : list
})

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    subs.value = await api.getSubscriptions()
  } catch {
    loadError.value = '无法读取订阅源，请确认 API 服务可用。'
  } finally {
    loaded.value = true
    loading.value = false
  }
}

function toggleAdd() {
  showAdd.value = !showAdd.value
  error.value = ''
}

function setDraftType(type: SourceType) {
  draft.source_type = type
  error.value = ''
}

async function add() {
  if (!canSubmitDraft.value || saving.value) return
  saving.value = true
  error.value = ''
  try {
    if (draft.source_type === 'rss') {
      await api.addSubscription({
        source_type: 'rss',
        feed_url: draft.feed_url.trim(),
        title: draft.title.trim(),
        category: draft.category.trim(),
      })
    } else if (draft.source_type === 'chaoxing') {
      await api.addSubscription({
        source_type: 'chaoxing',
        account: draft.account.trim(),
        password: draft.password,
        cookie: draft.cookie.trim(),
        title: draft.title.trim(),
        category: draft.category.trim(),
        alert_hours: Number(draft.alert_hours) || 24,
        notify_new: draft.notify_new,
        notify_due: draft.notify_due,
      })
    } else {
      await api.addSubscription({
        source_type: 'email',
        account: draft.account.trim(),
        password: draft.password,
        provider: draft.provider.trim(),
        host: draft.host.trim(),
        port: Number(draft.port) || 993,
        mailbox: draft.mailbox.trim() || 'INBOX',
        since_days: Number(draft.since_days) || 1,
        max_messages: Number(draft.max_messages) || 20,
        notify_important: draft.notify_important,
        sync_unread_only: draft.sync_unread_only,
        title: draft.title.trim(),
        category: draft.category.trim(),
      })
    }
    draft.feed_url = ''
    draft.title = ''
    draft.category = ''
    draft.account = ''
    draft.password = ''
    draft.cookie = ''
    draft.alert_hours = 24
    draft.notify_new = true
    draft.notify_due = true
    draft.provider = 'imap'
    draft.host = ''
    draft.port = 993
    draft.mailbox = 'INBOX'
    draft.since_days = 1
    draft.max_messages = 20
    draft.notify_important = true
    draft.sync_unread_only = false
    showAdd.value = false
    await load()
  } catch {
    if (draft.source_type === 'chaoxing') {
      error.value = '添加失败，请检查学习通账号、密码或 Cookie。'
    } else if (draft.source_type === 'email') {
      error.value = '添加失败，请检查邮箱地址、IMAP 主机或授权码。'
    } else {
      error.value = '添加失败，请检查 Feed 地址。'
    }
  } finally {
    saving.value = false
  }
}

function startEdit(sub: Subscription) {
  editing.value = sub
  editError.value = ''
  editDraft.feed_url = sub.feed_url
  editDraft.title = sub.title || ''
  editDraft.category = sub.category || ''
  editDraft.enabled = sub.enabled
  editDraft.account = sub.account || ''
  editDraft.password = ''
  editDraft.cookie = ''
  editDraft.alert_hours = sub.alert_hours || 24
  editDraft.notify_new = sub.notify_new
  editDraft.notify_due = sub.notify_due
  editDraft.provider = sub.provider || 'imap'
  editDraft.host = sub.host || ''
  editDraft.port = sub.port || 993
  editDraft.mailbox = sub.mailbox || 'INBOX'
  editDraft.since_days = sub.since_days || 1
  editDraft.max_messages = sub.max_messages || 20
  editDraft.notify_important = sub.notify_important
  editDraft.sync_unread_only = sub.sync_unread_only
}

async function saveEdit() {
  if (!editing.value || saving.value) return
  saving.value = true
  editError.value = ''
  const editingSourceType = editing.value.source_type
  try {
    if (editingSourceType === 'chaoxing') {
      const payload: {
        account: string
        title: string
        category: string
        enabled: boolean
        alert_hours: number
        notify_new: boolean
        notify_due: boolean
        password?: string
        cookie?: string
      } = {
        account: editDraft.account.trim(),
        title: editDraft.title.trim(),
        category: editDraft.category.trim(),
        enabled: editDraft.enabled,
        alert_hours: Number(editDraft.alert_hours) || 24,
        notify_new: editDraft.notify_new,
        notify_due: editDraft.notify_due,
      }
      if (editDraft.password) payload.password = editDraft.password
      if (editDraft.cookie.trim()) payload.cookie = editDraft.cookie.trim()
      await api.updateSubscription(editing.value.id, payload)
    } else if (editingSourceType === 'email') {
      const payload: {
        account: string
        title: string
        category: string
        enabled: boolean
        provider: string
        host: string
        port: number
        mailbox: string
        since_days: number
        max_messages: number
        notify_important: boolean
        sync_unread_only: boolean
        password?: string
      } = {
        account: editDraft.account.trim(),
        title: editDraft.title.trim(),
        category: editDraft.category.trim(),
        enabled: editDraft.enabled,
        provider: editDraft.provider.trim(),
        host: editDraft.host.trim(),
        port: Number(editDraft.port) || 993,
        mailbox: editDraft.mailbox.trim() || 'INBOX',
        since_days: Number(editDraft.since_days) || 1,
        max_messages: Number(editDraft.max_messages) || 20,
        notify_important: editDraft.notify_important,
        sync_unread_only: editDraft.sync_unread_only,
      }
      if (editDraft.password) payload.password = editDraft.password
      await api.updateSubscription(editing.value.id, payload)
    } else {
      await api.updateSubscription(editing.value.id, {
        feed_url: editDraft.feed_url.trim(),
        title: editDraft.title.trim(),
        category: editDraft.category.trim(),
        enabled: editDraft.enabled,
      })
    }
    editing.value = null
    await load()
  } catch {
    if (editingSourceType === 'chaoxing') {
      editError.value = '保存失败，请检查学习通配置。'
    } else if (editingSourceType === 'email') {
      editError.value = '保存失败，请检查邮箱 IMAP 配置。'
    } else {
      editError.value = '保存失败，请检查 Feed 地址或稍后重试。'
    }
  } finally {
    saving.value = false
  }
}

async function toggleEnabled(sub: Subscription) {
  await api.updateSubscription(sub.id, { enabled: !sub.enabled })
  await load()
}

async function refresh(sub: Subscription) {
  busyID.value = sub.id
  try {
    await api.refreshSubscription(sub.id)
    await load()
  } catch {
    await load()
    loadError.value = `${displayName(sub)} 刷新失败，已记录到订阅源状态。`
  } finally {
    busyID.value = ''
  }
}

async function remove(sub: Subscription) {
  if (!window.confirm(`删除订阅源「${displayName(sub)}」？`)) return
  await api.deleteSubscription(sub.id)
  await load()
}

function displayName(sub: Subscription) {
  if (sub.title?.trim()) return sub.title.trim()
  if (sub.source_type === 'chaoxing') return sub.account ? `学习通 ${sub.account}` : '学习通巡检'
  if (sub.source_type === 'email') return sub.account ? `邮箱 ${sub.account}` : '邮箱收件箱'
  return feedName(sub.feed_url)
}

function sourceDescription(sub: Subscription) {
  if (sub.source_type === 'email') {
    const states = [
      sub.password_configured ? '授权码已配置' : '授权码未配置',
      `${sub.host || '自动识别'}:${sub.port || 993}`,
      sub.mailbox || 'INBOX',
      `${sub.max_messages || 20} 封/次`,
      sub.sync_unread_only ? '仅未读' : '全部最近邮件',
    ]
    return states.join(' · ')
  }
  if (sub.source_type !== 'chaoxing') return sub.feed_url
  const states = [
    sub.password_configured ? '密码已配置' : '密码未配置',
    sub.cookie_configured ? 'Cookie 已配置' : 'Cookie 未配置',
    `${sub.alert_hours || 24} 小时提醒`,
  ]
  return `账号：${sub.account || '未填写'} · ${states.join(' · ')}`
}

function feedName(url: string) {
  try {
    return new URL(url).hostname.replace(/^www\./, '')
  } catch {
    return 'RSS Feed'
  }
}

function statusLabel(sub: Subscription) {
  if (sub.last_error) return '异常'
  if (!sub.enabled) return '暂停'
  return '巡检中'
}

function typeLabel(sub: Subscription) {
  if (sub.source_type === 'email') return '邮箱'
  return sub.source_type === 'chaoxing' ? '学习通' : 'RSS'
}

function timeAgo(iso: string) {
  const diff = Date.now() - new Date(iso).getTime()
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)} 分钟前`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)} 小时前`
  return `${Math.floor(diff / 86400000)} 天前`
}

onMounted(load)
</script>
