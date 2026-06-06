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

  addSubscription: (feedUrl: string) =>
    axios.post<{ id: string }>('/api/subscriptions', { feed_url: feedUrl }).then(r => r.data),
}
