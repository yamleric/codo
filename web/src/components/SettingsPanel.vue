<template>
  <section class="settings-panel">
    <header class="section-heading">
      <div>
        <span class="section-kicker">CONTROL</span>
        <h2>前台配置项</h2>
      </div>
      <div class="settings-heading-actions">
        <button type="button" class="icon-button" title="重新读取设置" :disabled="loading" @click="load">
          <RefreshCw :size="16" :class="{ spinning: loading }" />
        </button>
      </div>
    </header>

    <div v-if="loadError" class="source-alert">
      <CircleAlert :size="15" />
      <span>{{ loadError }}</span>
      <button type="button" @click="load">重试</button>
    </div>

    <form v-if="loaded && settings" class="settings-layout" @submit.prevent="save">
      <section class="settings-card">
        <header>
          <span class="settings-card-icon"><Bell :size="16" /></span>
          <div>
            <strong>通知</strong>
            <small>{{ settings?.user_id || 'demo-user' }}</small>
          </div>
        </header>

        <label class="settings-field">
          <span>通知通道</span>
          <div class="settings-segmented">
            <button
              v-for="option in notifyChannelOptions"
              :key="option.value"
              type="button"
              :class="{ active: form.notify_channel === option.value }"
              :aria-pressed="form.notify_channel === option.value"
              @click="form.notify_channel = option.value"
            >
              <component :is="option.icon" :size="14" />
              {{ option.label }}
            </button>
          </div>
        </label>

        <label class="settings-field">
          <span>通知策略</span>
          <div class="settings-segmented">
            <button
              v-for="option in notifyPolicyOptions"
              :key="option.value"
              type="button"
              :class="{ active: form.notify_policy === option.value }"
              :aria-pressed="form.notify_policy === option.value"
              @click="form.notify_policy = option.value"
            >
              <component :is="option.icon" :size="14" />
              {{ option.label }}
            </button>
          </div>
        </label>
      </section>

      <section class="settings-card">
        <header>
          <span class="settings-card-icon"><FileText :size="16" /></span>
          <div>
            <strong>摘要</strong>
            <small>{{ form.language === 'zh-CN' ? '中文输出' : 'English output' }}</small>
          </div>
        </header>

        <label class="settings-field">
          <span>摘要风格</span>
          <div class="settings-segmented three">
            <button
              v-for="option in summaryStyleOptions"
              :key="option.value"
              type="button"
              :class="{ active: form.summary_style === option.value }"
              :aria-pressed="form.summary_style === option.value"
              @click="form.summary_style = option.value"
            >
              <component :is="option.icon" :size="14" />
              {{ option.label }}
            </button>
          </div>
        </label>

        <div class="settings-pair">
          <label class="settings-field">
            <span>输出语言</span>
            <div class="settings-segmented">
              <button
                v-for="option in languageOptions"
                :key="option.value"
                type="button"
                :class="{ active: form.language === option.value }"
                :aria-pressed="form.language === option.value"
                @click="form.language = option.value"
              >
                <Languages :size="14" />
                {{ option.label }}
              </button>
            </div>
          </label>

          <label class="settings-field">
            <span>摘要长度</span>
            <input
              v-model.number="form.max_summary_chars"
              type="number"
              min="120"
              max="1000"
              step="20"
            />
          </label>
        </div>
      </section>

      <section class="settings-card daily-report-card">
        <header>
          <span class="settings-card-icon"><Mail :size="16" /></span>
          <div>
            <strong>日报</strong>
            <small>{{ form.daily_report.enabled ? reportScheduleText : '邮箱日报已关闭' }}</small>
          </div>
        </header>

        <label class="settings-field">
          <span>日报推送</span>
          <div class="settings-segmented">
            <button
              type="button"
              :class="{ active: form.daily_report.enabled }"
              :aria-pressed="form.daily_report.enabled"
              @click="form.daily_report.enabled = true"
            >
              <MailCheck :size="14" />
              开启
            </button>
            <button
              type="button"
              :class="{ active: !form.daily_report.enabled }"
              :aria-pressed="!form.daily_report.enabled"
              @click="form.daily_report.enabled = false"
            >
              <MailX :size="14" />
              关闭
            </button>
          </div>
        </label>

        <label class="settings-field">
          <span>收件邮箱</span>
          <input
            v-model.trim="form.daily_report.email"
            type="email"
            placeholder="name@example.com"
          />
        </label>

        <div class="settings-triple">
          <label class="settings-field">
            <span>发送小时</span>
            <input
              v-model.number="form.daily_report.hour"
              type="number"
              min="0"
              max="23"
              step="1"
            />
          </label>
          <label class="settings-field">
            <span>时区</span>
            <input
              v-model.trim="form.daily_report.timezone"
              type="text"
              placeholder="Asia/Shanghai"
            />
          </label>
          <label class="settings-field">
            <span>最大条数</span>
            <input
              v-model.number="form.daily_report.max_items"
              type="number"
              min="1"
              max="80"
              step="1"
            />
          </label>
        </div>
      </section>

      <section class="settings-card keywords-card">
        <header>
          <span class="settings-card-icon"><ListChecks :size="16" /></span>
          <div>
            <strong>过滤关键词</strong>
            <small>{{ form.filter_keywords.length }} 个偏好</small>
          </div>
        </header>

        <div class="keyword-composer">
          <input
            v-model="keywordDraft"
            type="text"
            placeholder="输入关键词，回车添加"
            @keydown.enter.prevent="addKeywords"
          />
          <button type="button" title="添加关键词" :disabled="!keywordDraft.trim()" @click="addKeywords">
            <Plus :size="15" />
          </button>
        </div>

        <div v-if="form.filter_keywords.length" class="keyword-list">
          <span v-for="keyword in form.filter_keywords" :key="keyword" class="keyword-chip">
            {{ keyword }}
            <button type="button" :title="`删除 ${keyword}`" @click="removeKeyword(keyword)">
              <X :size="12" />
            </button>
          </span>
        </div>
        <div v-else class="keyword-empty">未设置关键词</div>
      </section>

      <section class="settings-card runtime-card">
        <header>
          <span class="settings-card-icon"><ShieldCheck :size="16" /></span>
          <div>
            <strong>运行能力</strong>
            <small>密钥值不在前台显示</small>
          </div>
        </header>

        <div class="runtime-grid">
          <article v-for="item in runtimeItems" :key="item.label">
            <component :is="item.icon" :size="16" />
            <div>
              <strong>{{ item.label }}</strong>
              <span>{{ item.detail }}</span>
            </div>
            <span class="runtime-state" :class="{ ok: item.configured }">
              {{ item.configured ? '已配置' : '未配置' }}
            </span>
          </article>
        </div>
      </section>

      <footer class="settings-savebar">
        <span v-if="savedMessage" class="settings-saved"><CheckCircle2 :size="14" />{{ savedMessage }}</span>
        <span v-else>{{ dirty ? '有未保存改动' : '设置已同步' }}</span>
        <div>
          <button type="button" :disabled="saving || !dirty" @click="resetForm">撤销</button>
          <button type="submit" :disabled="saving || !dirty">
            <LoaderCircle v-if="saving" :size="15" class="spinning" />
            <Save v-else :size="15" />
            保存设置
          </button>
        </div>
      </footer>
    </form>

    <div v-else-if="!loaded" class="loading-row"><LoaderCircle :size="16" class="spinning" />读取配置</div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import {
  Bell,
  BellOff,
  CheckCircle2,
  CircleAlert,
  FileText,
  KeyRound,
  Languages,
  ListChecks,
  LoaderCircle,
  Mail,
  MailCheck,
  MailX,
  MessageCircle,
  Mic,
  Plus,
  RefreshCw,
  Save,
  SearchCheck,
  ShieldCheck,
  Terminal,
  X,
} from '@lucide/vue'
import { api } from '../api'
import type {
  NotifyChannel,
  NotifyPolicy,
  SummaryLanguage,
  SummaryStyle,
  UserSettings,
  UserSettingsPatch,
} from '../types'

