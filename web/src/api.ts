import axios from 'axios'
import type { Article, Bookmark, BookmarkImportResult, KnowledgeFacets, QAResponse, SearchResponse, Subscription, Task, UserSettings, UserSettingsPatch } from './types'

export const api = {
  submitUrl: (url: string) =>
    axios.post<{ id: string }>('/api/tasks', { url }).then(r => r.data),

  getTasks: () =>
    axios.get<Task[]>('/api/tasks').then(r => r.data),

  retry: (id: string) =>
    axios.post(`/api/tasks/${id}/retry`),

  getSubscriptions: () =>
    axios.get<Subscription[]>('/api/subscriptions').then(r => r.data),

  addSubscription: (payload: { feed_url: string; title?: string; category?: string }) =>
    axios.post<{ id: string }>('/api/subscriptions', payload).then(r => r.data),

  updateSubscription: (id: string, payload: Partial<Pick<Subscription, 'feed_url' | 'title' | 'category' | 'enabled'>>) =>
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

  getKnowledgeFacets: () =>
    axios.get<KnowledgeFacets>('/api/knowledge/facets').then(r => r.data),

  searchKnowledge: (params: { q: string; limit?: number }) =>
    axios.get<SearchResponse>('/api/search', { params }).then(r => r.data),

  askKnowledge: (question: string) =>
    axios.post<QAResponse>('/api/qa', { question }).then(r => r.data),

  getSettings: () =>
    axios.get<UserSettings>('/api/settings').then(r => r.data),

  updateSettings: (payload: UserSettingsPatch) =>
    axios.patch<UserSettings>('/api/settings', payload).then(r => r.data),
}
