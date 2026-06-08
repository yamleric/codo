<template>
  <section class="settings-card memory-card">
    <header>
      <span class="settings-card-icon"><BrainCircuit :size="16" /></span>
      <div>
        <strong>偏好记忆</strong>
        <small>{{ profile ? `${profile.memory_count} 条记忆 · ${profile.feedback_count} 次反馈` : '读取中' }}</small>
      </div>
      <button type="button" class="icon-button" title="刷新记忆" :disabled="loading" @click="load">
        <RefreshCw :size="15" :class="{ spinning: loading }" />
      </button>
    </header>

    <div v-if="error" class="source-alert memory-alert">
      <CircleAlert :size="15" />
      <span>{{ error }}</span>
      <button type="button" @click="load">重试</button>
    </div>

    <div v-if="loading && !state" class="loading-row">
      <LoaderCircle :size="16" class="spinning" />
      读取偏好记忆
    </div>

    <template v-else-if="profile">
      <label class="settings-field settings-toggle memory-toggle">
        <span>参与过滤判断</span>
        <input :checked="profile.memory_enabled" type="checkbox" :disabled="saving" @change="toggleMemory" />
      </label>

      <div class="memory-profile-grid">
        <article v-for="group in profileGroups" :key="group.label">
          <strong>{{ group.label }}</strong>
          <div v-if="group.items.length" class="memory-chip-list">
            <span v-for="item in group.items" :key="item">{{ item }}</span>
          </div>
          <small v-else>暂无</small>
        </article>
      </div>

      <div class="memory-composer">
        <select v-model="draft.memory_type" aria-label="记忆类型">
          <option v-for="option in memoryOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
        </select>
        <textarea v-model.trim="draft.content" rows="3" placeholder="补一条偏好，例如：产品战略、AI 工具落地、学校作业提醒要优先通知" />
        <button type="button" :disabled="saving || !draft.content.trim()" @click="addMemory">
          <Plus :size="15" />
          添加
        </button>
      </div>

      <div class="memory-list">
        <article v-for="memory in memories" :key="memory.id" class="memory-row" :class="{ disabled: !!memory.disabled_at }">
          <template v-if="editing[memory.id]">
            <div class="memory-edit-grid">
              <select v-model="editing[memory.id].memory_type" aria-label="记忆类型">
                <option v-for="option in memoryOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
              </select>
              <label>
                <span>权重</span>
                <input v-model.number="editing[memory.id].confidence" type="number" min="0.1" max="1" step="0.05" />
              </label>
              <label class="memory-inline-toggle">
                <span>停用</span>
                <input v-model="editing[memory.id].disabled" type="checkbox" />
              </label>
            </div>
            <textarea v-model.trim="editing[memory.id].content" rows="3" />
            <div class="memory-actions">
              <button type="button" :disabled="saving || !editing[memory.id].content.trim()" @click="saveMemory(memory.id)">
                <Save :size="14" />
                保存
              </button>
              <button type="button" :disabled="saving" @click="cancelEdit(memory.id)">
                <X :size="14" />
                取消
              </button>
            </div>
          </template>

          <template v-else>
            <header>
              <span>{{ memoryTypeLabel(memory.memory_type) }}</span>
              <small>{{ Math.round(memory.confidence * 100) }}%</small>
            </header>
            <p>{{ memory.content }}</p>
            <footer>
              <span>{{ memory.disabled_at ? '已停用' : memorySourceLabel(memory.source_type) }}</span>
              <div class="memory-actions">
                <button type="button" :disabled="saving" @click="startEdit(memory)">
                  <Pencil :size="14" />
                  编辑
                </button>
                <button type="button" :disabled="saving" @click="deleteMemory(memory.id)">
                  <Trash2 :size="14" />
                  删除
                </button>
              </div>
            </footer>
          </template>
        </article>

        <div v-if="!memories.length" class="keyword-empty">还没有偏好记忆</div>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import {
  BrainCircuit,
  CircleAlert,
  LoaderCircle,
  Pencil,
  Plus,
  RefreshCw,
  Save,
  Trash2,
  X,
} from '@lucide/vue'
import { api } from '../api'
import type { MemoryType, PreferenceMemoryResponse, UserMemory } from '../types'

