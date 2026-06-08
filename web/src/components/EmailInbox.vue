<template>
  <section class="email-panel">
    <header class="section-heading">
      <div>
        <span class="section-kicker">MAIL DIGEST</span>
        <h2>个人邮件收件箱</h2>
      </div>
      <div class="source-heading-actions">
        <button type="button" class="icon-button" title="同步邮箱" :disabled="syncing || !emailSubs.length" @click="syncAll">
          <RefreshCw :size="16" :class="{ spinning: syncing }" />
        </button>
        <button type="button" class="icon-button" title="刷新列表" :disabled="loading" @click="load">
          <RotateCw :size="16" :class="{ spinning: loading }" />
        </button>
      </div>
    </header>

    <div v-if="error" class="source-alert">
      <CircleAlert :size="15" />
      <span>{{ error }}</span>
      <button type="button" @click="load">重试</button>
    </div>

    <div class="email-summary-strip" aria-label="邮件概览">
      <article>
        <strong>{{ items.length }}</strong>
        <span>已同步</span>
      </article>
      <article>
        <strong>{{ importantCount }}</strong>
        <span>重要邮件</span>
      </article>
      <article>
        <strong>{{ todayCount }}</strong>
        <span>今日邮件</span>
      </article>
      <article>
        <strong>{{ emailSubs.length }}</strong>
        <span>邮箱账号</span>
      </article>
    </div>

    <div class="email-toolbar">
      <label class="search-field email-search">
        <Search :size="15" />
        <input v-model="query" type="search" placeholder="搜索发件人、主题、摘要或标签" />
      </label>
      <div class="filter-tabs" aria-label="邮件筛选">
        <button
          v-for="item in filters"
          :key="item.id"
          type="button"
          :class="{ active: activeFilter === item.id }"
          @click="activeFilter = item.id"
        >
          <component :is="item.icon" :size="13" />
          {{ item.label }}
        </button>
      </div>
    </div>

    <div v-if="visibleItems.length" class="email-list">
      <article
        v-for="item in visibleItems"
        :key="item.id"
        class="email-row"
        :class="{ important: item.status === 'important', failed: item.status === 'failed' }"
        @click="openItem(item)"
      >
        <span class="email-kind" :class="item.status">
          <Star v-if="item.status === 'important'" :size="15" />
          <ShieldAlert v-else-if="item.status === 'failed'" :size="15" />
          <MailOpen v-else :size="15" />
        </span>
        <div class="email-main">
          <div class="email-title-line">
            <strong>{{ subject(item) }}</strong>
            <span class="email-badge" :class="item.status">{{ statusLabel(item) }}</span>
            <span v-if="category(item)" class="email-badge">{{ category(item) }}</span>
          </div>
          <span class="email-from">{{ sender(item) }}</span>
          <p>{{ summary(item) }}</p>
          <div class="email-meta">
            <span><Clock3 :size="12" />{{ receivedLabel(item) }}</span>
            <span v-if="account(item)"><Inbox :size="12" />{{ account(item) }}</span>
          </div>
        </div>
      </article>
    </div>

    <div v-else-if="loaded" class="inline-empty">
      <Mail :size="18" />
      <p>{{ items.length ? '没有匹配的邮件' : '还没有同步邮件' }}</p>
      <span>{{ emailSubs.length ? '点击右上角同步按钮，或等待后台定时巡检。' : '先到订阅源管理中添加邮箱账号。' }}</span>
    </div>
    <div v-else class="loading-row"><LoaderCircle :size="16" class="spinning" />读取邮件摘要</div>

    <div v-if="readerOpen" class="knowledge-reader-backdrop" @click.self="closeReader">
      <article class="knowledge-reader">
        <header>
          <div>
            <span class="section-kicker">EMAIL DETAIL</span>
            <h3>{{ readerTitle }}</h3>
          </div>
          <button type="button" class="icon-button" title="关闭" @click="closeReader">
            <X :size="16" />
          </button>
        </header>
        <div class="knowledge-reader-tabs">
          <button type="button" :class="{ active: readerMode === 'summary' }" @click="readerMode = 'summary'">摘要</button>
          <button type="button" :class="{ active: readerMode === 'content' }" @click="readerMode = 'content'">正文</button>
        </div>
        <div v-if="readerMode === 'summary'" class="knowledge-reader-summary">
          <p v-for="(paragraph, index) in readerSummary" :key="index">{{ paragraph }}</p>
        </div>
        <div v-else class="knowledge-reader-body">
          <p v-for="(paragraph, index) in readerContent" :key="index">{{ paragraph }}</p>
        </div>
      </article>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  CircleAlert,
  Clock3,
  Inbox,
  LoaderCircle,
  Mail,
  MailOpen,
  RefreshCw,
  RotateCw,
  Search,
  ShieldAlert,
  Star,
  X,
} from '@lucide/vue'
import { api } from '../api'
import type { Article, SourceItem, Subscription } from '../types'

type FilterID = 'all' | 'important' | 'today' | 'normal' | 'failed'

const items = ref<SourceItem[]>([])
const subs = ref<Subscription[]>([])
const loading = ref(false)
const syncing = ref(false)
const loaded = ref(false)
const error = ref('')
const query = ref('')
const activeFilter = ref<FilterID>('important')
const readerOpen = ref(false)
const readerMode = ref<'summary' | 'content'>('summary')
const readerArticle = ref<Article | null>(null)
const readerFallback = ref<SourceItem | null>(null)

