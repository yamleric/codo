<template>
  <section class="knowledge-panel">
    <header class="section-heading">
      <div>
        <span class="section-kicker">KNOWLEDGE</span>
        <h2>知识库</h2>
      </div>
      <button type="button" class="icon-button" title="刷新知识库" :disabled="loading" @click="load">
        <RefreshCw :size="16" :class="{ spinning: loading }" />
      </button>
    </header>

    <div v-if="error" class="source-alert">
      <CircleAlert :size="15" />
      <span>{{ error }}</span>
      <button type="button" @click="load">重试</button>
    </div>

    <div class="knowledge-summary-strip" aria-label="知识库概览">
      <article>
        <strong>{{ facets?.total ?? 0 }}</strong>
        <span>全部内容</span>
      </article>
      <article>
        <strong>{{ facets?.categories.length ?? 0 }}</strong>
        <span>分类</span>
      </article>
      <article>
        <strong>{{ facets?.tags.length ?? 0 }}</strong>
        <span>标签</span>
      </article>
      <article>
        <strong>{{ facets?.sources.length ?? 0 }}</strong>
        <span>来源</span>
      </article>
    </div>

    <div class="knowledge-toolbar">
      <label class="search-field knowledge-search">
        <Search :size="14" />
        <input v-model="queryDraft" type="search" placeholder="搜索正文、摘要、链接、分类或标签" @keydown.enter.prevent="applySearch" />
      </label>
      <button type="button" class="knowledge-search-button" :disabled="loading || !queryDraft.trim()" @click="applySearch">
        <SearchCheck :size="14" />
        搜索
      </button>
      <div class="filter-tabs">
        <button type="button" :class="{ active: facetMode === 'category' }" @click="setFacetMode('category')">
          <Layers :size="13" />
          分类
        </button>
        <button type="button" :class="{ active: facetMode === 'tag' }" @click="setFacetMode('tag')">
          <Tags :size="13" />
          标签
        </button>
      </div>
    </div>

    <section class="knowledge-qa-panel">
      <form class="knowledge-qa-form" @submit.prevent="askQuestion">
        <div class="knowledge-qa-title">
          <span><Bot :size="16" /></span>
          <div>
            <strong>知识库问答</strong>
            <small>{{ qaModeText }}</small>
          </div>
        </div>
        <div class="knowledge-qa-input">
          <input v-model="questionDraft" type="text" placeholder="问一个知识库里的问题" />
          <button type="submit" :disabled="qaLoading || !questionDraft.trim()">
            <LoaderCircle v-if="qaLoading" :size="15" class="spinning" />
            <Send v-else :size="15" />
            提问
          </button>
        </div>
      </form>

      <div v-if="qaError" class="source-alert qa-alert">
        <CircleAlert :size="15" />
        <span>{{ qaError }}</span>
      </div>

      <article v-if="qa" class="knowledge-answer">
        <header>
          <MessageSquareText :size="16" />
          <strong>{{ qa.question }}</strong>
        </header>
        <p>{{ qa.answer }}</p>
        <div v-if="qa.citations.length" class="knowledge-citations">
          <a
            v-for="citation in qa.citations"
            :key="citation.chunk_id"
            :href="citation.url || undefined"
            :target="citation.url ? '_blank' : undefined"
            rel="noreferrer"
          >
            <span>[{{ citation.index }}]</span>
            <strong>{{ citationTitle(citation) }}</strong>
            <small>{{ citation.category || sourceLabel(citation.source) }}</small>
          </a>
        </div>
      </article>
    </section>

    <div class="knowledge-layout">
      <aside class="knowledge-facets">
        <button type="button" :class="{ active: !activeFacet && !query }" @click="selectFacet('')">
          <span>全部</span>
          <strong>{{ facets?.total ?? 0 }}</strong>
        </button>
        <button
          v-for="facet in visibleFacets"
          :key="facet.name"
          type="button"
          :class="{ active: activeFacet === facet.name && !query }"
          @click="selectFacet(facet.name)"
        >
          <span>{{ facet.name }}</span>
          <strong>{{ facet.count }}</strong>
        </button>
      </aside>

      <div class="knowledge-list">
        <div v-if="loading && !articles.length && !searchResults.length" class="loading-row">
          <LoaderCircle :size="16" class="spinning" />
          读取知识库
        </div>

        <article v-for="result in searchResults" :key="result.chunk_id" class="knowledge-card search-result-card">
          <header>
            <div>
              <strong>{{ resultTitle(result) }}</strong>
              <span>{{ sourceLabel(result.source) }} · {{ contentTypeLabel(result.content_type) }} · {{ formatDate(result.created_at) }}</span>
            </div>
            <span class="task-category">{{ matchLabel(result.match) }}</span>
          </header>
          <button type="button" class="knowledge-summary-preview" @click="openArticle(result.article_id, 'summary')">
            <span>{{ resultPreview(result) }}</span>
          </button>
          <div class="knowledge-card-footer">
            <div class="task-tags">
              <button v-if="result.category" type="button" @click="selectCategory(result.category)">{{ result.category }}</button>
              <button v-for="tag in result.tags" :key="tag" type="button" @click="selectTag(tag)">{{ tag }}</button>
            </div>
            <div class="knowledge-card-actions">
              <div class="knowledge-feedback-actions" :aria-label="`反馈 ${resultTitle(result)}`">
                <button type="button" title="有用" :disabled="isFeedbackLoading(result.article_id)" @click="sendArticleFeedback(result.article_id, 'useful')">
                  <ThumbsUp :size="13" />
                </button>
                <button type="button" title="没用" :disabled="isFeedbackLoading(result.article_id)" @click="sendArticleFeedback(result.article_id, 'not_useful')">
                  <ThumbsDown :size="13" />
                </button>
                <button type="button" title="以后类似通知" :disabled="isFeedbackLoading(result.article_id)" @click="sendArticleFeedback(result.article_id, 'notify_similar')">
                  <BellRing :size="13" />
                </button>
                <button type="button" title="以后类似静默" :disabled="isFeedbackLoading(result.article_id)" @click="sendArticleFeedback(result.article_id, 'silent_similar')">
                  <Archive :size="13" />
                </button>
              </div>
              <button type="button" class="knowledge-read-button" @click="openArticle(result.article_id, 'content')">
                <BookOpenText :size="13" />
                解析
              </button>
              <a v-if="result.url" :href="result.url" target="_blank" rel="noreferrer">
                <ExternalLink :size="13" />
                原文
              </a>
            </div>
          </div>
        </article>

        <article v-for="article in articles" :key="article.id" class="knowledge-card">
          <header>
            <div>
              <strong>{{ articleTitle(article) }}</strong>
              <span>{{ sourceLabel(article.source) }} · {{ contentTypeLabel(article.content_type) }} · {{ formatDate(article.published_at || article.created_at) }}</span>
            </div>
            <span v-if="article.category" class="task-category">{{ article.category }}</span>
          </header>
          <button type="button" class="knowledge-summary-preview" @click="openArticle(article.id, 'summary')">
            <span>{{ articlePreview(article) }}</span>
          </button>
          <div class="knowledge-card-footer">
            <div v-if="article.tags?.length" class="task-tags">
              <button v-for="tag in article.tags" :key="tag" type="button" @click="selectTag(tag)">{{ tag }}</button>
            </div>
            <div class="knowledge-card-actions">
              <div class="knowledge-feedback-actions" :aria-label="`反馈 ${articleTitle(article)}`">
                <button type="button" title="有用" :disabled="isFeedbackLoading(article.id)" @click="sendArticleFeedback(article.id, 'useful')">
                  <ThumbsUp :size="13" />
                </button>
                <button type="button" title="没用" :disabled="isFeedbackLoading(article.id)" @click="sendArticleFeedback(article.id, 'not_useful')">
                  <ThumbsDown :size="13" />
                </button>
                <button type="button" title="以后类似通知" :disabled="isFeedbackLoading(article.id)" @click="sendArticleFeedback(article.id, 'notify_similar')">
                  <BellRing :size="13" />
                </button>
                <button type="button" title="以后类似静默" :disabled="isFeedbackLoading(article.id)" @click="sendArticleFeedback(article.id, 'silent_similar')">
                  <Archive :size="13" />
                </button>
              </div>
              <button type="button" class="knowledge-read-button" @click="openArticle(article.id, 'content')">
                <BookOpenText :size="13" />
                解析
              </button>
              <a v-if="article.url" :href="article.url" target="_blank" rel="noreferrer">
                <ExternalLink :size="13" />
                原文
              </a>
            </div>
          </div>
        </article>

        <div v-if="loaded && !articles.length && !searchResults.length" class="task-empty">
          <Database :size="20" />
          <strong>暂无内容</strong>
          <span>{{ emptyText }}</span>
        </div>
      </div>
    </div>

    <div v-if="readerOpen" class="article-reader-backdrop" @click.self="closeArticle">
      <aside class="article-reader" aria-label="解析内容浏览器">
        <header class="article-reader-header">
          <div>
            <span class="section-kicker">PARSED PAGE</span>
            <strong>{{ selectedArticle ? articleTitle(selectedArticle) : '读取解析内容' }}</strong>
            <small v-if="selectedArticle">{{ articleHost(selectedArticle.url) }} · {{ formatDate(selectedArticle.published_at || selectedArticle.created_at) }}</small>
          </div>
          <div class="article-reader-actions">
            <a v-if="selectedArticle?.url" :href="selectedArticle.url" target="_blank" rel="noreferrer" title="打开原网页">
              <ExternalLink :size="14" />
              打开原网页
            </a>
            <button type="button" class="icon-button" title="关闭" @click="closeArticle">
              <X :size="16" />
            </button>
          </div>
        </header>

        <div v-if="detailLoading" class="article-reader-state">
          <LoaderCircle :size="16" class="spinning" />
          读取正文
        </div>

        <div v-else-if="detailError" class="source-alert article-reader-error">
          <CircleAlert :size="15" />
          <span>{{ detailError }}</span>
          <button type="button" @click="retryArticle">重试</button>
        </div>

        <template v-else-if="selectedArticle">
          <div class="article-reader-meta">
            <span>{{ sourceLabel(selectedArticle.source) }}</span>
            <span>{{ contentTypeLabel(selectedArticle.content_type) }}</span>
            <span v-if="selectedArticle.category">{{ selectedArticle.category }}</span>
            <span>{{ contentStats }}</span>
          </div>

          <div class="article-reader-feedback">
            <button type="button" :disabled="isFeedbackLoading(selectedArticle.id)" @click="sendArticleFeedback(selectedArticle.id, 'useful')">
              <ThumbsUp :size="13" />
              有用
            </button>
            <button type="button" :disabled="isFeedbackLoading(selectedArticle.id)" @click="sendArticleFeedback(selectedArticle.id, 'not_useful')">
              <ThumbsDown :size="13" />
              没用
            </button>
            <button type="button" :disabled="isFeedbackLoading(selectedArticle.id)" @click="sendArticleFeedback(selectedArticle.id, 'notify_similar')">
              <BellRing :size="13" />
              类似通知
            </button>
            <button type="button" :disabled="isFeedbackLoading(selectedArticle.id)" @click="sendArticleFeedback(selectedArticle.id, 'silent_similar')">
              <Archive :size="13" />
              类似静默
            </button>
            <span v-if="feedbackState[selectedArticle.id]">{{ feedbackState[selectedArticle.id] }}</span>
          </div>

          <div class="article-reader-tabs" role="tablist" aria-label="阅读内容切换">
            <button type="button" :class="{ active: readerMode === 'summary' }" @click="readerMode = 'summary'">
              <MessageSquareText :size="14" />
              摘要
            </button>
            <button type="button" :class="{ active: readerMode === 'content' }" @click="readerMode = 'content'">
              <FileText :size="14" />
              正文
            </button>
          </div>

          <section v-if="readerMode === 'summary'" class="article-reader-summary">
            <header>
              <MessageSquareText :size="14" />
              <strong>摘要</strong>
              <span>{{ summaryStats }}</span>
            </header>
            <div v-if="summaryParagraphs.length" class="article-reader-summary-text">
              <p v-for="(paragraph, index) in summaryParagraphs" :key="index">{{ paragraph }}</p>
            </div>
            <div v-else class="article-reader-empty">
              <Database :size="18" />
              <strong>没有摘要</strong>
              <span>这条内容尚未生成摘要，可以切换到正文查看解析文本。</span>
            </div>
          </section>

          <section v-else class="article-reader-content">
            <header>
              <FileText :size="14" />
              <strong>解析正文</strong>
              <span>{{ articleParagraphs.length }} 段</span>
            </header>
            <div v-if="articleParagraphs.length" class="article-reader-text">
              <p v-for="(paragraph, index) in articleParagraphs" :key="index">{{ paragraph }}</p>
            </div>
            <div v-else class="article-reader-empty">
              <Database :size="18" />
              <strong>没有保存正文</strong>
              <span>这条内容可能只保存了摘要，或抓取时没有得到可读文本。</span>
            </div>
          </section>
        </template>
      </aside>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  Archive,
  BellRing,
  BookOpenText,
  Bot,
  CircleAlert,
  Database,
  ExternalLink,
  FileText,
  Layers,
  LoaderCircle,
  MessageSquareText,
  RefreshCw,
  Search,
  SearchCheck,
  Send,
  Tags,
  ThumbsDown,
  ThumbsUp,
  X,
} from '@lucide/vue'
import { api } from '../api'
import type { Article, FacetRow, FeedbackRating, KnowledgeCitation, KnowledgeFacets, QAResponse, SearchResult } from '../types'

