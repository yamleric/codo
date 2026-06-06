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
