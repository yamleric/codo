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

export type NotifyChannel = 'telegram' | 'none'
export type NotifyPolicy = 'pass_only' | 'save_only'
export type SummaryStyle = 'concise' | 'structured' | 'actionable'
export type SummaryLanguage = 'zh-CN' | 'en'

export interface SettingsRuntime {
  llm_configured: boolean
  asr_configured: boolean
  telegram_configured: boolean
  yt_dlp_configured: boolean
  ffmpeg_configured: boolean
}

export interface UserSettings {
  user_id: string
  notify_channel: NotifyChannel
  notify_policy: NotifyPolicy
  summary_style: SummaryStyle
  language: SummaryLanguage
  max_summary_chars: number
  filter_keywords: string[]
  runtime: SettingsRuntime
}

export type UserSettingsPatch = Partial<Pick<
  UserSettings,
  'notify_channel' | 'notify_policy' | 'summary_style' | 'language' | 'max_summary_chars' | 'filter_keywords'
>>
