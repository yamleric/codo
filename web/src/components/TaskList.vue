<template>
  <section class="task-panel" :class="{ compact }">
    <header class="task-panel-header">
      <div>
        <span class="section-kicker">ACTIVITY</span>
        <h2>{{ title }}</h2>
      </div>
      <span class="task-total">{{ filteredTasks.length }} / {{ tasks.length }}</span>
    </header>

    <div class="task-toolbar">
      <label class="search-field">
        <Search :size="15" />
        <input v-model="query" type="search" placeholder="搜索链接、摘要或标签" />
      </label>
      <div class="task-filters">
        <div class="filter-tabs" aria-label="任务状态筛选">
          <button
            v-for="item in filters"
            :key="item.id"
            type="button"
            :class="{ active: activeFilter === item.id }"
            @click="activeFilter = item.id"
          >{{ item.label }}</button>
        </div>
        <div v-if="categoryFilters.length > 1" class="filter-tabs category-tabs" aria-label="任务分类筛选">
          <button
            v-for="item in categoryFilters"
            :key="item.id"
            type="button"
            :class="{ active: activeCategory === item.id }"
            @click="activeCategory = item.id"
          >{{ item.label }}</button>
        </div>
      </div>
    </div>

    <div v-if="filteredTasks.length" class="task-list">
      <article
        v-for="task in visibleTasks"
        :key="task.id"
        class="task-row"
        :class="{ expanded: expanded.has(task.id) }"
      >
        <button type="button" class="task-summary" @click="toggle(task.id)">
          <span :class="statusTone(task.status)" class="status-dot"></span>
          <div class="task-main">
            <div class="task-meta">
              <span :class="statusTone(task.status)" class="status-label">{{ statusLabel(task.status) }}</span>
              <span>{{ contentTypeLabel(task.content_type) }}</span>
              <span>{{ sourceLabel(task.source) }}</span>
              <span>{{ timeAgo(task.created_at) }}</span>
            </div>
            <div class="task-title-line">
              <strong>{{ taskName(task.url) }}</strong>
              <span v-if="task.category" class="task-category">{{ task.category }}</span>
            </div>
            <span class="task-url">{{ task.url }}</span>
            <div v-if="task.tags?.length" class="task-tags">
              <span v-for="tag in task.tags.slice(0, 5)" :key="tag">{{ tag }}</span>
            </div>
            <p v-if="task.summary">{{ task.summary }}</p>
            <p v-else-if="task.error" class="task-error">{{ task.error }}</p>
          </div>
          <ChevronDown :size="16" class="chevron" />
        </button>

        <div v-if="expanded.has(task.id)" class="task-detail">
          <div v-if="task.steps?.length" class="step-list">
            <div v-for="(step, index) in task.steps" :key="`${step.label}-${index}`" class="step-row">
              <span :class="stepTone(step.status)" class="step-state">
                <Check v-if="step.status === 'ok'" :size="12" />
                <X v-else-if="step.status === 'error'" :size="12" />
                <Minus v-else :size="12" />
              </span>
              <strong>{{ step.label }}</strong>
              <span>{{ step.detail || '步骤已执行' }}</span>
              <time>{{ durationLabel(step.duration_ms) }}</time>
            </div>
          </div>
          <div v-else class="no-steps">任务尚未产生步骤记录。</div>
          <div class="task-actions">
            <a :href="task.url" target="_blank" rel="noreferrer">
              <ExternalLink :size="14" />打开原文
            </a>
            <button v-if="task.status === 'failed'" type="button" @click="retry(task.id)">
              <RotateCcw :size="14" />重试任务
            </button>
          </div>
        </div>
      </article>
    </div>

    <div v-else class="task-empty">
      <Inbox :size="22" />
      <strong>{{ tasks.length ? '没有匹配的任务' : '等待第一个任务' }}</strong>
      <span>{{ tasks.length ? '调整状态筛选或搜索词。' : '提交链接后，处理进度会实时显示在这里。' }}</span>
    </div>

    <button
      v-if="compact && filteredTasks.length > compactLimit"
      type="button"
      class="show-more"
      @click="showAll = !showAll"
    >
      {{ showAll ? '收起任务' : `查看其余 ${filteredTasks.length - compactLimit} 条任务` }}
      <ChevronDown :size="14" :class="{ rotated: showAll }" />
    </button>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Check, ChevronDown, ExternalLink, Inbox, Minus, RotateCcw, Search, X } from '@lucide/vue'
