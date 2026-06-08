import axios from 'axios'
import type { Article, AuthStatus, Bookmark, BookmarkImportResult, FeedbackRating, KnowledgeFacets, MemoryType, PreferenceMemoryResponse, QAResponse, SearchResponse, SourceItem, Subscription, Task, UserMemory, UserSettings, UserSettingsPatch } from './types'

axios.defaults.withCredentials = true

export const api = {
  authStatus: () =>
    axios.get<AuthStatus>('/api/auth/status').then(r => r.data),

  setupOwner: (payload: { username: string; password: string }) =>
    axios.post<AuthStatus>('/api/auth/setup', payload).then(r => r.data),

  login: (payload: { username: string; password: string }) =>
    axios.post<AuthStatus>('/api/auth/login', payload).then(r => r.data),

  logout: () =>
    axios.post('/api/auth/logout'),

  submitUrl: (url: string, intent?: string) =>
    axios.post<{ id: string }>('/api/tasks', { url, intent: intent || '' }).then(r => r.data),

  getTasks: () =>
    axios.get<Task[]>('/api/tasks').then(r => r.data),

  retry: (id: string) =>
    axios.post(`/api/tasks/${id}/retry`),

  getSubscriptions: () =>
    axios.get<Subscription[]>('/api/subscriptions').then(r => r.data),

  addSubscription: (payload: {
    source_type?: 'rss' | 'chaoxing' | 'email'
    feed_url?: string
    title?: string
    category?: string
    account?: string
    password?: string
    cookie?: string
    alert_hours?: number
    notify_new?: boolean
    notify_due?: boolean
    provider?: string
    host?: string
    port?: number
    mailbox?: string
    since_days?: number
    max_messages?: number
    notify_important?: boolean
    sync_unread_only?: boolean
  }) =>
    axios.post<{ id: string }>('/api/subscriptions', payload).then(r => r.data),

  updateSubscription: (id: string, payload: Partial<Pick<Subscription, 'feed_url' | 'title' | 'category' | 'enabled' | 'account' | 'alert_hours' | 'notify_new' | 'notify_due' | 'provider' | 'host' | 'port' | 'mailbox' | 'since_days' | 'max_messages' | 'notify_important' | 'sync_unread_only'>> & { password?: string; cookie?: string }) =>
    axios.patch(`/api/subscriptions/${id}`, payload),

  deleteSubscription: (id: string) =>
    axios.delete(`/api/subscriptions/${id}`),

  refreshSubscription: (id: string) =>
    axios.post<{ items: number }>(`/api/subscriptions/${id}/refresh`).then(r => r.data),

  getBookmarks: () =>
    axios.get<Bookmark[]>('/api/bookmarks').then(r => r.data),

  importBookmarks: (payload: { url?: string; text?: string; folder?: string; bookmarks?: Array<{ url: string; title?: string; folder?: string; note?: string }> }) =>
    axios.post<BookmarkImportResult>('/api/bookmarks', payload).then(r => r.data),

  updateBookmark: (id: string, payload: Partial<Pick<Bookmark, 'title' | 'folder' | 'note'>>) =>
    axios.patch(`/api/bookmarks/${id}`, payload),

  deleteBookmark: (id: string) =>
    axios.delete(`/api/bookmarks/${id}`),

  syncBookmarks: (ids?: string[]) =>
    axios.post<{ queued: number; task_ids: string[] }>('/api/bookmarks/sync', { ids: ids ?? [] }).then(r => r.data),

  getArticles: (params?: { category?: string; tag?: string; q?: string; limit?: number }) =>
    axios.get<Article[]>('/api/articles', { params }).then(r => r.data),

  getArticle: (id: string) =>
    axios.get<Article>(`/api/articles/${encodeURIComponent(id)}`).then(r => r.data),

  getSourceItems: (params?: { source_type?: string; limit?: number; current?: boolean }) =>
    axios.get<SourceItem[]>('/api/source-items', { params }).then(r => r.data),

  getKnowledgeFacets: () =>
    axios.get<KnowledgeFacets>('/api/knowledge/facets').then(r => r.data),

  searchKnowledge: (params: { q: string; limit?: number }) =>
    axios.get<SearchResponse>('/api/search', { params }).then(r => r.data),

  askKnowledge: (question: string) =>
    axios.post<QAResponse>('/api/qa', { question }).then(r => r.data),

  sendFeedback: (payload: { target_type: string; target_id: string; rating: FeedbackRating; intent?: string; comment?: string; source?: string }) =>
    axios.post('/api/feedback', payload).then(r => r.data),

  getPreferenceMemory: () =>
    axios.get<PreferenceMemoryResponse>('/api/preference-memory').then(r => r.data),

  updatePreferenceMemory: (payload: { memory_enabled?: boolean }) =>
    axios.patch<PreferenceMemoryResponse>('/api/preference-memory', payload).then(r => r.data),

  addMemory: (payload: { memory_type: MemoryType; content: string; confidence?: number; disabled?: boolean }) =>
    axios.post<UserMemory>('/api/preference-memory/memories', payload).then(r => r.data),

  updateMemory: (id: string, payload: { memory_type: MemoryType | string; content: string; confidence: number; disabled?: boolean }) =>
    axios.patch<UserMemory>(`/api/preference-memory/memories/${encodeURIComponent(id)}`, payload).then(r => r.data),

  deleteMemory: (id: string) =>
    axios.delete(`/api/preference-memory/memories/${encodeURIComponent(id)}`),

  getSettings: () =>
    axios.get<UserSettings>('/api/settings').then(r => r.data),

  updateSettings: (payload: UserSettingsPatch) =>
    axios.patch<UserSettings>('/api/settings', payload).then(r => r.data),
}