interface SettingsForm {
  notify_channel: NotifyChannel
  notify_policy: NotifyPolicy
  summary_style: SummaryStyle
  language: SummaryLanguage
  max_summary_chars: number
  filter_keywords: string[]
  daily_report: {
    enabled: boolean
    email: string
    hour: number
    timezone: string
    max_items: number
  }
}

const settings = ref<UserSettings | null>(null)
const loading = ref(false)
const loaded = ref(false)
const saving = ref(false)
const loadError = ref('')
const savedMessage = ref('')
const keywordDraft = ref('')
const original = ref('')

const form = reactive<SettingsForm>({
  notify_channel: 'telegram',
  notify_policy: 'pass_only',
  summary_style: 'concise',
  language: 'zh-CN',
  max_summary_chars: 300,
  filter_keywords: [],
  daily_report: {
    enabled: false,
    email: '',
    hour: 21,
    timezone: 'Asia/Shanghai',
    max_items: 20,
  },
})

const notifyChannelOptions = [
  { value: 'telegram' as const, label: 'Telegram', icon: Bell },
  { value: 'none' as const, label: '不推送', icon: BellOff },
]

const notifyPolicyOptions = [
  { value: 'pass_only' as const, label: '高价值', icon: CheckCircle2 },
  { value: 'save_only' as const, label: '仅归档', icon: BellOff },
]