type FacetMode = 'category' | 'tag'

const articles = ref<Article[]>([])
const searchResults = ref<SearchResult[]>([])
const facets = ref<KnowledgeFacets | null>(null)
const loading = ref(false)
const loaded = ref(false)
const error = ref('')
const facetMode = ref<FacetMode>('category')
const activeFacet = ref('')
const query = ref('')
const queryDraft = ref('')
const searchMode = ref('')
const semanticAvailable = ref(false)
const questionDraft = ref('')
const qa = ref<QAResponse | null>(null)
const qaLoading = ref(false)
const qaError = ref('')
const selectedArticle = ref<Article | null>(null)
const selectedArticleID = ref('')
const readerMode = ref<'summary' | 'content'>('content')
const detailLoading = ref(false)
const detailError = ref('')
const feedbackLoading = ref<Record<string, boolean>>({})
const feedbackState = ref<Record<string, string>>({})

const visibleFacets = computed<FacetRow[]>(() => {
  const source = facetMode.value === 'category' ? facets.value?.categories : facets.value?.tags
  return source ?? []
})

const emptyText = computed(() => {
  if (query.value) return '没有匹配当前搜索条件的内容。'
  if (activeFacet.value) return '这个分类或标签下还没有归档内容。'
  return '完成抓取总结后，内容会出现在这里。'
})

