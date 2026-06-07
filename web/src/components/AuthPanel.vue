<template>
  <main class="auth-shell">
    <section class="auth-panel">
      <div class="auth-brand">
        <span class="brand-mark"><Workflow :size="20" :stroke-width="2.2" /></span>
        <div>
          <strong>Codo</strong>
          <span>{{ setupRequired ? '首次设置' : '登录工作台' }}</span>
        </div>
      </div>

      <form class="auth-form" @submit.prevent="submit">
        <header>
          <span class="section-kicker">{{ setupRequired ? 'OWNER SETUP' : 'SESSION' }}</span>
          <h1>{{ setupRequired ? '创建单人工作台账号' : '进入工作台' }}</h1>
          <p>{{ setupRequired ? '这个实例只维护一个 owner 账号。' : '使用已设置的账号继续访问。' }}</p>
        </header>

        <label>
          <span>用户名</span>
          <input v-model.trim="username" type="text" autocomplete="username" placeholder="owner" />
        </label>

        <label>
          <span>密码</span>
          <input
            v-model="password"
            type="password"
            :autocomplete="setupRequired ? 'new-password' : 'current-password'"
            placeholder="至少 8 位"
          />
        </label>

        <div v-if="error" class="auth-error">
          <CircleAlert :size="15" />
          <span>{{ error }}</span>
        </div>

        <button type="submit" :disabled="loading || !canSubmit">
          <LoaderCircle v-if="loading" :size="16" class="spinning" />
          <LogIn v-else :size="16" />
          {{ setupRequired ? '完成设置' : '登录' }}
        </button>
      </form>
    </section>
  </main>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { CircleAlert, LoaderCircle, LogIn, Workflow } from '@lucide/vue'
import { api } from '../api'
import type { AuthStatus } from '../types'

const props = defineProps<{
  setupRequired: boolean
}>()

const emit = defineEmits<{
  authenticated: [AuthStatus]
}>()

const username = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')

const canSubmit = computed(() => username.value.trim() !== '' && password.value.length >= 8)

async function submit() {
  if (!canSubmit.value || loading.value) return
  loading.value = true
  error.value = ''
  try {
    const payload = { username: username.value.trim(), password: password.value }
    const status = props.setupRequired ? await api.setupOwner(payload) : await api.login(payload)
    emit('authenticated', status)
  } catch {
    error.value = props.setupRequired ? '设置失败，请换一个用户名或检查密码。' : '用户名或密码不正确。'
  } finally {
    loading.value = false
  }
}
</script>
