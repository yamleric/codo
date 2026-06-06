<template>
  <section class="bookmark-panel">
    <header class="section-heading">
      <div>
        <span class="section-kicker">BOOKMARKS</span>
        <h2>收藏夹</h2>
      </div>
      <div class="source-heading-actions">
        <button type="button" class="icon-button" title="刷新收藏夹" :disabled="loading" @click="load">
          <RefreshCw :size="16" :class="{ spinning: loading }" />
        </button>
        <button type="button" class="icon-button" title="同步待处理收藏" :disabled="syncing || !syncableCount" @click="syncPending">
          <FolderSync :size="16" :class="{ spinning: syncing }" />
        </button>
      </div>
    </header>

    <div class="bookmark-summary-strip" aria-label="收藏夹概览">
      <article>
        <strong>{{ bookmarks.length }}</strong>
        <span>全部收藏</span>
      </article>
      <article>
        <strong>{{ pendingCount }}</strong>
        <span>待同步</span>
      </article>
      <article>
        <strong>{{ syncedCount }}</strong>
        <span>已同步</span>
      </article>
      <article>
        <strong>{{ folderCount }}</strong>
        <span>收藏夹</span>
      </article>
    </div>

    <div class="bookmark-import">
      <form class="bookmark-url-form" @submit.prevent="importSingle">
        <label for="bookmark-url">导入收藏网址</label>
        <div>
          <input id="bookmark-url" v-model="singleURL" type="url" placeholder="https://example.com/article" @input="notice = ''" />
          <input v-model="defaultFolder" type="text" placeholder="收藏夹名，可选" @input="notice = ''" />
          <button type="submit" :disabled="importing || !singleURL.trim()" title="导入网址">
            <LoaderCircle v-if="importing" :size="15" class="spinning" />
            <Import v-else :size="15" />
          </button>
        </div>
      </form>
      <form class="bookmark-text-form" @submit.prevent="importText">
        <label for="bookmark-text">批量导入</label>
        <textarea
          id="bookmark-text"
          v-model="bulkText"
          placeholder="粘贴浏览器导出的书签文本、网页列表或多行 URL"
          @input="notice = ''"
        ></textarea>
        <footer>
          <span>{{ extractedCount }} 个可识别链接</span>
          <button type="submit" :disabled="importing || !bulkText.trim()">
            <LoaderCircle v-if="importing" :size="15" class="spinning" />
            <Upload v-else :size="15" />
            导入文本
          </button>
        </footer>
      </form>
    </div>

    <div v-if="notice" class="bookmark-notice" :class="{ danger: noticeType === 'error' }">
      <CircleAlert v-if="noticeType === 'error'" :size="15" />
      <CheckCircle2 v-else :size="15" />
      <span>{{ notice }}</span>
    </div>

    <div class="bookmark-toolbar">
      <label class="search-field">
        <Search :size="15" />
        <input v-model="query" type="search" placeholder="搜索标题、收藏夹或 URL" />
      </label>
      <div class="filter-tabs" aria-label="收藏状态筛选">
        <button v-for="item in filters" :key="item.id" type="button" :class="{ active: activeFilter === item.id }" @click="activeFilter = item.id">
          {{ item.label }}
        </button>
      </div>
    </div>

    <div v-if="visibleBookmarks.length" class="bookmark-list">
      <article v-for="bookmark in visibleBookmarks" :key="bookmark.id" class="bookmark-row" :class="{ failed: bookmark.status === 'failed' }">
        <span class="bookmark-icon" :class="bookmark.status">
          <LoaderCircle v-if="bookmark.status === 'syncing'" :size="15" class="spinning" />
          <CheckCircle2 v-else-if="bookmark.status === 'synced'" :size="15" />
          <CircleAlert v-else-if="bookmark.status === 'failed'" :size="15" />
          <BookmarkIcon v-else :size="15" />
        </span>
        <div class="bookmark-main">
          <div class="bookmark-title-line">
            <strong>{{ displayName(bookmark) }}</strong>
            <span class="bookmark-badge" :class="bookmark.status">{{ statusLabel(bookmark.status) }}</span>
            <span v-if="bookmark.folder" class="bookmark-badge folder">{{ bookmark.folder }}</span>
          </div>
          <span class="bookmark-url">{{ bookmark.url }}</span>
          <small v-if="bookmark.last_error" class="bookmark-error">{{ bookmark.last_error }}</small>
          <small v-else-if="bookmark.last_synced_at" class="bookmark-time">上次同步 {{ timeAgo(bookmark.last_synced_at) }}</small>
        </div>
        <div class="bookmark-actions">
          <button type="button" title="同步此收藏" :disabled="syncing || bookmark.status === 'syncing'" @click="syncOne(bookmark)">
            <FolderSync :size="14" />
          </button>
          <a v-if="bookmark.last_task_id" :href="`#task-${bookmark.last_task_id}`" title="已创建任务">
            <Send :size="14" />
          </a>
          <a :href="bookmark.url" target="_blank" rel="noreferrer" title="打开原链接">
            <ExternalLink :size="14" />
          </a>
          <button type="button" title="编辑收藏" @click="startEdit(bookmark)">
            <Pencil :size="14" />
          </button>
          <button type="button" title="删除收藏" class="danger" @click="remove(bookmark)">
            <Trash2 :size="14" />
          </button>
        </div>
      </article>
    </div>

    <div v-else-if="loaded" class="inline-empty">
      <Inbox :size="18" />
      <p>{{ bookmarks.length ? '没有匹配的收藏' : '还没有收藏网址' }}</p>
      <span>{{ bookmarks.length ? '调整搜索或状态筛选。' : '导入 URL 后，点击同步即可进入抓取总结流程。' }}</span>
    </div>
    <div v-else class="loading-row"><LoaderCircle :size="16" class="spinning" />读取收藏夹</div>

    <div v-if="editing" class="source-edit-backdrop" @click.self="editing = null">
      <form class="source-edit-dialog" @submit.prevent="saveEdit">
        <header>
          <div>
            <span class="section-kicker">EDIT BOOKMARK</span>
            <h3>编辑收藏</h3>
          </div>
          <button type="button" class="icon-button" title="关闭" @click="editing = null">
            <X :size="16" />
          </button>
        </header>
        <label>
          <span>显示名称</span>
          <input v-model="editDraft.title" type="text" placeholder="默认显示域名" />
        </label>
        <label>
          <span>收藏夹</span>
          <input v-model="editDraft.folder" type="text" placeholder="例如 待读 / 产品 / 技术" />
        </label>
        <label>
          <span>备注</span>
          <input v-model="editDraft.note" type="text" placeholder="可选备注" />
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
  Bookmark as BookmarkIcon,
  CheckCircle2,
  CircleAlert,
  ExternalLink,
  FolderSync,
  Import,
  Inbox,
  LoaderCircle,
  Pencil,
  RefreshCw,
  Search,
  Send,
  Trash2,
  Upload,
  X,
} from '@lucide/vue'
import { api } from '../api'
import type { Bookmark } from '../types'