const qaModeText = computed(() => {
  if (qa.value?.mode === 'hybrid' || searchMode.value === 'hybrid') return '混合检索'
  if (semanticAvailable.value) return '语义检索可用'
  return '关键词检索'
})

const readerOpen = computed(() => detailLoading.value || !!detailError.value || !!selectedArticle.value)

const articleParagraphs = computed(() => readableParagraphs(selectedArticle.value?.content || ''))

const summaryParagraphs = computed(() => readableParagraphs(selectedArticle.value?.summary || ''))

const contentStats = computed(() => {
  const content = selectedArticle.value?.content || ''
  const chars = Array.from(content.trim()).length
  if (!chars) return '0 字'
  return `${chars} 字`
})

const summaryStats = computed(() => {
  const summary = selectedArticle.value?.summary || ''
  const chars = Array.from(summary.trim()).length
  if (!chars) return '0 字'
  return `${chars} 字`
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    const nextFacets = await api.getKnowledgeFacets()
    facets.value = nextFacets
    if (query.value) {
      const result = await api.searchKnowledge({ q: query.value, limit: 80 })
      searchResults.value = result.results
      searchMode.value = result.mode
      semanticAvailable.value = result.semantic_available
      articles.value = []
    } else {
      articles.value = await api.getArticles(articleParams())
      searchResults.value = []
      searchMode.value = ''
      semanticAvailable.value = false
    }
    loaded.value = true
  } catch {
    error.value = '无法读取知识库，请确认 API 服务可用。'
  } finally {
    loading.value = false
  }
}

