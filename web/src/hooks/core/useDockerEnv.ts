/**
 * Docker 环境选择（模块级单例）：所有 Docker 页面共享同一个当前环境，
 * 选择结果写入 localStorage，刷新页面后保持不变。
 */
import { computed, ref } from 'vue'
import { fetchDockerEnvironments } from '@/api/docker'

const STORAGE_KEY = 'tomato-docker:env-id'

const environments = ref<Docker.Environment[]>([])
const selectedEnvId = ref<string>(localStorage.getItem(STORAGE_KEY) || '')
const loading = ref(false)
const loaded = ref(false)

const selectedEnv = computed(() =>
  environments.value.find((env) => env.id === selectedEnvId.value)
)

const accessLevel = computed(() => selectedEnv.value?.accessLevel ?? 0)

function persist(id: string) {
  selectedEnvId.value = id
  if (id) localStorage.setItem(STORAGE_KEY, id)
  else localStorage.removeItem(STORAGE_KEY)
}

function ensureSelection() {
  if (!environments.value.length) {
    if (selectedEnvId.value) persist('')
    return
  }
  const exists = environments.value.some((env) => env.id === selectedEnvId.value)
  if (exists) return
  const preferred =
    environments.value.find((env) => env.isDefault) ||
    environments.value[0]
  persist(preferred.id)
}

async function loadEnvironments(force = false) {
  if (loading.value) return environments.value
  if (loaded.value && !force) {
    ensureSelection()
    return environments.value
  }
  loading.value = true
  try {
    environments.value = (await fetchDockerEnvironments()) || []
    loaded.value = true
    ensureSelection()
    return environments.value
  } finally {
    loading.value = false
  }
}

/** 登录后权限可能变化，退出/切换账号时调用一次即可。 */
function resetDockerEnvironments() {
  loaded.value = false
  environments.value = []
  selectedEnvId.value = ''
  localStorage.removeItem(STORAGE_KEY)
}

export function useDockerEnv() {
  return {
    environments,
    selectedEnvId,
    selectedEnv,
    accessLevel,
    loading,
    loaded,
    loadEnvironments,
    ensureSelection,
    persist,
    resetDockerEnvironments
  }
}