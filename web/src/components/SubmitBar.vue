<template>
  <section class="submit-panel">
    <div class="submit-copy">
      <span class="submit-icon"><Link2 :size="18" /></span>
      <div>
        <strong>提交新链接</strong>
        <span>网页、公众号、B站或抖音链接</span>
      </div>
    </div>
    <form class="submit-form" @submit.prevent="submit">
      <input
        v-model="url"
        type="text"
        inputmode="url"
        autocomplete="url"
        placeholder="粘贴网页、B站或抖音分享链接"
        aria-label="要收藏的链接"
        :disabled="loading"
        @input="error = ''"
      />
      <button type="submit" :disabled="loading || !url.trim()">
        <LoaderCircle v-if="loading" :size="17" class="spinning" />
        <ArrowUpRight v-else :size="17" />
        <span>{{ loading ? '提交中' : '开始处理' }}</span>
      </button>
    </form>
    <p v-if="error" class="field-error"><CircleAlert :size="14" />{{ error }}</p>
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ArrowUpRight, CircleAlert, Link2, LoaderCircle } from '@lucide/vue'
import { api } from '../api'

const emit = defineEmits<{ submitted: [id: string] }>()
const url = ref('')
const loading = ref(false)
const error = ref('')

async function submit() {
  if (!url.value.trim() || loading.value) return
  const match = url.value.trim().match(/https?:\/\/[^\s<>"']+/)
  if (!match) {
    error.value = '请输入或粘贴包含 http / https 的链接。'
    return
  }
  const normalized = match[0].replace(/[.,;:!?，。；：！？)\]}）】》>]+$/, '')
  try {
    const parsed = new URL(normalized)
    if (!['http:', 'https:'].includes(parsed.protocol)) throw new Error()
  } catch {
    error.value = '请输入有效的 http 或 https 链接。'
    return
  }

  loading.value = true
  error.value = ''
  try {
    const { id } = await api.submitUrl(url.value.trim())
    emit('submitted', id)
    url.value = ''
  } catch {
    error.value = '提交失败，请确认 Agent API 已启动。'
  } finally {
    loading.value = false
  }
}
</script>