function articleParams() {
  return {
    category: facetMode.value === 'category' ? activeFacet.value || undefined : undefined,
    tag: facetMode.value === 'tag' ? activeFacet.value || undefined : undefined,
    limit: 80,
  }
}

function setFacetMode(mode: FacetMode) {
  if (facetMode.value === mode) return
  facetMode.value = mode
  activeFacet.value = ''
  query.value = ''
  queryDraft.value = ''
  load()
}

function selectFacet(name: string) {
  activeFacet.value = name
  query.value = ''
  queryDraft.value = ''
  load()
}

function selectCategory(category: string) {
  facetMode.value = 'category'
  selectFacet(category)
}

function selectTag(tag: string) {
  facetMode.value = 'tag'
  activeFacet.value = tag
  query.value = ''
  queryDraft.value = ''
  load()
}

function applySearch() {
  query.value = queryDraft.value.trim()
  activeFacet.value = ''
  load()
}

async function askQuestion() {
  const question = questionDraft.value.trim()
  if (!question || qaLoading.value) return
  qaLoading.value = true
  qaError.value = ''
  try {
    qa.value = await api.askKnowledge(question)
  } catch {
    qaError.value = '问答失败，请确认 LLM 服务已配置且可用。'
  } finally {
    qaLoading.value = false
  }
}

