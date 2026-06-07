export interface Step {
  label: string
  status: 'ok' | 'error' | 'skipped'
  detail: string
  duration_ms: number
}

export interface Task {
  id: string
  source: string
  content_type: string
  url: string
  status: string
  filter_decision: string
  category: string
  tags: string[]
  summary: string
  error: string
  created_at: string
  steps: Step[]
}

export interface Subscription {
  id: string
  user_id: string
  source_type: string
  feed_url: string
  title: string
  category: string
  last_fetched_at: string | null
  last_error: string
  last_error_at: string | null
  enabled: boolean
  created_at: string
}

export interface Bookmark {
  id: string
  user_id: string
  url: string
  title: string
  folder: string
  note: string
  status: 'pending' | 'syncing' | 'synced' | 'failed' | string
  last_task_id: string
  last_synced_at: string | null
  last_error: string
  created_at: string
  updated_at: string
}

export interface BookmarkImportResult {
  imported: number
  updated: number
  skipped: number
  bookmarks: Bookmark[]
}

export interface Article {
  id: string
  user_id: string
  task_id: string
  url: string
  title: string
  source: string
  content_type: string
  summary: string
  category: string
  tags: string[]
  metadata: Record<string, unknown>
  published_at: string | null
  created_at: string
}

export interface FacetRow {
  name: string
  count: number
}

export interface KnowledgeFacets {
  total: number
  categories: FacetRow[]
  tags: FacetRow[]
  sources: FacetRow[]
}

export interface SearchResult {
  chunk_id: string
  article_id: string
  title: string
  url: string
  source: string
  content_type: string
  summary: string
  category: string
  tags: string[]
  snippet: string
  score: number
  match: 'keyword' | 'semantic' | 'hybrid' | string
  created_at: string
}

export interface SearchResponse {
  query: string
  mode: 'keyword' | 'hybrid' | string
  semantic_available: boolean
  results: SearchResult[]
}

export interface KnowledgeCitation {
  index: number
  article_id: string
  chunk_id: string
  title: string
  url: string
  source: string
  content_type: string
  category: string
  tags: string[]
  snippet: string
}

export interface QAResponse {
  question: string
  answer: string
  mode: string
  citations: KnowledgeCitation[]
}

export interface AuthStatus {
  setup_required: boolean
  authenticated: boolean
  user_id: string
  username: string
}

export type NotifyChannel = 'telegram' | 'email' | 'none'
export type NotifyPolicy = 'pass_only' | 'save_only'
export type SummaryStyle = 'concise' | 'structured' | 'actionable'
export type SummaryLanguage = 'zh-CN' | 'en'

export interface SettingsRuntime {
  llm_configured: boolean
  embedding_configured: boolean
  asr_configured: boolean
  telegram_configured: boolean
  email_configured: boolean
  yt_dlp_configured: boolean
  yt_dlp_cookies_set: boolean
  yt_dlp_browser_cookies_set: boolean
  playwright_configured: boolean
  ffmpeg_configured: boolean
}

export interface ServiceKeyConfig {
  base_url: string
  model: string
  key_configured: boolean
}

export interface TelegramRuntimeConfig {
  chat_id: string
  token_configured: boolean
}

export interface SMTPRuntimeConfig {
  host: string
  port: number
  username: string
  from: string
  use_tls: boolean
  password_configured: boolean
}

export interface RuntimeConfig {
  llm: ServiceKeyConfig
  embedding: ServiceKeyConfig
  asr: ServiceKeyConfig
  telegram: TelegramRuntimeConfig
  smtp: SMTPRuntimeConfig
}

export interface RuntimeConfigPatch {
  llm?: Partial<{ base_url: string; model: string; api_key: string }>
  embedding?: Partial<{ base_url: string; model: string; api_key: string }>
  asr?: Partial<{ base_url: string; model: string; api_key: string }>
  telegram?: Partial<{ token: string; chat_id: string }>
  smtp?: Partial<{ host: string; port: number; username: string; password: string; from: string; use_tls: boolean }>
}

export interface DailyReportSettings {
  enabled: boolean
  email: string
  hour: number
  timezone: string
  max_items: number
}

export interface UserSettings {
  user_id: string
  notify_channel: NotifyChannel
  notify_policy: NotifyPolicy
  summary_style: SummaryStyle
  language: SummaryLanguage
  max_summary_chars: number
  filter_keywords: string[]
  daily_report: DailyReportSettings
  runtime: SettingsRuntime
  runtime_config: RuntimeConfig
}

export type UserSettingsPatch = Partial<Pick<
  UserSettings,
  'notify_channel' | 'notify_policy' | 'summary_style' | 'language' | 'max_summary_chars' | 'filter_keywords' | 'daily_report'
>> & {
  runtime_config?: RuntimeConfigPatch
}