interface MemoryDraft {
  memory_type: MemoryType
  content: string
  confidence: number
  disabled: boolean
}

const state = ref<PreferenceMemoryResponse | null>(null)
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const draft = reactive<MemoryDraft>({
  memory_type: 'interest',
  content: '',
  confidence: 0.75,
  disabled: false,
})
const editing = ref<Record<string, MemoryDraft>>({})

const memoryOptions = [
  { value: 'interest' as const, label: '感兴趣' },
  { value: 'notify' as const, label: '通知' },
  { value: 'silent' as const, label: '静默' },
  { value: 'reject' as const, label: '降低' },
  { value: 'intent' as const, label: '意图' },
]

const profile = computed(() => state.value?.profile || null)
const memories = computed(() => state.value?.memories || [])

const profileGroups = computed(() => {
  const next = profile.value
  return [
    { label: '近期意图', items: next?.recent_intents || [] },
    { label: '感兴趣', items: next?.interests || [] },
    { label: '优先通知', items: next?.notify_preferences || [] },
    { label: '静默归档', items: next?.archive_preferences || [] },
    { label: '降低优先级', items: next?.reject_patterns || [] },
  ]
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    state.value = await api.getPreferenceMemory()
    editing.value = {}
  } catch {
    error.value = '无法读取偏好记忆。'
  } finally {
    loading.value = false
  }
}

async function toggleMemory(event: Event) {
  const input = event.target as HTMLInputElement
  saving.value = true
  error.value = ''
  try {
    state.value = await api.updatePreferenceMemory({ memory_enabled: input.checked })
  } catch {
    error.value = '保存记忆开关失败。'
    input.checked = !input.checked
  } finally {
    saving.value = false
  }
}

async function addMemory() {
  if (!draft.content.trim() || saving.value) return
  saving.value = true
  error.value = ''
  try {
    await api.addMemory({
      memory_type: draft.memory_type,
      content: draft.content.trim(),
      confidence: draft.confidence,
      disabled: false,
    })
    draft.content = ''
    await load()
  } catch {
    error.value = '添加记忆失败。'
  } finally {
    saving.value = false
  }
}

function startEdit(memory: UserMemory) {
  editing.value = {
    ...editing.value,
    [memory.id]: {
      memory_type: normalizeMemoryType(memory.memory_type),
      content: memory.content,
      confidence: memory.confidence,
      disabled: !!memory.disabled_at,
    },
  }
}

function cancelEdit(id: string) {
  const next = { ...editing.value }
  delete next[id]
  editing.value = next
}

async function saveMemory(id: string) {
  const next = editing.value[id]
  if (!next?.content.trim() || saving.value) return
  saving.value = true
  error.value = ''
  try {
    await api.updateMemory(id, {
      memory_type: next.memory_type,
      content: next.content.trim(),
      confidence: clampConfidence(next.confidence),
      disabled: !!next.disabled,
    })
    await load()
  } catch {
    error.value = '保存记忆失败。'
  } finally {
    saving.value = false
  }
}

async function deleteMemory(id: string) {
  if (saving.value) return
  saving.value = true
  error.value = ''
  try {
    await api.deleteMemory(id)
    await load()
  } catch {
    error.value = '删除记忆失败。'
  } finally {
    saving.value = false
  }
}

function normalizeMemoryType(value: string): MemoryType {
  if (value === 'notify' || value === 'silent' || value === 'reject' || value === 'intent') return value
  return 'interest'
}

function memoryTypeLabel(value: string) {
  return memoryOptions.find(option => option.value === value)?.label || '偏好'
}

function memorySourceLabel(value: string) {
  const labels: Record<string, string> = {
    feedback: '来自反馈',
    manual_intent: '来自提交意图',
  }
  return labels[value] || '手动'
}

function clampConfidence(value: number) {
  const numeric = Number.isFinite(value) ? value : 0.75
  return Math.min(1, Math.max(0.1, numeric))
}

onMounted(load)
</script>