const filters = [
  { id: 'all' as FilterID, label: '全部', icon: Inbox },
  { id: 'important' as FilterID, label: '重要', icon: Star },
  { id: 'today' as FilterID, label: '今日', icon: Clock3 },
  { id: 'normal' as FilterID, label: '普通', icon: MailOpen },
  { id: 'failed' as FilterID, label: '异常', icon: ShieldAlert },
]

const emailSubs = computed(() => subs.value.filter(sub => sub.source_type === 'email'))
const importantCount = computed(() => items.value.filter(item => item.status === 'important').length)
const todayCount = computed(() => items.value.filter(isTodayMail).length)

const visibleItems = computed(() => {
  const term = query.value.trim().toLowerCase()
  return sortedItems(items.value).filter((item) => {
    const haystack = `${subject(item)} ${sender(item)} ${summary(item)} ${category(item)} ${tags(item).join(' ')}`.toLowerCase()
    if (term && !haystack.includes(term)) return false
    if (activeFilter.value === 'important') return item.status === 'important'
    if (activeFilter.value === 'today') return isTodayMail(item)
    if (activeFilter.value === 'normal') return item.status === 'normal'
    if (activeFilter.value === 'failed') return item.status === 'failed'
    return true
  })
})

const readerTitle = computed(() => readerArticle.value?.title || (readerFallback.value ? subject(readerFallback.value) : '邮件详情'))
const readerSummary = computed(() => paragraphs(readerArticle.value?.summary || (readerFallback.value ? summary(readerFallback.value) : '')))
const readerContent = computed(() => paragraphs(readerArticle.value?.content || snippet(readerFallback.value)))

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [nextSubs, nextItems] = await Promise.all([
      api.getSubscriptions(),
      api.getSourceItems({ source_type: 'email', limit: 200 }),
    ])
    subs.value = nextSubs
    items.value = nextItems
    loaded.value = true
  } catch {
    error.value = '无法读取邮件摘要，请确认 API 服务可用。'
  } finally {
    loading.value = false
  }
}

async function syncAll() {
  if (!emailSubs.value.length || syncing.value) return
  syncing.value = true
  error.value = ''
  try {
    for (const sub of emailSubs.value) {
      if (sub.enabled) await api.refreshSubscription(sub.id)
    }
    await load()
  } catch {
    await load()
    error.value = '邮箱同步失败，已在订阅源状态里记录错误。'
  } finally {
    syncing.value = false
  }
}

async function openItem(item: SourceItem) {
  readerFallback.value = item
  readerArticle.value = null
  readerMode.value = 'summary'
  readerOpen.value = true
  const articleID = stringPayload(item, 'article_id')
  if (!articleID) return
  try {
    readerArticle.value = await api.getArticle(articleID)
  } catch {
    readerArticle.value = null
  }
}

function closeReader() {
  readerOpen.value = false
  readerArticle.value = null
  readerFallback.value = null
}

function sortedItems(source: SourceItem[]) {
  return [...source].sort((a, b) => {
    if (a.status === 'important' && b.status !== 'important') return -1
    if (a.status !== 'important' && b.status === 'important') return 1
    return receivedTimestamp(b) - receivedTimestamp(a)
  })
}

function subject(item: SourceItem) {
  return stringPayload(item, 'subject') || item.title || '(无主题)'
}

function sender(item: SourceItem) {
  return stringPayload(item, 'from') || item.course || '未知发件人'
}

function account(item: SourceItem) {
  return stringPayload(item, 'account')
}

function category(item: SourceItem) {
  return stringPayload(item, 'category')
}

function tags(item: SourceItem) {
  const raw = item.payload?.tags
  return Array.isArray(raw) ? raw.map(String).filter(Boolean) : []
}

function summary(item: SourceItem) {
  return stringPayload(item, 'summary') || snippet(item) || '等待分析摘要'
}

function snippet(item: SourceItem | null) {
  if (!item) return ''
  return stringPayload(item, 'snippet')
}

function statusLabel(item: SourceItem) {
  if (item.status === 'important') return '重要'
  if (item.status === 'failed') return '异常'
  if (item.status === 'spam') return '已过滤'
  if (item.status === 'empty') return '空正文'
  if (item.status === 'pending') return '待分析'
  return '普通'
}

function receivedLabel(item: SourceItem) {
  const received = receivedDate(item)
  if (!received) return '时间未知'
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(received)
}

function isTodayMail(item: SourceItem) {
  const date = receivedDate(item)
  if (!date) return false
  const now = new Date()
  return date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth() && date.getDate() === now.getDate()
}

function receivedTimestamp(item: SourceItem) {
  return receivedDate(item)?.getTime() || new Date(item.last_seen_at).getTime() || 0
}

function receivedDate(item: SourceItem) {
  const raw = stringPayload(item, 'received_at')
  const date = raw ? new Date(raw) : new Date(item.last_seen_at)
  return Number.isNaN(date.getTime()) ? null : date
}

function stringPayload(item: SourceItem, key: string) {
  const value = item.payload?.[key]
  return typeof value === 'string' ? value.trim() : ''
}

function paragraphs(value: string) {
  const parts = value
    .split(/\n{2,}|\r?\n/)
    .map(part => part.trim())
    .filter(Boolean)
  return parts.length ? parts : ['暂无内容']
}

onMounted(load)
</script>
