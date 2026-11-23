<!-- Docker 网络管理：创建、详情、容器接入/断开与清理 -->
<template>
  <div class="docker-page art-full-height">
    <DockerEnvBar>
      <ElInput v-model="keyword" placeholder="搜索网络名称" clearable class="!w-56" @keyup.enter="load" @clear="load" />
      <ElButton v-auth="'docker:network:create'" type="primary" @click="openCreate">
        <ArtSvgIcon icon="ep:plus" class="mr-1" />创建网络
      </ElButton>
      <ElButton :loading="loading" @click="load">
        <ArtSvgIcon icon="ep:refresh" class="mr-1" />刷新
      </ElButton>
      <ElButton v-auth="'docker:network:delete'" @click="prune">清理未使用</ElButton>
    </DockerEnvBar>

    <ElCard class="art-table-card">
      <ElTable v-loading="loading" :data="networks" row-key="id" empty-text="当前环境没有网络">
        <ElTableColumn label="名称" min-width="200">
          <template #default="{ row }">
            <span class="font-medium">{{ row.name }}</span>
            <ElTag v-if="row.ingress" class="ml-2" size="small" type="primary" effect="plain">ingress</ElTag>
            <ElTag v-if="row.internal" class="ml-2" size="small" type="warning" effect="plain">internal</ElTag>
            <div class="mt-1 text-xs text-g-500"><code>{{ row.id.slice(0, 12) }}</code></div>
          </template>
        </ElTableColumn>
        <ElTableColumn prop="driver" label="驱动" width="100" />
        <ElTableColumn prop="scope" label="范围" width="90" />
        <ElTableColumn label="子网" min-width="160">
          <template #default="{ row }">
            <span v-if="row.subnets?.length">{{ row.subnets.join(', ') }}</span>
            <span v-else class="text-g-500">-</span>
          </template>
        </ElTableColumn>
        <ElTableColumn prop="containers" label="容器数" width="90" />
        <ElTableColumn label="创建时间" width="170">
          <template #default="{ row }">{{ formatTime(row.created) }}</template>
        </ElTableColumn>
        <ElTableColumn label="操作" width="250" fixed="right">
          <template #default="{ row }">
            <ElButton link type="primary" @click="inspect(row)">详情</ElButton>
            <ElButton v-auth="'docker:network:update'" link type="primary" @click="openAttach(row)">接入容器</ElButton>
            <ElButton v-auth="'docker:network:update'" link type="warning" @click="openDisconnect(row)">断开容器</ElButton>
            <ElButton v-auth="'docker:network:delete'" link type="danger" @click="remove(row)">删除</ElButton>
          </template>
        </ElTableColumn>
      </ElTable>
    </ElCard>

    <ElDialog v-model="createVisible" title="创建网络" width="560px" align-center destroy-on-close>
      <ElForm ref="formRef" :model="form" :rules="rules" label-width="100px">
        <ElFormItem label="网络名称" prop="name">
          <ElInput v-model.trim="form.name" placeholder="my-network" />
        </ElFormItem>
        <ElFormItem label="驱动">
          <ElSelect v-model="form.driver" class="w-full">
            <ElOption label="bridge" value="bridge" />
            <ElOption label="overlay（Swarm）" value="overlay" />
            <ElOption label="macvlan" value="macvlan" />
            <ElOption label="host" value="host" />
          </ElSelect>
        </ElFormItem>
        <ElRow :gutter="12">
          <ElCol :span="12">
            <ElFormItem label="子网">
              <ElInput v-model.trim="form.subnet" placeholder="172.30.0.0/16" />
            </ElFormItem>
          </ElCol>
          <ElCol :span="12">
            <ElFormItem label="网关">
              <ElInput v-model.trim="form.gateway" placeholder="172.30.0.1" />
            </ElFormItem>
          </ElCol>
        </ElRow>
        <ElFormItem label="选项">
          <ElCheckbox v-model="form.internal">内部网络</ElCheckbox>
          <ElCheckbox v-model="form.attachable">可手动接入（overlay）</ElCheckbox>
          <ElCheckbox v-model="form.enableIpv6">启用 IPv6</ElCheckbox>
        </ElFormItem>
      </ElForm>
      <template #footer>
        <ElButton @click="createVisible = false">取消</ElButton>
        <ElButton type="primary" :loading="submitting" @click="submit">创建</ElButton>
      </template>
    </ElDialog>

    <ElDialog v-model="attachVisible" :title="`接入容器 - ${current?.name}`" width="560px" align-center>
      <ElForm label-width="100px">
        <ElFormItem label="容器">
          <ElSelect v-model="attachContainer" filterable placeholder="选择容器" class="w-full">
            <ElOption v-for="c in containers" :key="c.id" :label="`${c.name}（${c.image}）`" :value="c.id" />
          </ElSelect>
        </ElFormItem>
        <ElFormItem label="网络别名">
          <ElInput v-model="attachAliases" placeholder="多个别名用逗号分隔（可选）" />
        </ElFormItem>
      </ElForm>
      <ElAlert type="info" :closable="false" show-icon class="mt-2">
        接入后容器可以通过网络别名被同网络容器访问；删除网络前请先断开所有容器。
      </ElAlert>
      <template #footer>
        <ElButton @click="attachVisible = false">取消</ElButton>
        <ElButton type="primary" :loading="attaching" @click="submitAttach">接入</ElButton>
      </template>
    </ElDialog>

    <ElDialog v-model="disconnectVisible" :title="`断开容器 - ${current?.name}`" width="520px" align-center>
      <ElForm label-width="100px">
        <ElFormItem label="已接入容器">
          <ElSelect v-model="disconnectContainer" filterable placeholder="选择要断开的容器" class="w-full">
            <ElOption v-for="c in networkContainers" :key="c.id" :label="`${c.name}（${c.id.slice(0, 12)}）`" :value="c.id" />
          </ElSelect>
        </ElFormItem>
      </ElForm>
      <ElEmpty v-if="!networkContainers.length" description="该网络当前没有接入容器" :image-size="80" />
      <template #footer>
        <ElButton @click="disconnectVisible = false">取消</ElButton>
        <ElButton type="warning" :loading="disconnecting" :disabled="!disconnectContainer" @click="submitDisconnect">
          断开
        </ElButton>
      </template>
    </ElDialog>

    <ElDialog v-model="inspectVisible" title="网络详情（Docker inspect）" width="860px" align-center>
      <pre class="docker-log-view max-h-[60vh]">{{ inspectText }}</pre>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, reactive, ref, watch } from 'vue'
  import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
  import {
    connectDockerNetwork,
    createDockerNetwork,
    disconnectDockerNetwork,
    fetchDockerContainers,
    fetchDockerNetworkInspect,
    fetchDockerNetworks,
    pruneDockerNetworks,
    removeDockerNetwork
  } from '@/api/docker'
  import { useDockerEnv } from '@/hooks/core/useDockerEnv'
  import { formatTime } from '@/utils/web-basic'
  import DockerEnvBar from '../components/DockerEnvBar.vue'

  defineOptions({ name: 'DockerNetworks' })

  const { selectedEnvId } = useDockerEnv()
  const networks = ref<Docker.Network[]>([])
  const loading = ref(false)
  const keyword = ref('')

  const createVisible = ref(false)
  const submitting = ref(false)
  const formRef = ref<FormInstance>()
  const rules: FormRules = { name: [{ required: true, message: '请输入网络名称', trigger: 'blur' }] }
  const form = reactive({
    name: '',
    driver: 'bridge',
    subnet: '',
    gateway: '',
    internal: false,
    attachable: false,
    enableIpv6: false
  })

  const attachVisible = ref(false)
  const attaching = ref(false)
  const current = ref<Docker.Network>()
  const containers = ref<Docker.Container[]>([])
  const attachContainer = ref('')
  const attachAliases = ref('')

  const disconnectVisible = ref(false)
  const disconnecting = ref(false)
  const disconnectContainer = ref('')
  const networkContainers = ref<Array<{ id: string; name: string }>>([])

  const inspectVisible = ref(false)
  const inspectText = ref('')

  onMounted(() => load())
  watch(selectedEnvId, load)

  async function load() {
    if (!selectedEnvId.value) {
      networks.value = []
      return
    }
    loading.value = true
    try {
      networks.value = await fetchDockerNetworks(selectedEnvId.value, { keyword: keyword.value || undefined })
    } finally {
      loading.value = false
    }
  }

  function openCreate() {
    form.name = ''
    form.driver = 'bridge'
    form.subnet = ''
    form.gateway = ''
    form.internal = false
    form.attachable = false
    form.enableIpv6 = false
    createVisible.value = true
  }

  async function submit() {
    if (!formRef.value) return
    if (!(await formRef.value.validate().catch(() => false))) return
    submitting.value = true
    try {
      await createDockerNetwork(selectedEnvId.value, { ...form })
      createVisible.value = false
      await load()
    } finally {
      submitting.value = false
    }
  }

  async function openAttach(row: Docker.Network) {
    current.value = row
    attachContainer.value = ''
    attachAliases.value = ''
    attachVisible.value = true
    try {
      containers.value = await fetchDockerContainers(selectedEnvId.value, { all: true })
    } catch {
      containers.value = []
    }
  }

  async function submitAttach() {
    if (!attachContainer.value) {
      ElMessage.warning('请选择容器')
      return
    }
    attaching.value = true
    try {
      await connectDockerNetwork(
        selectedEnvId.value,
        current.value!.id,
        attachContainer.value,
        attachAliases.value.split(',').map((s) => s.trim()).filter(Boolean)
      )
      attachVisible.value = false
      await load()
    } finally {
      attaching.value = false
    }
  }

  async function openDisconnect(row: Docker.Network) {
    current.value = row
    disconnectContainer.value = ''
    networkContainers.value = []
    disconnectVisible.value = true
    try {
      const data: any = await fetchDockerNetworkInspect(selectedEnvId.value, row.id)
      const map = data?.Containers || {}
      networkContainers.value = Object.entries(map).map(([id, value]: any) => ({
        id,
        name: value?.Name || id.slice(0, 12)
      }))
    } catch {
      networkContainers.value = []
    }
  }

  async function submitDisconnect() {
    if (!current.value || !disconnectContainer.value) return
    disconnecting.value = true
    try {
      await disconnectDockerNetwork(selectedEnvId.value, current.value.id, disconnectContainer.value)
      disconnectVisible.value = false
      await load()
    } finally {
      disconnecting.value = false
    }
  }

  async function remove(row: Docker.Network) {
    try {
      await ElMessageBox.confirm(`确定删除网络「${row.name}」吗？仍有容器接入时会失败。`, '删除网络', {
        type: 'warning',
        confirmButtonText: '删除',
        cancelButtonText: '取消'
      })
    } catch {
      return
    }
    await removeDockerNetwork(selectedEnvId.value, row.id)
    await load()
  }

  async function prune() {
    try {
      await ElMessageBox.confirm('将清理所有未使用的自定义网络，确定继续吗？', '清理网络', {
        type: 'warning',
        confirmButtonText: '清理',
        cancelButtonText: '取消'
      })
    } catch {
      return
    }
    await pruneDockerNetworks(selectedEnvId.value)
    await load()
  }

  async function inspect(row: Docker.Network) {
    const data = await fetchDockerNetworkInspect(selectedEnvId.value, row.id)
    inspectText.value = JSON.stringify(data, null, 2)
    inspectVisible.value = true
  }
</script>

<style scoped>
.docker-log-view {
  padding: 12px 14px;
  overflow: auto;
  font-family: 'JetBrains Mono', Consolas, Monaco, monospace;
  font-size: 12px;
  line-height: 1.55;
  color: #c8d3e0;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  background: #1e2227;
  border-radius: 8px;
}
</style>