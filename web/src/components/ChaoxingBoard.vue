<template>
  <section class="chaoxing-panel">
    <header class="section-heading">
      <div>
        <span class="section-kicker">CHAOXING</span>
        <h2>学习通作业考试</h2>
      </div>
      <button type="button" class="icon-button" title="刷新学习通列表" :disabled="loading" @click="load">
        <RefreshCw :size="16" :class="{ spinning: loading }" />
      </button>
    </header>

    <div v-if="error" class="source-alert">
      <CircleAlert :size="15" />
      <span>{{ error }}</span>
      <button type="button" @click="load">重试</button>
    </div>

    <div class="chaoxing-summary-strip" aria-label="学习通概览">
      <article>
        <strong>{{ items.length }}</strong>
        <span>全部条目</span>
      </article>
      <article>
        <strong>{{ actionableCount }}</strong>
        <span>待处理</span>
      </article>
      <article>
        <strong>{{ dueSoonCount }}</strong>
        <span>截止关注</span>
      </article>
      <article>
        <strong>{{ courseCount }}</strong>
        <span>课程</span>
      </article>
    </div>

    <div class="chaoxing-toolbar">
      <label class="search-field chaoxing-search">
        <Search :size="15" />
        <input v-model="query" type="search" placeholder="搜索课程、标题或状态" />
      </label>
      <div class="filter-tabs" aria-label="学习通条目筛选">
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

    <div v-if="visibleItems.length" class="chaoxing-list">
      <article v-for="item in visibleItems" :key="item.id" class="chaoxing-row" :class="{ urgent: isDueSoon(item), completed: isCompleted(item) }">
        <span class="chaoxing-kind" :class="item.item_type">
          <ClipboardList v-if="item.item_type === 'homework'" :size="15" />
          <FileCheck2 v-else :size="15" />
        </span>

        <div class="chaoxing-main">
          <div class="chaoxing-title-line">
            <strong>{{ item.title || typeLabel(item.item_type) }}</strong>
            <span class="chaoxing-badge" :class="item.item_type">{{ typeLabel(item.item_type) }}</span>
            <span class="chaoxing-status" :class="statusClass(item)">{{ statusLabel(item) }}</span>
          </div>
          <span class="chaoxing-course">{{ item.course || '未识别课程' }}</span>
          <div class="chaoxing-meta">
            <span :class="{ danger: isDueSoon(item) }">
              <CalendarClock :size="12" />
              {{ dueLabel(item) }}
            </span>
            <span>
              <Clock3 :size="12" />
              {{ timeAgo(item.last_seen_at) }}
            </span>
          </div>
        </div>

        <div class="chaoxing-actions">
          <a v-if="item.url" :href="item.url" target="_blank" rel="noreferrer" title="打开学习通页面">
            <ExternalLink :size="14" />
          </a>
        </div>
      </article>
    </div>

    <div v-else-if="loaded" class="inline-empty">
      <GraduationCap :size="18" />
      <p>{{ items.length ? '没有匹配的作业或考试' : '还没有学习通条目' }}</p>
      <span>{{ items.length ? '调整搜索或筛选条件。' : '在订阅源里配置学习通并刷新后，作业和考试会显示在这里。' }}</span>
    </div>
    <div v-else class="loading-row"><LoaderCircle :size="16" class="spinning" />读取学习通条目</div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  CalendarClock,
  CheckCircle2,
  CircleAlert,
  ClipboardList,
  Clock3,
  ExternalLink,
  FileCheck2,
  GraduationCap,
  Inbox,
  LoaderCircle,
  RefreshCw,
  Search,
} from '@lucide/vue'
import { api } from '../api'
import type { SourceItem } from '../types'

type FilterID = 'all' | 'actionable' | 'due' | 'homework' | 'exam' | 'done'

const items = ref<SourceItem[]>([])
const loading = ref(false)
const loaded = ref(false)
const error = ref('')
const query = ref('')
const activeFilter = ref<FilterID>('actionable')

const filters = [
  { id: 'all' as FilterID, label: '全部', icon: Inbox },
  { id: 'actionable' as FilterID, label: '待处理', icon: ClipboardList },
  { id: 'due' as FilterID, label: '临近截止', icon: CalendarClock },
  { id: 'homework' as FilterID, label: '作业', icon: ClipboardList },
  { id: 'exam' as FilterID, label: '考试', icon: FileCheck2 },
  { id: 'done' as FilterID, label: '已完成', icon: CheckCircle2 },
]