import { api } from '../api'
import type { Task } from '../types'

type FilterID = 'all' | 'running' | 'done' | 'failed'
type CategoryFilterID = 'all' | string

const props = withDefaults(defineProps<{ tasks: Task[]; title?: string; compact?: boolean }>(), {
  title: '任务',
  compact: false,
})
const emit = defineEmits<{ updated: [] }>()

const expanded = ref(new Set<string>())
const query = ref('')
const activeFilter = ref<FilterID>('all')
const activeCategory = ref<CategoryFilterID>('all')
const showAll = ref(false)
const compactLimit = 6

const filters: { id: FilterID; label: string }[] = [
  { id: 'all', label: '全部' },
  { id: 'running', label: '进行中' },
  { id: 'done', label: '完成' },
  { id: 'failed', label: '失败' },
]

const categoryFilters = computed(() => {
  const categories = Array.from(
    new Set(props.tasks.map(task => task.category).filter(Boolean)),
  ).sort((a, b) => a.localeCompare(b, 'zh-CN'))
  return [
    { id: 'all', label: '全部分类' },
    ...categories.map(category => ({ id: category, label: category })),
  ]
})

const filteredTasks = computed(() => {
  const term = query.value.trim().toLowerCase()
  return props.tasks.filter((task) => {
    const haystack = `${task.url} ${task.summary} ${task.category} ${(task.tags || []).join(' ')}`.toLowerCase()
    const matchesTerm = !term || haystack.includes(term)
    if (!matchesTerm) return false
    if (activeCategory.value !== 'all' && task.category !== activeCategory.value) return false
    if (activeFilter.value === 'running') return !['done', 'failed', 'discarded'].includes(task.status)
    if (activeFilter.value === 'done') return task.status === 'done' || task.status === 'discarded'
    if (activeFilter.value === 'failed') return task.status === 'failed'
    return true
  })
})

const visibleTasks = computed(() => {
  if (!props.compact || showAll.value) return filteredTasks.value
  return filteredTasks.value.slice(0, compactLimit)
})

function toggle(id: string) {
  expanded.value.has(id) ? expanded.value.delete(id) : expanded.value.add(id)
}

async function retry(id: string) {
  await api.retry(id)
  emit('updated')
}

function statusLabel(status: string) {
  const labels: Record<string, string> = {
    pending: '等待',
    fetching: '抓取中',
    filtering: '过滤中',
    analyzing: '分析中',
    saving: '存储中',
    notifying: '推送中',
    done: '完成',
    failed: '失败',
    discarded: '已丢弃',
  }
  return labels[status] ?? status
}

function statusTone(status: string) {
  if (status === 'done') return 'success'
  if (status === 'failed') return 'danger'
  if (status === 'discarded') return 'muted'
  return 'active'
}

function stepTone(status: string) {
  if (status === 'ok') return 'success'
  if (status === 'error') return 'danger'
  return 'muted'
}

function sourceLabel(source: string) {
  const labels: Record<string, string> = {
    manual: '手动提交',
    rss: 'RSS',
    wechat_mp: '公众号',
    linux_do: 'linux.do',
    chaoxing: '学习通',
    email: '邮件',
    group_chat: '群消息',
  }
  return labels[source] ?? source ?? '手动提交'
}

function contentTypeLabel(contentType: string) {
  const labels: Record<string, string> = {
    webpage: '网页',
    video: '视频',
    email: '邮件',
    post: '帖子',
    message: '消息',
  }
  return labels[contentType] ?? contentType ?? '内容'
}

function taskName(url: string) {
  try {
    const parsed = new URL(url)
    const path = parsed.pathname.replace(/\/$/, '').split('/').filter(Boolean).pop()
    return path ? decodeURIComponent(path).slice(0, 80) : parsed.hostname
  } catch {
    return url
  }
}

function durationLabel(ms: number) {
  if (!ms) return '—'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

function timeAgo(iso: string) {
  const diff = Date.now() - new Date(iso).getTime()
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)} 分钟前`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)} 小时前`
  return `${Math.floor(diff / 86400000)} 天前`
}
</script>