async function openArticle(id: string, mode: 'summary' | 'content' = 'content') {
  if (!id || detailLoading.value) return
  selectedArticleID.value = id
  readerMode.value = mode
  detailLoading.value = true
  detailError.value = ''
  try {
    selectedArticle.value = await api.getArticle(id)
  } catch {
    selectedArticle.value = null
    detailError.value = '无法读取解析正文，请稍后重试。'
  } finally {
    detailLoading.value = false
  }
}

function retryArticle() {
  if (selectedArticleID.value) openArticle(selectedArticleID.value, readerMode.value)
}

function closeArticle() {
  selectedArticle.value = null
  selectedArticleID.value = ''
  readerMode.value = 'content'
  detailError.value = ''
  detailLoading.value = false
}

function isFeedbackLoading(articleID: string) {
  return !!feedbackLoading.value[articleID]
}

async function sendArticleFeedback(articleID: string, rating: FeedbackRating) {
  if (!articleID || isFeedbackLoading(articleID)) return
  feedbackLoading.value = { ...feedbackLoading.value, [articleID]: true }
  feedbackState.value = { ...feedbackState.value, [articleID]: '' }
  try {
    await api.sendFeedback({
      target_type: 'article',
      target_id: articleID,
      rating,
      source: 'knowledge',
    })
    feedbackState.value = { ...feedbackState.value, [articleID]: feedbackLabel(rating) }
  } catch {
    feedbackState.value = { ...feedbackState.value, [articleID]: '反馈失败' }
  } finally {
    feedbackLoading.value = { ...feedbackLoading.value, [articleID]: false }
  }
}

function feedbackLabel(rating: FeedbackRating) {
  const labels: Record<FeedbackRating, string> = {
    useful: '已记为有用',
    not_useful: '已记为低价值',
    notify_similar: '以后类似内容会更倾向通知',
    silent_similar: '以后类似内容会更倾向静默',
    discard_similar: '以后类似内容会更倾向丢弃',
  }
  return labels[rating]
}

function articleTitle(article: Article) {
  if (article.title) return article.title
  try {
    return new URL(article.url).hostname
  } catch {
    return article.url || article.id
  }
}

function resultTitle(result: SearchResult) {
  if (result.title) return result.title
  try {
    return new URL(result.url).hostname
  } catch {
    return result.url || result.article_id
  }
}

function articlePreview(article: Article) {
  return compactPreview(article.summary) || '暂无摘要，点击查看解析内容'
}

function resultPreview(result: SearchResult) {
  return compactPreview(result.snippet) || compactPreview(result.summary) || '暂无摘要，点击查看解析内容'
}

function citationTitle(citation: KnowledgeCitation) {
  if (citation.title) return citation.title
  try {
    return new URL(citation.url).hostname
  } catch {
    return citation.url || citation.article_id
  }
}

function articleHost(url: string) {
  try {
    return new URL(url).hostname
  } catch {
    return url || '未知来源'
  }
}

function readableParagraphs(content: string) {
  return content
    .replace(/\r\n/g, '\n')
    .split(/\n{2,}|\n/)
    .map(line => line.trim())
    .filter(Boolean)
}

function compactPreview(value: string) {
  return value
    .replace(/\s+/g, ' ')
    .trim()
}

function sourceLabel(source: string) {
  const labels: Record<string, string> = {
    manual: '手动',
    rss: 'RSS',
    bookmark: '收藏夹',
    wechat_mp: '公众号',
    linux_do: 'linux.do',
    email: '邮件',
    chaoxing: '学习通',
  }
  return labels[source] || source || '未知'
}

function contentTypeLabel(contentType: string) {
  const labels: Record<string, string> = {
    webpage: '网页',
    video: '视频',
    email: '邮件',
    post: '帖子',
    message: '消息',
  }
  return labels[contentType] || '内容'
}

function matchLabel(match: string) {
  const labels: Record<string, string> = {
    hybrid: '混合',
    semantic: '语义',
    keyword: '关键词',
  }
  return labels[match] || '匹配'
}

function formatDate(value: string | null) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return new Intl.DateTimeFormat('zh-CN', {
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

onMounted(load)
</script>