type FilterID = 'all' | 'pending' | 'synced' | 'failed'
type NoticeType = 'success' | 'error'

const bookmarks = ref<Bookmark[]>([])
const loading = ref(false)
const loaded = ref(false)
const importing = ref(false)
const syncing = ref(false)
const saving = ref(false)
const singleURL = ref('')
const defaultFolder = ref('')
const bulkText = ref('')
const query = ref('')
const activeFilter = ref<FilterID>('all')
const notice = ref('')
const noticeType = ref<NoticeType>('success')
const editing = ref<Bookmark | null>(null)
const editError = ref('')
const editDraft = reactive({ title: '', folder: '', note: '' })

const filters: { id: FilterID; label: string }[] = [
  { id: 'all', label: '全部' },
  { id: 'pending', label: '待同步' },
  { id: 'synced', label: '已同步' },
  { id: 'failed', label: '异常' },
]

const pendingCount = computed(() => bookmarks.value.filter(item => item.status === 'pending' || item.status === 'failed').length)
const syncedCount = computed(() => bookmarks.value.filter(item => item.status === 'synced').length)
const folderCount = computed(() => new Set(bookmarks.value.map(item => item.folder).filter(Boolean)).size)
const syncableCount = computed(() => bookmarks.value.filter(item => item.status === 'pending' || item.status === 'failed').length)
const extractedCount = computed(() => extractURLs(bulkText.value).length)