const summaryStyleOptions = [
  { value: 'concise' as const, label: '简洁', icon: FileText },
  { value: 'structured' as const, label: '结构化', icon: ListChecks },
  { value: 'actionable' as const, label: '行动项', icon: CheckCircle2 },
]

const languageOptions = [
  { value: 'zh-CN' as const, label: '中文' },
  { value: 'en' as const, label: 'English' },
]

const dirty = computed(() => serializeForm() !== original.value)

const reportScheduleText = computed(() => {
  const hour = clampReportHour(form.daily_report.hour).toString().padStart(2, '0')
  return `${form.daily_report.timezone || 'Asia/Shanghai'} ${hour}:00`
})

const runtimeItems = computed(() => {
  const runtime = settings.value?.runtime
  return [
    { label: 'LLM', detail: '过滤与摘要', configured: !!runtime?.llm_configured, icon: KeyRound },
    { label: 'Embedding', detail: '知识检索', configured: !!runtime?.embedding_configured, icon: SearchCheck },
    { label: 'ASR', detail: '视频转写', configured: !!runtime?.asr_configured, icon: Mic },
    { label: 'Telegram', detail: '消息推送', configured: !!runtime?.telegram_configured, icon: MessageCircle },
    { label: 'SMTP', detail: '邮箱日报', configured: !!runtime?.email_configured, icon: Mail },
    { label: '浏览器抓取', detail: '知乎渲染', configured: !!runtime?.playwright_configured, icon: Terminal },
    { label: 'yt-dlp', detail: '视频获取', configured: !!runtime?.yt_dlp_configured, icon: Terminal },
    { label: '视频 Cookies', detail: '抖音授权', configured: !!runtime?.yt_dlp_cookies_set, icon: KeyRound },
    { label: 'ffmpeg', detail: '音频处理', configured: !!runtime?.ffmpeg_configured, icon: Terminal },
  ]
})

async function load() {
  loading.value = true
  loadError.value = ''
  savedMessage.value = ''
  try {
    const next = await api.getSettings()
    hydrate(next)
  } catch {
    loadError.value = '无法读取配置项，请确认 API 服务可用。'
  } finally {
    loaded.value = true
    loading.value = false
  }
}

