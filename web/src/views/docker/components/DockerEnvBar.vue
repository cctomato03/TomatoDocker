<!-- Docker 页面通用环境选择条 -->
<template>
  <div class="docker-env-bar">
    <span class="docker-env-bar__label">Docker 环境</span>
    <ElSelect
      v-model="selectedEnvId"
      class="docker-env-bar__select"
      placeholder="请选择 Docker 环境"
      filterable
      :loading="loading"
      :disabled="!environments.length"
    >
      <ElOption v-for="env in environments" :key="env.id" :label="env.name" :value="env.id">
        <div class="flex items-center justify-between gap-3">
          <span>{{ env.name }}</span>
          <span class="text-xs text-g-500">{{ env.type }}</span>
        </div>
      </ElOption>
    </ElSelect>

    <template v-if="selectedEnv">
      <ElTag :type="envStatusType" size="small" effect="light">
        {{ envStatusText }}
      </ElTag>
      <ElTag v-if="selectedEnv.swarmActive" type="primary" size="small" effect="plain">
        Swarm {{ selectedEnv.swarmRole === 'manager' ? '管理节点' : '工作节点' }}
      </ElTag>
      <ElTag :type="accessTagType" size="small" effect="plain">{{ accessText }}</ElTag>
      <ElTooltip v-if="selectedEnv.statusMessage" :content="selectedEnv.statusMessage">
        <ArtSvgIcon icon="ep:warning-filled" class="text-amber-500" />
      </ElTooltip>
    </template>

    <div class="docker-env-bar__actions">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted } from 'vue'
  import { useDockerEnv } from '@/hooks/core/useDockerEnv'

  const { environments, selectedEnvId, selectedEnv, loading, loadEnvironments } = useDockerEnv()

  onMounted(() => {
    loadEnvironments()
  })

  const envStatusType = computed(() => {
    if (selectedEnv.value?.status === 'online') return 'success'
    if (selectedEnv.value?.status === 'offline') return 'danger'
    return 'info'
  })
  const envStatusText = computed(() => {
    const s = selectedEnv.value?.status
    if (s === 'online') return '在线'
    if (s === 'offline') return '离线'
    return '未知'
  })
  const accessTagType = computed(() => {
    const l = selectedEnv.value?.accessLevel ?? 0
    if (l >= 3) return 'danger'
    if (l >= 2) return 'warning'
    return 'info'
  })
  const accessText = computed(() => {
    const l = selectedEnv.value?.accessLevel ?? 0
    if (l >= 3) return '环境管理'
    if (l >= 2) return '可操作'
    if (l >= 1) return '只读'
    return '无权限'
  })
</script>

<style scoped>
.docker-env-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  margin-bottom: 12px;
  background: var(--art-main-bg-color);
  border: 1px solid var(--art-border-color);
  border-radius: 10px;
}
.docker-env-bar__label {
  font-size: 13px;
  color: var(--art-gray-600);
}
.docker-env-bar__select {
  width: 220px;
}
.docker-env-bar__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-left: auto;
}
</style>