const visibleBookmarks = computed(() => {
  const term = query.value.trim().toLowerCase()
  return bookmarks.value.filter((bookmark) => {
    const haystack = `${displayName(bookmark)} ${bookmark.url} ${bookmark.folder} ${bookmark.note}`.toLowerCase()
    if (term && !haystack.includes(term)) return false
    if (activeFilter.value === 'pending') return bookmark.status === 'pending' || bookmark.status === 'syncing'
    if (activeFilter.value === 'synced') return bookmark.status === 'synced'
    if (activeFilter.value === 'failed') return bookmark.status === 'failed'
    return true
  })
})

async function load() {
  loading.value = true
  try {
    bookmarks.value = await api.getBookmarks()
  } catch {
    showNotice('无法读取收藏夹，请确认 API 服务可用。', 'error')
  } finally {
    loaded.value = true
    loading.value = false
  }
}

async function importSingle() {
  if (!singleURL.value.trim()) return
  await importPayload({ url: singleURL.value.trim(), folder: defaultFolder.value.trim() })
  singleURL.value = ''
}

async function importText() {
  if (!bulkText.value.trim()) return
  await importPayload({ text: bulkText.value, folder: defaultFolder.value.trim() })
  bulkText.value = ''
}

async function importPayload(payload: { url?: string; text?: string; folder?: string }) {
  importing.value = true
  try {
    const result = await api.importBookmarks(payload)
    await load()
    showNotice(`导入 ${result.imported} 个，更新 ${result.updated} 个，跳过 ${result.skipped} 个。`, 'success')
  } catch {
    showNotice('导入失败，请检查网址格式。', 'error')
  } finally {
    importing.value = false
  }
}

async function syncPending() {
  await syncIDs()
}

async function syncOne(bookmark: Bookmark) {
  await syncIDs([bookmark.id])
}

async function syncIDs(ids?: string[]) {
  syncing.value = true
  try {
    const result = await api.syncBookmarks(ids)
    await load()
    showNotice(`已提交 ${result.queued} 个收藏到处理队列。`, 'success')
  } catch {
    showNotice('同步失败，请稍后重试。', 'error')
  } finally {
    syncing.value = false
  }
}

function startEdit(bookmark: Bookmark) {
  editing.value = bookmark
  editError.value = ''
  editDraft.title = bookmark.title || ''
  editDraft.folder = bookmark.folder || ''
  editDraft.note = bookmark.note || ''
}

async function saveEdit() {
  if (!editing.value) return
  saving.value = true
  editError.value = ''
  try {
    await api.updateBookmark(editing.value.id, {
      title: editDraft.title.trim(),
      folder: editDraft.folder.trim(),
      note: editDraft.note.trim(),
    })
    editing.value = null
    await load()
  } catch {
    editError.value = '保存失败，请稍后重试。'
  } finally {
    saving.value = false
  }
}

async function remove(bookmark: Bookmark) {
  if (!window.confirm(`删除收藏「${displayName(bookmark)}」？`)) return
  await api.deleteBookmark(bookmark.id)
  await load()
}

function showNotice(message: string, type: NoticeType) {
  notice.value = message
  noticeType.value = type
}

function displayName(bookmark: Bookmark) {
  if (bookmark.title?.trim()) return bookmark.title.trim()
  try {
    return new URL(bookmark.url).hostname.replace(/^www\./, '')
  } catch {
    return '收藏链接'
  }
}

function statusLabel(status: string) {
  if (status === 'synced') return '已同步'
  if (status === 'syncing') return '同步中'
  if (status === 'failed') return '异常'
  return '待同步'
}

function timeAgo(iso: string) {
  const diff = Date.now() - new Date(iso).getTime()
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)} 分钟前`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)} 小时前`
  return `${Math.floor(diff / 86400000)} 天前`
}

function extractURLs(text: string) {
  const matches = text.match(/https?:\/\/[^\s<>"']+/g) ?? []
  return Array.from(new Set(matches.map(item => item.replace(/[ \t\r\n.,;:!?，。；：！？)\]}）】》>]+$/g, ''))))
}

onMounted(load)
</script>