async function save() {
  if (!dirty.value || saving.value) return
  saving.value = true
  loadError.value = ''
  savedMessage.value = ''
  try {
    form.max_summary_chars = clampSummaryChars(form.max_summary_chars)
    form.daily_report = normalizeDailyReport(form.daily_report)
    form.filter_keywords = normalizeKeywords(form.filter_keywords)
    const payload: UserSettingsPatch = {
      notify_channel: form.notify_channel,
      notify_policy: form.notify_policy,
      summary_style: form.summary_style,
      language: form.language,
      max_summary_chars: form.max_summary_chars,
      filter_keywords: form.filter_keywords,
      daily_report: form.daily_report,
    }
    const next = await api.updateSettings(payload)
    hydrate(next)
    savedMessage.value = '已保存'
    window.setTimeout(() => {
      savedMessage.value = ''
    }, 1800)
  } catch {
    loadError.value = '保存失败，请检查配置项后重试。'
  } finally {
    saving.value = false
  }
}

function hydrate(next: UserSettings) {
  settings.value = next
  form.notify_channel = next.notify_channel
  form.notify_policy = next.notify_policy
  form.summary_style = next.summary_style
  form.language = next.language
  form.max_summary_chars = next.max_summary_chars
  form.filter_keywords = [...next.filter_keywords]
  form.daily_report = normalizeDailyReport(next.daily_report || {
    enabled: false,
    email: '',
    hour: 21,
    timezone: 'Asia/Shanghai',
    max_items: 20,
  })
  original.value = serializeForm()
}

function resetForm() {
  if (settings.value) hydrate(settings.value)
  keywordDraft.value = ''
  savedMessage.value = ''
}

function addKeywords() {
  const parts = keywordDraft.value
    .split(/[,，\n]/)
    .map(item => item.trim())
    .filter(Boolean)
  if (!parts.length) return
  form.filter_keywords = normalizeKeywords([...form.filter_keywords, ...parts])
  keywordDraft.value = ''
  savedMessage.value = ''
}

function removeKeyword(keyword: string) {
  form.filter_keywords = form.filter_keywords.filter(item => item !== keyword)
  savedMessage.value = ''
}

function serializeForm() {
  return JSON.stringify({
    notify_channel: form.notify_channel,
    notify_policy: form.notify_policy,
    summary_style: form.summary_style,
    language: form.language,
    max_summary_chars: Number.isFinite(form.max_summary_chars) ? Math.round(form.max_summary_chars) : 300,
    filter_keywords: normalizeKeywords(form.filter_keywords),
    daily_report: normalizeDailyReport(form.daily_report),
  })
}

function normalizeKeywords(values: string[]) {
  const out: string[] = []
  const seen = new Set<string>()
  for (const raw of values) {
    const keyword = Array.from(raw.trim()).slice(0, 40).join('')
    const key = keyword.toLowerCase()
    if (!keyword || seen.has(key)) continue
    seen.add(key)
    out.push(keyword)
    if (out.length >= 32) break
  }
  return out
}

function clampSummaryChars(value: number) {
  const numeric = Number.isFinite(value) ? value : 300
  return Math.min(1000, Math.max(120, Math.round(numeric)))
}

function normalizeDailyReport(report: SettingsForm['daily_report']) {
  return {
    enabled: !!report.enabled,
    email: (report.email || '').trim(),
    hour: clampReportHour(report.hour),
    timezone: (report.timezone || 'Asia/Shanghai').trim(),
    max_items: clampReportMaxItems(report.max_items),
  }
}

function clampReportHour(value: number) {
  const numeric = Number.isFinite(value) ? value : 21
  return Math.min(23, Math.max(0, Math.round(numeric)))
}

function clampReportMaxItems(value: number) {
  const numeric = Number.isFinite(value) ? value : 20
  return Math.min(80, Math.max(1, Math.round(numeric)))
}

onMounted(load)
</script>
