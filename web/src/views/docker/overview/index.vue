<!-- Docker 概览：主机信息、资源计数、磁盘占用与 Stack 运行状态 -->
<template>
  <div class="docker-page art-full-height">
    <DockerEnvBar>
      <ElButton :loading="loading" @click="load">
        <ArtSvgIcon icon="ep:refresh" class="mr-1" />刷新
      </ElButton>
    </DockerEnvBar>

    <ElEmpty v-if="!selectedEnvId" description="暂无可用 Docker 环境，请先在「环境管理」中创建或申请资源授权" />

    <template v-else>
      <ElRow :gutter="12">
        <ElCol v-for="card in cards" :key="card.label" :xs="12" :sm="12" :md="6">
          <ElCard class="mb-3 docker-stat-card">
            <div class="flex items-start justify-between">
              <div>
                <p class="text-sm text-g-500">{{ card.label }}</p>
                <p class="mt-1 text-2xl font-semibold">{{ card.value }}</p>
                <p v-if="card.hint" class="mt-1 text-xs text-g-500">{{ card.hint }}</p>
              </div>
              <ArtSvgIcon :icon="card.icon" class="text-3xl text-theme opacity-70" />
            </div>
          </ElCard>
        </ElCol>
      </ElRow>

      <ElRow :gutter="12">
        <ElCol :xs="24" :md="12">
          <ElCard class="mb-3">
            <template #header>
              <div class="flex-cb">
                <span>主机信息</span>
                <ElTag :type="info?.swarmActive ? 'primary' : 'info'" size="small">
                  {{ info?.swarmActive ? `Swarm ${info?.swarmRole === 'manager' ? 'Manager' : 'Worker'}` : '单机 Docker' }}
                </ElTag>
              </div>
            </template>
            <ElDescriptions :column="2" border size="small">
              <ElDescriptionsItem label="主机名">{{ info?.name || '-' }}</ElDescriptionsItem>
              <ElDescriptionsItem label="Docker 版本">{{ info?.serverVersion || '-' }}</ElDescriptionsItem>
              <ElDescriptionsItem label="操作系统">{{ info?.operatingSystem || '-' }}</ElDescriptionsItem>
              <ElDescriptionsItem label="架构">{{ info?.osType }} / {{ info?.architecture }}</ElDescriptionsItem>
              <ElDescriptionsItem label="内核">{{ info?.kernelVersion || '-' }}</ElDescriptionsItem>
              <ElDescriptionsItem label="存储驱动">{{ info?.driver || '-' }}</ElDescriptionsItem>
              <ElDescriptionsItem label="CPU">{{ info?.nCpu ?? '-' }} 核</ElDescriptionsItem>
              <ElDescriptionsItem label="内存">{{ formatSize(info?.memTotal ?? 0) }}</ElDescriptionsItem>
              <ElDescriptionsItem label="Docker Root">{{ info?.dockerRootDir || '-' }}</ElDescriptionsItem>
              <ElDescriptionsItem label="日志驱动">{{ info?.loggingDriver || '-' }}</ElDescriptionsItem>
            </ElDescriptions>
          </ElCard>
        </ElCol>
        <ElCol :xs="24" :md="12">
          <ElCard class="mb-3">
            <template #header>
              <span>磁盘与资源</span>
            </template>
            <div class="space-y-3">
              <div>
                <div class="flex-cb mb-1 text-sm">
                  <span>镜像占用</span>
                  <span>{{ formatSize(usage?.imagesSize ?? 0) }}</span>
                </div>
                <ElProgress :percentage="percent(usage?.imagesSize, usage?.layersSize)" :show-text="false" />
              </div>
              <div>
                <div class="flex-cb mb-1 text-sm">
                  <span>构建缓存</span>
                  <span>{{ formatSize(usage?.buildCache ?? 0) }}</span>
                </div>
                <ElProgress :percentage="percent(usage?.buildCache, usage?.layersSize)" :show-text="false" color="#e6a23c" />
              </div>
              <ElDescriptions :column="1" border size="small">
                <ElDescriptionsItem label="镜像层总大小">{{ formatSize(usage?.layersSize ?? 0) }}</ElDescriptionsItem>
                <ElDescriptionsItem label="网络 / 数据卷">{{ overview?.networks ?? 0 }} / {{ usage?.volumes ?? 0 }}</ElDescriptionsItem>
                <ElDescriptionsItem label="Swarm 节点 / 服务">
                  {{ overview?.nodes ?? 0 }} / {{ overview?.services ?? 0 }}
                </ElDescriptionsItem>
              </ElDescriptions>
            </div>
          </ElCard>
        </ElCol>
      </ElRow>

      <ElCard>
        <template #header>
          <div class="flex-cb">
            <span>Stack 概览</span>
            <ElButton text type="primary" @click="goStacks">查看全部</ElButton>
          </div>
        </template>
        <ElTable :data="overview?.stackList ?? []" size="small" empty-text="当前环境还没有 Stack">
          <ElTableColumn prop="name" label="名称" min-width="150" show-overflow-tooltip />
          <ElTableColumn prop="type" label="类型" width="90">
            <template #default="{ row }">
              <ElTag size="small" :type="row.type === 'swarm' ? 'primary' : 'success'">
                {{ row.type === 'swarm' ? 'Swarm' : 'Compose' }}
              </ElTag>
            </template>
          </ElTableColumn>
          <ElTableColumn prop="status" label="状态" width="100">
            <template #default="{ row }">
              <ElTag size="small" :type="statusType(row.status)">{{ statusText(row.status) }}</ElTag>
            </template>
          </ElTableColumn>
          <ElTableColumn label="运行态" min-width="160">
            <template #default="{ row }">
              {{ runtimeText(row) }}
            </template>
          </ElTableColumn>
          <ElTableColumn prop="updatedAt" label="更新时间" width="170">
            <template #default="{ row }">{{ formatTime(row.updatedAt) }}</template>
          </ElTableColumn>
        </ElTable>
      </ElCard>
    </template>
  </div>
