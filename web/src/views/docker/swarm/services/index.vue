<!-- Swarm 服务：创建、更新、扩缩容、日志与删除 -->
<template>
  <div class="docker-page art-full-height">
    <DockerEnvBar>
      <ElInput v-model="keyword" placeholder="搜索服务名称" clearable class="!w-56" @keyup.enter="load" @clear="load" />
      <ElButton v-auth="'docker:service:create'" type="primary" @click="openCreate">
        <ArtSvgIcon icon="ep:plus" class="mr-1" />创建服务
      </ElButton>
      <ElButton :loading="loading" @click="load">
        <ArtSvgIcon icon="ep:refresh" class="mr-1" />刷新
      </ElButton>
    </DockerEnvBar>

    <ElAlert v-if="!state?.active" class="mb-3" type="warning" :closable="false" show-icon>
      当前环境未启用 Swarm；服务列表需要管理节点。请先在「Swarm 集群」初始化或加入集群。
    </ElAlert>

    <ElCard class="art-table-card">
      <ElTable v-loading="loading" :data="services" row-key="id" empty-text="没有服务">
        <ElTableColumn label="服务" min-width="210">
          <template #default="{ row }">
            <div class="flex items-center gap-2">
              <span class="font-medium">{{ row.name }}</span>
              <ElTag v-if="row.stackName" size="small" type="primary" effect="plain">{{ row.stackName }}</ElTag>
            </div>
            <div class="mt-1 text-xs text-g-500"><code>{{ row.id.slice(0, 12) }}</code> · {{ row.mode }}</div>
          </template>
        </ElTableColumn>
        <ElTableColumn prop="image" label="镜像" min-width="190" show-overflow-tooltip />
        <ElTableColumn label="副本" width="110">
          <template #default="{ row }">
            <ElTag :type="row.running >= row.replicas && row.replicas > 0 ? 'success' : 'warning'" size="small">
              {{ row.running }}/{{ row.replicas }}
            </ElTag>
            <span v-if="row.mode === 'global'" class="ml-1 text-xs text-g-500">global</span>
          </template>
        </ElTableColumn>
        <ElTableColumn label="端口" min-width="180">
          <template #default="{ row }">
            <div v-if="row.ports?.length" class="flex flex-col gap-0.5 text-xs">
              <span v-for="(p, i) in row.ports" :key="i">
                *:{{ p.publishedPort }} → {{ p.targetPort }}/{{ p.protocol }} ({{ p.publishMode }})
              </span>
            </div>
            <span v-else class="text-g-500">-</span>
          </template>
        </ElTableColumn>
        <ElTableColumn label="更新时间" width="170">
          <template #default="{ row }">{{ formatTime(row.updatedAt) }}</template>
        </ElTableColumn>
        <ElTableColumn label="操作" width="340" fixed="right">
          <template #default="{ row }">
            <ElButton link type="primary" @click="openLogs(row)">日志</ElButton>
            <ElButton v-auth="'docker:service:scale'" link type="primary" @click="openScale(row)">扩缩容</ElButton>
            <ElButton v-auth="'docker:service:update'" link type="primary" @click="openEdit(row)">编辑</ElButton>
            <ElButton link type="primary" @click="inspect(row)">详情</ElButton>
            <ElButton v-auth="'docker:service:delete'" link type="danger" @click="remove(row)">删除</ElButton>
          </template>
        </ElTableColumn>
      </ElTable>
    </ElCard>

    <!-- 创建/编辑 -->
    <ElDialog v-model="dialogVisible" :title="editingId ? '编辑服务' : '创建 Swarm 服务'" width="720px" align-center>
      <ElForm ref="formRef" :model="form" :rules="rules" label-width="100px">
        <ElRow :gutter="12">
          <ElCol :span="12">
            <ElFormItem label="服务名称" prop="name">
              <ElInput v-model.trim="form.name" :disabled="!!editingId" placeholder="my-service" />
            </ElFormItem>
          </ElCol>
          <ElCol :span="12">
            <ElFormItem label="镜像" prop="image">
              <ElInput v-model.trim="form.image" placeholder="nginx:alpine" />
            </ElFormItem>
          </ElCol>
        </ElRow>
        <ElRow :gutter="12">
          <ElCol :span="12">
            <ElFormItem label="副本数">
              <ElInputNumber v-model="form.replicas" :min="0" :max="1000" controls-position="right" />
            </ElFormItem>
          </ElCol>
          <ElCol :span="12">
            <ElFormItem label="网络">
              <ElSelect v-model="form.networkId" clearable placeholder="不接入自定义网络" class="w-full">
                <ElOption v-for="n in networks" :key="n.id" :label="n.name" :value="n.id" />
              </ElSelect>
            </ElFormItem>
          </ElCol>
        </ElRow>
        <ElFormItem label="启动命令">
          <ElInput v-model="form.command" placeholder="可选，覆盖镜像 CMD（空格分隔）" />
        </ElFormItem>
        <ElFormItem label="调度约束">
          <ElInput v-model="form.constraintsText" placeholder="逗号分隔，如 node.role==manager,node.labels.zone==east" />
        </ElFormItem>

        <ElDivider content-position="left">端口</ElDivider>
        <div v-for="(p, i) in form.ports" :key="`p${i}`" class="mb-2 flex items-center gap-2">
          <ElInputNumber v-model="p.hostPort" :min="0" :max="65535" placeholder="发布端口" controls-position="right" />
          <span class="text-g-500">→</span>
          <ElInputNumber v-model="p.containerPort" :min="1" :max="65535" placeholder="容器端口" controls-position="right" />
          <ElSelect v-model="p.protocol" class="!w-24">
            <ElOption label="TCP" value="tcp" />
            <ElOption label="UDP" value="udp" />
          </ElSelect>
          <ElButton link type="danger" @click="form.ports.splice(i, 1)">移除</ElButton>
        </div>
        <ElButton size="small" @click="form.ports.push({ hostPort: 8080, containerPort: 80, protocol: 'tcp' })">添加端口</ElButton>

        <ElDivider content-position="left">环境变量</ElDivider>
        <div v-for="(e, i) in form.env" :key="`e${i}`" class="mb-2 flex items-center gap-2">
          <ElInput v-model="e.key" placeholder="KEY" class="!w-56" />
          <span class="text-g-500">=</span>
          <ElInput v-model="e.value" placeholder="VALUE" />
          <ElButton link type="danger" @click="form.env.splice(i, 1)">移除</ElButton>
        </div>
        <ElButton size="small" @click="form.env.push({ key: '', value: '' })">添加环境变量</ElButton>
      </ElForm>
      <template #footer>
        <ElButton @click="dialogVisible = false">取消</ElButton>
        <ElButton type="primary" :loading="submitting" @click="submit">保存</ElButton>
      </template>
    </ElDialog>

    <!-- 扩缩容 -->
    <ElDialog v-model="scaleVisible" :title="`扩缩容 - ${current?.name}`" width="440px" align-center>
      <div class="flex items-center gap-3">
        <ElInputNumber v-model="scaleValue" :min="0" :max="1000" controls-position="right" />
        <span class="text-g-500">个副本</span>
      </div>
      <template #footer>
        <ElButton @click="scaleVisible = false">取消</ElButton>
        <ElButton type="primary" :loading="scaling" @click="submitScale">应用</ElButton>
      </template>
    </ElDialog>

    <!-- 日志 -->
    <ElDrawer v-model="logsVisible" :title="`服务日志 - ${current?.name}`" size="70%" @open="loadLogs">
      <div class="mb-2 flex items-center gap-2">
        <ElSelect v-model="logTail" class="!w-32" @change="loadLogs">
          <ElOption :value="100" label="最近 100 行" />
          <ElOption :value="500" label="最近 500 行" />
          <ElOption :value="2000" label="最近 2000 行" />
        </ElSelect>
        <ElButton :loading="logsLoading" @click="loadLogs">刷新</ElButton>
      </div>
      <pre class="docker-log-view">{{ logs || '暂无日志' }}</pre>
    </ElDrawer>

    <ElDialog v-model="inspectVisible" title="服务详情（Docker inspect）" width="860px" align-center>
      <pre class="docker-log-view max-h-[65vh]">{{ inspectText }}</pre>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, reactive, ref, watch } from 'vue'
  import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
  import {
    createDockerService,
    fetchDockerNetworks,
    fetchDockerServiceDetail,
    fetchDockerServiceLogs,
    fetchDockerServices,
    fetchDockerSwarm,
    removeDockerService,
    scaleDockerService,
    updateDockerService
  } from '@/api/docker'
  import { useDockerEnv } from '@/hooks/core/useDockerEnv'
  import { formatTime } from '@/utils/web-basic'
  import DockerEnvBar from '../../components/DockerEnvBar.vue'

  defineOptions({ name: 'DockerSwarmServices' })

  const { selectedEnvId } = useDockerEnv()
  const services = ref<Docker.Service[]>([])
  const networks = ref<Docker.Network[]>([])
  const state = ref<Docker.SwarmState>()
  const loading = ref(false)
  const keyword = ref('')
  const current = ref<Docker.Service>()

  const dialogVisible = ref(false)
  const submitting = ref(false)
  const editingId = ref<string>()
  const formRef = ref<FormInstance>()
  const rules: FormRules = {
    name: [{ required: true, message: '请输入服务名称', trigger: 'blur' }],
    image: [{ required: true, message: '请输入镜像名称', trigger: 'blur' }]
  }
  const form = reactive({
    name: '',
    image: 'nginx:alpine',
    replicas: 1,
    command: '',
    networkId: '',
    constraintsText: '',
    ports: [] as Array<{ hostPort: number; containerPort: number; protocol: string }>,
    env: [] as Array<{ key: string; value: string }>
  })

  const scaleVisible = ref(false)
  const scaling = ref(false)
  const scaleValue = ref(1)

  const logsVisible = ref(false)
  const logsLoading = ref(false)
  const logs = ref('')
  const logTail = ref(500)

  const inspectVisible = ref(false)
  const inspectText = ref('')

  onMounted(() => load())
  watch(selectedEnvId, () => {
    load()
    loadNetworks()
  })

  async function load() {
    if (!selectedEnvId.value) {
      services.value = []
      return
    }
    loading.value = true
    try {
      try {
        state.value = await fetchDockerSwarm(selectedEnvId.value)
      } catch {
        state.value = undefined
      }
      services.value = state.value?.active
        ? await fetchDockerServices(selectedEnvId.value, { keyword: keyword.value || undefined })
        : []
    } finally {
      loading.value = false
    }
  }

  async function loadNetworks() {
    if (!selectedEnvId.value) return
    try {
      networks.value = await fetchDockerNetworks(selectedEnvId.value)
    } catch {
      networks.value = []
    }
  }

  function openCreate() {
    editingId.value = undefined
    formRef.value?.resetFields()
    form.name = ''
    form.image = 'nginx:alpine'
    form.replicas = 1
    form.command = ''
    form.networkId = ''
    form.constraintsText = ''
    form.ports = [{ hostPort: 8080, containerPort: 80, protocol: 'tcp' }]
    form.env = []
    loadNetworks()
    dialogVisible.value = true
  }

  async function openEdit(row: Docker.Service) {
    await loadNetworks()
    editingId.value = row.id
    form.name = row.name
    form.image = row.image
    form.replicas = row.replicas
    form.command = row.command || ''
    form.networkId = ''
    form.constraintsText = ''
    form.ports = (row.ports || []).map((p) => ({
      hostPort: p.publishedPort,
      containerPort: p.targetPort,
      protocol: p.protocol || 'tcp'
    }))
    form.env = []
    // 编辑时从 inspect 回填环境变量 / 网络 / 约束，避免保存时误清空。
    try {
      const detail: any = await fetchDockerServiceDetail(selectedEnvId.value, row.id)
      const spec = detail?.spec
      const containerSpec = spec?.TaskTemplate?.ContainerSpec
      if (Array.isArray(containerSpec?.Env)) {
        form.env = containerSpec.Env.map((item: string) => {
          const idx = item.indexOf('=')
          return { key: idx >= 0 ? item.slice(0, idx) : item, value: idx >= 0 ? item.slice(idx + 1) : '' }
        })
      }
      const constraints = spec?.TaskTemplate?.Placement?.Constraints
      if (Array.isArray(constraints)) form.constraintsText = constraints.join(',')
      const netTarget = spec?.TaskTemplate?.Networks?.[0]?.Target
      if (netTarget) {
        const matched = networks.value.find((n) => n.id === netTarget || n.name === netTarget)
        form.networkId = matched?.id || ''
      }
    } catch {
      // 回填失败不阻塞编辑
    }
    dialogVisible.value = true
  }

  async function submit() {
    if (!formRef.value) return
    if (!(await formRef.value.validate().catch(() => false))) return
    const payload: Record<string, any> = {
      name: form.name,
      image: form.image,
      replicas: form.replicas,
      command: form.command,
      networkId: form.networkId,
      constraints: form.constraintsText.split(',').map((s) => s.trim()).filter(Boolean),
      ports: form.ports.filter((p) => p.containerPort > 0),
      env: form.env
        .filter((e) => e.key.trim())
        .map((e) => `${e.key.trim()}=${e.value ?? ''}`)
    }
    submitting.value = true
    try {
      if (editingId.value) {
        await updateDockerService(selectedEnvId.value, editingId.value, payload)
      } else {
        await createDockerService(selectedEnvId.value, payload)
      }
      dialogVisible.value = false
      await load()
    } finally {
      submitting.value = false
    }
  }

  function openScale(row: Docker.Service) {
    current.value = row
    scaleValue.value = row.replicas
    scaleVisible.value = true
  }

  async function submitScale() {
    if (!current.value) return
    scaling.value = true
    try {
      await scaleDockerService(selectedEnvId.value, current.value.id, scaleValue.value)
      scaleVisible.value = false
      await load()
    } finally {
      scaling.value = false
    }
  }

  async function remove(row: Docker.Service) {
    try {
      await ElMessageBox.confirm(`确定删除服务「${row.name}」吗？该服务的所有任务会被移除。`, '删除服务', {
        type: 'warning',
        confirmButtonText: '删除',
        cancelButtonText: '取消'
      })
    } catch {
      return
    }
    await removeDockerService(selectedEnvId.value, row.id)
    await load()
  }

  function openLogs(row: Docker.Service) {
    current.value = row
    logs.value = ''
    logsVisible.value = true
  }

  async function loadLogs() {
    if (!current.value) return
    logsLoading.value = true
    try {
      const res = await fetchDockerServiceLogs(selectedEnvId.value, current.value.id, logTail.value)
      logs.value = res.logs || ''
    } finally {
      logsLoading.value = false
    }
  }

  async function inspect(row: Docker.Service) {
    const data = await fetchDockerServiceDetail(selectedEnvId.value, row.id)
    inspectText.value = JSON.stringify(data, null, 2)
    inspectVisible.value = true
  }
</script>

<style scoped>
.docker-log-view {
  max-height: 62vh;
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