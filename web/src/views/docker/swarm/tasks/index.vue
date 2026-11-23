<!-- Swarm 任务运行状态 -->
<template>
  <div class="docker-page art-full-height">
    <DockerEnvBar>
      <ElInput v-model="keyword" placeholder="搜索服务 / 节点 / 任务" clearable class="!w-56" />
      <ElSelect v-model="stateFilter" placeholder="任务状态" clearable class="!w-36">
        <ElOption label="running" value="running" />
        <ElOption label="failed" value="failed" />
        <ElOption label="shutdown" value="shutdown" />
        <ElOption label="pending" value="pending" />
        <ElOption label="complete" value="complete" />
        <ElOption label="rejected" value="rejected" />
      </ElSelect>
      <ElButton :loading="loading" @click="load">
        <ArtSvgIcon icon="ep:refresh" class="mr-1" />刷新
      </ElButton>
    </DockerEnvBar>

    <ElAlert v-if="!state?.active" class="mb-3" type="warning" :closable="false" show-icon>
      当前环境未启用 Swarm。
    </ElAlert>

    <ElCard class="art-table-card">
      <ElTable v-loading="loading" :data="filtered" row-key="id" empty-text="没有任务">
        <ElTableColumn label="任务" min-width="200">
          <template #default="{ row }">
            <span class="font-medium">{{ row.name || row.id.slice(0, 12) }}</span>
            <div class="mt-1 text-xs text-g-500"><code>{{ row.id.slice(0, 12) }}</code> · slot {{ row.slot }}</div>
          </template>
        </ElTableColumn>
        <ElTableColumn prop="serviceName" label="服务" min-width="170" show-overflow-tooltip />
        <ElTableColumn prop="nodeName" label="节点" min-width="140" />
        <ElTableColumn label="状态" width="120">
          <template #default="{ row }">
            <ElTag size="small" :type="taskType(row.state)">{{ row.state }}</ElTag>
          </template>
        </ElTableColumn>
        <ElTableColumn prop="desiredState" label="期望状态" width="110" />
        <ElTableColumn label="消息" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.err" class="text-red-500">{{ row.err }}</span>
            <span v-else>{{ row.message || '-' }}</span>
          </template>
        </ElTableColumn>
        <ElTableColumn label="创建时间" width="170">
          <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
        </ElTableColumn>
      </ElTable>
    </ElCard>
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted, ref, watch } from 'vue'
  import { fetchDockerSwarm, fetchDockerTasks } from '@/api/docker'
  import { useDockerEnv } from '@/hooks/core/useDockerEnv'
  import { formatTime } from '@/utils/web-basic'
  import DockerEnvBar from '../../components/DockerEnvBar.vue'

  defineOptions({ name: 'DockerSwarmTasks' })

  const { selectedEnvId } = useDockerEnv()
  const tasks = ref<Docker.Task[]>([])
  const state = ref<Docker.SwarmState>()
  const loading = ref(false)
  const keyword = ref('')
  const stateFilter = ref('')

  const filtered = computed(() => {
    const kw = keyword.value.trim().toLowerCase()
    return tasks.value.filter((t) => {
      if (stateFilter.value && t.state !== stateFilter.value) return false
      if (!kw) return true
      return [t.name, t.serviceName, t.nodeName, t.id].some((v) => String(v || '').toLowerCase().includes(kw))
    })
  })

  onMounted(() => load())
  watch(selectedEnvId, load)

  async function load() {
    if (!selectedEnvId.value) {
      tasks.value = []
      return
    }
    loading.value = true
    try {
      try {
        state.value = await fetchDockerSwarm(selectedEnvId.value)
      } catch {
        state.value = undefined
      }
      tasks.value = state.value?.active ? await fetchDockerTasks(selectedEnvId.value) : []
    } finally {
      loading.value = false
    }
  }

  function taskType(s: string) {
    if (s === 'running') return 'success'
    if (s === 'failed' || s === 'rejected') return 'danger'
    if (s === 'pending' || s === 'preparing') return 'warning'
    return 'info'
  }
</script>