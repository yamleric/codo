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
      <label for="feed-url">Feed 地址</label>
      <div>
        <input
          id="feed-url"
          v-model="draft.feed_url"
          type="url"
          placeholder="https://example.com/feed.xml"
          @input="error = ''"
        />
        <button type="submit" title="添加订阅源" :disabled="saving || !draft.feed_url.trim()">
          <LoaderCircle v-if="saving" :size="16" class="spinning" />
          <ArrowRight v-else :size="16" />
        </button>
      </div>
      <div v-if="!compact" class="source-form-meta">
        <input v-model="draft.title" type="text" placeholder="显示名称，可选" @input="error = ''" />
        <input v-model="draft.category" type="text" placeholder="分组，例如 技术 / 新闻" @input="error = ''" />
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
          <Rss v-else :size="15" />
        </span>
        <div class="source-body">
          <div class="source-title-line">
            <strong>{{ displayName(sub) }}</strong>
            <span class="source-badge" :class="{ paused: !sub.enabled, danger: !!sub.last_error }">
              {{ statusLabel(sub) }}
            </span>
            <span v-if="sub.category" class="source-badge">{{ sub.category }}</span>
          </div>
          <span class="source-url">{{ sub.feed_url }}</span>
          <small v-if="sub.last_error" class="source-error">{{ sub.last_error }}</small>
        </div>
        <span class="source-time">
          <Clock3 :size="12" />
          {{ sub.last_fetched_at ? timeAgo(sub.last_fetched_at) : '等待首次拉取' }}
        </span>
        <div class="source-actions">
          <button type="button" title="立即刷新" :disabled="busyID === sub.id" @click="refresh(sub)">
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
        <label>
          <span>Feed 地址</span>
          <input v-model="editDraft.feed_url" type="url" required />
        </label>
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
  LoaderCircle,
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
const draft = reactive({ feed_url: '', title: '', category: '' })
const editDraft = reactive({ feed_url: '', title: '', category: '', enabled: true })

const filters: { id: FilterID; label: string }[] = [
  { id: 'all', label: '全部' },
  { id: 'enabled', label: '启用' },
  { id: 'paused', label: '暂停' },
  { id: 'error', label: '异常' },
]

const enabledCount = computed(() => subs.value.filter(sub => sub.enabled).length)
const errorCount = computed(() => subs.value.filter(sub => !!sub.last_error).length)
const categoryCount = computed(() => new Set(subs.value.map(sub => sub.category).filter(Boolean)).size)

const filteredSubs = computed(() => {
  const term = query.value.trim().toLowerCase()
  return subs.value.filter((sub) => {
    const haystack = `${displayName(sub)} ${sub.feed_url} ${sub.category}`.toLowerCase()
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

async function add() {
  if (!draft.feed_url.trim() || saving.value) return
  saving.value = true
  error.value = ''
  try {
    await api.addSubscription({
      feed_url: draft.feed_url.trim(),
      title: draft.title.trim(),
      category: draft.category.trim(),
    })
    draft.feed_url = ''
    draft.title = ''
    draft.category = ''
    showAdd.value = false
    await load()
  } catch {
    error.value = '添加失败，请检查 Feed 地址。'
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
}

async function saveEdit() {
  if (!editing.value || saving.value) return
  saving.value = true
  editError.value = ''
  try {
    await api.updateSubscription(editing.value.id, {
      feed_url: editDraft.feed_url.trim(),
      title: editDraft.title.trim(),
      category: editDraft.category.trim(),
      enabled: editDraft.enabled,
    })
    editing.value = null
    await load()
  } catch {
    editError.value = '保存失败，请检查 Feed 地址或稍后重试。'
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
  return feedName(sub.feed_url)
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

function timeAgo(iso: string) {
  const diff = Date.now() - new Date(iso).getTime()
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)} 分钟前`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)} 小时前`
  return `${Math.floor(diff / 86400000)} 天前`
}

onMounted(load)
</script>
