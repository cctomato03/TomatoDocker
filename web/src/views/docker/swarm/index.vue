<!-- Swarm 集群状态：初始化 / 退出 / 集群信息 -->
<template>
  <div class="docker-page art-full-height">
    <DockerEnvBar>
      <ElButton :loading="loading" @click="load">
        <ArtSvgIcon icon="ep:refresh" class="mr-1" />刷新
      </ElButton>
      <ElButton v-if="state?.active" v-auth="'docker:swarm:leave'" type="danger" @click="leaveVisible = true">
        退出 Swarm
      </ElButton>
      <ElButton v-if="!state?.active" v-auth="'docker:swarm:init'" type="primary" @click="initVisible = true">
        初始化 Swarm
      </ElButton>
    </DockerEnvBar>

    <ElEmpty v-if="!selectedEnvId" description="请先选择一个 Docker 环境" />

    <template v-else>
      <ElCard v-if="state?.active" class="mb-3">
        <template #header>
          <div class="flex-cb">
            <span>集群信息</span>
            <ElTag :type="state.controlAvailable ? 'primary' : 'info'">
              {{ state.controlAvailable ? '管理节点' : '工作节点' }}
            </ElTag>
          </div>
        </template>
        <ElDescriptions :column="3" border>
          <ElDescriptionsItem label="集群名称">{{ state.name || 'default' }}</ElDescriptionsItem>
          <ElDescriptionsItem label="Cluster ID"><code>{{ state.clusterId }}</code></ElDescriptionsItem>
          <ElDescriptionsItem label="本地状态">{{ state.localNodeState }}</ElDescriptionsItem>
          <ElDescriptionsItem label="当前节点">{{ state.nodeId }}</ElDescriptionsItem>
          <ElDescriptionsItem label="节点地址">{{ state.nodeAddr }}</ElDescriptionsItem>
          <ElDescriptionsItem label="管理节点数">{{ state.managers }}</ElDescriptionsItem>
          <ElDescriptionsItem label="节点总数">{{ state.nodes }}</ElDescriptionsItem>
          <ElDescriptionsItem label="创建时间">{{ formatTime(state.createdAt) }}</ElDescriptionsItem>
          <ElDescriptionsItem label="更新时间">{{ formatTime(state.updatedAt) }}</ElDescriptionsItem>
        </ElDescriptions>

        <ElAlert v-if="!state.controlAvailable" class="mt-3" type="warning" :closable="false" show-icon>
          当前为工作节点，无法部署服务或 Stack；请在管理节点上操作。
        </ElAlert>
      </ElCard>

      <ElCard v-else>
        <template #header><span>未启用 Swarm</span></template>
        <ElResult
          icon="info"
          title="当前 Docker 环境是单机模式"
          sub-title="初始化后可以使用 Swarm 服务、节点、密钥、配置与 Stack 部署；单机容器仍可继续使用。"
        >
          <template #extra>
            <ElButton v-auth="'docker:swarm:init'" type="primary" @click="initVisible = true">初始化 Swarm</ElButton>
          </template>
        </ElResult>
      </ElCard>

      <ElRow v-if="state?.active" :gutter="12">
        <ElCol :xs="24" :md="8">
          <ElCard class="mb-3">
            <div class="flex-cb"><span class="text-sm text-g-500">节点管理</span><ArtSvgIcon icon="ep:cpu" /></div>
            <p class="mt-2 text-2xl font-semibold">{{ state.nodes }} 个节点</p>
            <ElButton class="mt-2" text type="primary" @click="go('/docker/swarm/nodes')">查看节点</ElButton>
          </ElCard>
        </ElCol>
        <ElCol :xs="24" :md="8">
          <ElCard class="mb-3">
            <div class="flex-cb"><span class="text-sm text-g-500">服务</span><ArtSvgIcon icon="ep:management" /></div>
            <p class="mt-2 text-2xl font-semibold">服务编排</p>
            <ElButton class="mt-2" text type="primary" @click="go('/docker/swarm/services')">查看服务</ElButton>
          </ElCard>
        </ElCol>
        <ElCol :xs="24" :md="8">
          <ElCard class="mb-3">
            <div class="flex-cb"><span class="text-sm text-g-500">Stack</span><ArtSvgIcon icon="ep:files" /></div>
            <p class="mt-2 text-2xl font-semibold">Swarm Stack</p>
            <ElButton class="mt-2" text type="primary" @click="go('/docker/stacks')">前往堆栈</ElButton>
          </ElCard>
        </ElCol>
      </ElRow>
    </template>

    <ElDialog v-model="initVisible" title="初始化 Swarm 集群" width="540px" align-center>
      <ElForm label-width="110px">
        <ElFormItem label="广播地址">
          <ElInput v-model.trim="advertiseAddr" placeholder="留空自动选择本机地址，如 192.168.1.10" />
        </ElFormItem>
        <ElFormItem label="监听地址">
          <ElInput v-model.trim="listenAddr" placeholder="0.0.0.0:2377" />
        </ElFormItem>
      </ElForm>
      <ElAlert type="warning" :closable="false" show-icon>
        初始化会把当前 Docker 切换为 Swarm 管理节点；单机容器不受影响。
      </ElAlert>
      <template #footer>
        <ElButton @click="initVisible = false">取消</ElButton>
        <ElButton type="primary" :loading="submitting" @click="initSwarm">初始化</ElButton>
      </template>
    </ElDialog>

    <ElDialog v-model="leaveVisible" title="退出 Swarm" width="520px" align-center>
      <ElAlert type="warning" :closable="false" show-icon class="mb-3">
        退出会停止当前节点上的 Swarm 服务；管理节点退出可能导致集群不可管理。请确认其他节点已妥善处理。
      </ElAlert>
      <ElCheckbox v-model="forceLeave">强制退出（force）</ElCheckbox>
      <template #footer>
        <ElButton @click="leaveVisible = false">取消</ElButton>
        <ElButton type="danger" :loading="submitting" @click="leaveSwarm">退出</ElButton>
      </template>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, ref, watch } from 'vue'
  import { useRouter } from 'vue-router'
  import { fetchDockerSwarm, initDockerSwarm, leaveDockerSwarm } from '@/api/docker'
  import { useDockerEnv } from '@/hooks/core/useDockerEnv'
  import { formatTime } from '@/utils/web-basic'
  import DockerEnvBar from '../components/DockerEnvBar.vue'

  defineOptions({ name: 'DockerSwarm' })

  const router = useRouter()
  const { selectedEnvId } = useDockerEnv()

  const state = ref<Docker.SwarmState>()
  const loading = ref(false)
  const submitting = ref(false)
  const initVisible = ref(false)
  const leaveVisible = ref(false)
  const forceLeave = ref(false)
  const advertiseAddr = ref('')
  const listenAddr = ref('0.0.0.0:2377')

  onMounted(() => load())
  watch(selectedEnvId, load)

  async function load() {
    if (!selectedEnvId.value) {
      state.value = undefined
      return
    }
    loading.value = true
    try {
      state.value = await fetchDockerSwarm(selectedEnvId.value)
    } catch {
      state.value = undefined
    } finally {
      loading.value = false
    }
  }

  async function initSwarm() {
    submitting.value = true
    try {
      await initDockerSwarm(selectedEnvId.value, {
        advertiseAddr: advertiseAddr.value,
        listenAddr: listenAddr.value
      })
      initVisible.value = false
      await load()
    } finally {
      submitting.value = false
    }
  }

  async function leaveSwarm() {
    submitting.value = true
    try {
      await leaveDockerSwarm(selectedEnvId.value, forceLeave.value)
      leaveVisible.value = false
      await load()
    } finally {
      submitting.value = false
    }
  }

  function go(path: string) {
    router.push(path)
  }
</script>