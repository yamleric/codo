<template>
  <section class="subscription-panel" :class="{ compact }">
    <header class="section-heading">
      <div>
        <span class="section-kicker">SOURCES</span>
        <h2>RSS 订阅</h2>
      </div>
      <button
        type="button"
        class="icon-button"
        :title="showAdd ? '关闭添加表单' : '添加订阅源'"
        @click="showAdd = !showAdd"
      >
        <X v-if="showAdd" :size="16" />
        <Plus v-else :size="16" />
      </button>
    </header>

    <form v-if="showAdd" class="source-form" @submit.prevent="add">
      <label for="feed-url">Feed 地址</label>
      <div>
        <input
          id="feed-url"
          v-model="newUrl"
          type="url"
          placeholder="RSS / Atom Feed URL"
          @input="error = ''"
        />
        <button type="submit" title="添加订阅源" :disabled="adding || !newUrl.trim()">
          <LoaderCircle v-if="adding" :size="16" class="spinning" />
          <ArrowRight v-else :size="16" />
        </button>
      </div>
      <p v-if="error" class="field-error"><CircleAlert :size="14" />{{ error }}</p>
    </form>

    <div v-if="subs.length" class="source-list">
      <article v-for="sub in subs" :key="sub.id" class="source-row">
        <span class="source-icon"><Rss :size="15" /></span>
        <div>
          <strong>{{ feedName(sub.feed_url) }}</strong>
          <span>{{ sub.feed_url }}</span>
        </div>
        <span class="source-time">
          <Clock3 :size="12" />
          {{ sub.last_fetched_at ? timeAgo(sub.last_fetched_at) : '等待首次拉取' }}
        </span>
      </article>
    </div>
    <div v-else-if="loaded" class="inline-empty">
      <Rss :size="18" />
      <p>还没有自动订阅源</p>
      <span>添加 RSS 或 Atom Feed 后，Codo 会定时检查新内容。</span>
    </div>
    <div v-else class="loading-row"><LoaderCircle :size="16" class="spinning" />读取订阅源</div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ArrowRight, CircleAlert, Clock3, LoaderCircle, Plus, Rss, X } from '@lucide/vue'
import { api } from '../api'
import type { Subscription } from '../types'

defineProps<{ compact?: boolean }>()

const subs = ref<Subscription[]>([])
const showAdd = ref(false)
const newUrl = ref('')
const adding = ref(false)
const loaded = ref(false)
const error = ref('')

async function load() {
  try {
    subs.value = await api.getSubscriptions()
  } catch {
    error.value = '无法读取订阅源。'
  }
  loaded.value = true
}

async function add() {
  if (!newUrl.value.trim() || adding.value) return
  adding.value = true
  error.value = ''
  try {
    await api.addSubscription(newUrl.value.trim())
    newUrl.value = ''
    showAdd.value = false
    await load()
  } catch {
    error.value = '添加失败，请检查 Feed 地址或 API 服务。'
  } finally {
    adding.value = false
  }
}

function feedName(url: string) {
  try {
    return new URL(url).hostname.replace(/^www\./, '')
  } catch {
    return 'RSS Feed'
  }
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
