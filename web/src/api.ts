import axios from 'axios'
import type { Task, Subscription } from './types'

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
}