const actionableCount = computed(() => items.value.filter(isActionable).length)
const dueSoonCount = computed(() => items.value.filter(isDueSoon).length)
const courseCount = computed(() => new Set(items.value.map(item => item.course).filter(Boolean)).size)

const visibleItems = computed(() => {
  const term = query.value.trim().toLowerCase()
  return sortedItems(items.value).filter((item) => {
    const haystack = `${item.course} ${item.title} ${item.status} ${item.item_type}`.toLowerCase()
    if (term && !haystack.includes(term)) return false
    if (activeFilter.value === 'actionable') return isActionable(item)
    if (activeFilter.value === 'due') return isDueSoon(item)
    if (activeFilter.value === 'homework') return item.item_type === 'homework'
    if (activeFilter.value === 'exam') return item.item_type === 'exam'
    if (activeFilter.value === 'done') return isCompleted(item)
    return true
  })
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    items.value = await api.getSourceItems({ source_type: 'chaoxing', limit: 200 })
    loaded.value = true
  } catch {
    error.value = '无法读取学习通作业考试，请确认 API 服务可用。'
  } finally {
    loading.value = false
  }
}

function sortedItems(source: SourceItem[]) {
  return [...source].sort((a, b) => {
    const rankDiff = itemRank(a) - itemRank(b)
    if (rankDiff !== 0) return rankDiff
    if (itemRank(a) <= 1) return dueTimestamp(a) - dueTimestamp(b)
    return new Date(b.last_seen_at).getTime() - new Date(a.last_seen_at).getTime()
  })
}

function itemRank(item: SourceItem) {
  if (isDueSoon(item)) return 0
  if (isActionable(item)) return 1
  if (isCompleted(item)) return 3
  return 2
}

function dueTimestamp(item: SourceItem) {
  if (!item.due_at) return Number.MAX_SAFE_INTEGER
  const value = new Date(item.due_at).getTime()
  return Number.isNaN(value) ? Number.MAX_SAFE_INTEGER : value
}

function isActionable(item: SourceItem) {
  const status = normalizeStatus(item.status)
  if (isCompleted(item)) return false
  return status === '' || status.includes('未') || status.includes('待') || status.includes('进行')
}

function isCompleted(item: SourceItem) {
  const status = normalizeStatus(item.status)
  return status.includes('已完成') || status.includes('已提交') || status.includes('已交') || status.includes('待批阅') || status.includes('过期')
}

function isDueSoon(item: SourceItem) {
  if (!item.due_at || !isActionable(item)) return false
  const due = new Date(item.due_at).getTime()
  if (Number.isNaN(due)) return false
  const now = Date.now()
  return due - now <= 7 * 24 * 60 * 60 * 1000
}

function typeLabel(type: string) {
  if (type === 'homework') return '作业'
  if (type === 'exam') return '考试'
  return type || '条目'
}

function statusLabel(item: SourceItem) {
  return item.status?.trim() || (isActionable(item) ? '待确认' : '未识别')
}

function statusClass(item: SourceItem) {
  if (isDueSoon(item)) return 'danger'
  if (isCompleted(item)) return 'done'
  if (isActionable(item)) return 'pending'
  return 'muted'
}

function dueLabel(item: SourceItem) {
  if (!item.due_at) return '未解析截止时间'
  const due = new Date(item.due_at)
  if (Number.isNaN(due.getTime())) return '截止时间异常'
  const delta = due.getTime() - Date.now()
  const dateText = formatDate(item.due_at)
  if (delta < 0) return `${dateText} 已截止`
  const hours = Math.ceil(delta / (60 * 60 * 1000))
  if (hours < 24) return `${dateText} · ${hours} 小时内`
  return `${dateText} · ${Math.ceil(hours / 24)} 天内`
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}

function timeAgo(value: string) {
  const date = new Date(value)
  const delta = Date.now() - date.getTime()
  if (Number.isNaN(delta)) return '刚刚更新'
  const minutes = Math.max(0, Math.floor(delta / 60000))
  if (minutes < 1) return '刚刚更新'
  if (minutes < 60) return `${minutes} 分钟前更新`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} 小时前更新`
  return `${Math.floor(hours / 24)} 天前更新`
}

function normalizeStatus(status: string) {
  return (status || '').trim()
}

onMounted(load)
</script>