</template>

<script setup lang="ts">
  import { computed, ref, watch } from 'vue'
  import { useRouter } from 'vue-router'
  import { ElMessage } from 'element-plus'
  import { fetchDockerOverview } from '@/api/docker'
  import { useDockerEnv } from '@/hooks/core/useDockerEnv'
  import { formatSize, formatTime } from '@/utils/web-basic'
  import DockerEnvBar from '../components/DockerEnvBar.vue'

  defineOptions({ name: 'DockerOverview' })

  const router = useRouter()
  const { selectedEnvId, selectedEnv } = useDockerEnv()

  const loading = ref(false)
  const overview = ref<Docker.Overview>()
  const info = computed(() => overview.value?.info)
  const usage = computed(() => overview.value?.usage)

  const cards = computed(() => [
    {
      label: '容器',
      value: `${info.value?.running ?? 0} / ${info.value?.containers ?? 0}`,
      hint: '运行中 / 全部',
      icon: 'ep:box'
    },
    { label: '镜像', value: info.value?.images ?? 0, hint: '本地镜像数', icon: 'ep:files' },
    {
      label: 'Stack',
      value: overview.value?.stacks ?? overview.value?.stackList?.length ?? 0,
      hint: '当前环境',
      icon: 'ep:document-copy'
    },
    {
      label: 'Swarm',
      value: info.value?.swarmActive ? `${info.value.swarmNodes || 1} 节点` : '未启用',
      hint: info.value?.swarmActive ? `${overview.value?.services ?? 0} 个服务` : '单机模式',
      icon: 'ep:connection'
    }
  ])

  async function load() {
    if (!selectedEnvId.value) {
      overview.value = undefined
      return
    }
    loading.value = true
    try {
      overview.value = await fetchDockerOverview(selectedEnvId.value)
    } catch (error) {
      overview.value = undefined
      ElMessage.error(error instanceof Error ? error.message : '加载概览失败')
    } finally {
      loading.value = false
    }
  }

  watch(selectedEnvId, load, { immediate: true })

  function percent(value?: number, total?: number) {
    if (!value || !total) return 0
    return Math.min(100, Math.round((value / total) * 100))
  }
  function statusType(status: string) {
    if (status === 'active') return 'success'
    if (status === 'error') return 'danger'
    if (status === 'deploying') return 'warning'
    return 'info'
  }
  function statusText(status: string) {
    return { active: '运行中', inactive: '已停止', error: '部署失败', deploying: '部署中' }[status] || status
  }
  function runtimeText(row: Docker.Stack) {
    if (row.type === 'swarm') return `${row.runtime?.servicesRunning ?? 0}/${row.runtime?.servicesTotal ?? 0} 服务运行`
    return `${row.runtime?.containersRunning ?? 0}/${row.runtime?.containersTotal ?? 0} 容器运行`
  }
  function goStacks() {
    router.push('/docker/stacks')
  }
</script>

<style scoped>
.docker-stat-card :deep(.el-card__body) {
  padding: 16px;
}
</style>