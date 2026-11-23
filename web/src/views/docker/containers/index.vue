<!-- 容器管理：启停、日志、资源快照、命令执行与创建 -->
<template>
  <div class="docker-page art-full-height">
    <DockerEnvBar>
      <ElInput
        v-model="keyword"
        placeholder="搜索名称 / 镜像 / ID"
        clearable
        class="!w-56"
        @keyup.enter="load"
        @clear="load"
      />
      <ElSelect v-model="state" placeholder="状态" clearable class="!w-32" @change="load">
        <ElOption label="运行中" value="running" />
        <ElOption label="已停止" value="exited" />
        <ElOption label="暂停" value="paused" />
        <ElOption label="创建中" value="created" />
        <ElOption label="重启中" value="restarting" />
      </ElSelect>
      <ElButton v-auth="'docker:container:create'" type="primary" @click="openCreate">
        <ArtSvgIcon icon="ep:plus" class="mr-1" />创建容器
      </ElButton>
      <ElButton :loading="loading" @click="load">
        <ArtSvgIcon icon="ep:refresh" class="mr-1" />刷新
      </ElButton>
      <ElButton v-auth="'docker:container:delete'" @click="prune">清理已停止</ElButton>
    </DockerEnvBar>

    <ElCard class="art-table-card">
      <ElTable v-loading="loading" :data="containers" row-key="id" empty-text="当前环境没有容器">
        <ElTableColumn label="名称 / ID" min-width="220">
          <template #default="{ row }">
            <div class="flex items-center gap-2">
              <span class="font-medium">{{ row.name || row.id.slice(0, 12) }}</span>
              <ElTag v-if="row.health" size="small" :type="row.health === 'healthy' ? 'success' : row.health === 'unhealthy' ? 'danger' : 'warning'">
                {{ row.health }}
              </ElTag>
            </div>
            <div class="mt-1 text-xs text-g-500">
              <code>{{ row.id.slice(0, 12) }}</code>
              <span v-if="row.stackName" class="ml-2">Stack: {{ row.stackName }}</span>
              <span v-else-if="row.projectName" class="ml-2">Compose: {{ row.projectName }}</span>
              <span v-if="row.serviceName" class="ml-2">Service: {{ row.serviceName }}</span>
            </div>
          </template>
        </ElTableColumn>
        <ElTableColumn prop="image" label="镜像" min-width="180" show-overflow-tooltip />
        <ElTableColumn label="状态" width="150">
          <template #default="{ row }">
            <ElTag :type="stateType(row.state)" size="small">{{ stateText(row.state) }}</ElTag>
            <div class="mt-1 text-xs text-g-500">{{ row.status }}</div>
          </template>
        </ElTableColumn>
        <ElTableColumn label="端口" min-width="180">
          <template #default="{ row }">
            <div v-if="row.ports?.length" class="flex flex-col gap-0.5 text-xs">
              <span v-for="(p, i) in row.ports" :key="i">
                {{ p.publicPort ? `${p.ip || '0.0.0.0'}:${p.publicPort} → ` : '' }}{{ p.privatePort }}/{{ p.type }}
              </span>
            </div>
            <span v-else class="text-xs text-g-500">-</span>
          </template>
        </ElTableColumn>
        <ElTableColumn label="创建时间" width="170">
          <template #default="{ row }">{{ formatUnix(row.created) }}</template>
        </ElTableColumn>
        <ElTableColumn label="操作" width="300" fixed="right">
          <template #default="{ row }">
            <ElButton v-auth="'docker:container:operate'" link type="primary" @click="toggle(row)">
              {{ row.state === 'running' ? '停止' : '启动' }}
            </ElButton>
            <ElButton v-auth="'docker:container:operate'" link type="primary" @click="simpleAction(row, 'restart')">重启</ElButton>
            <ElButton link type="primary" @click="openLogs(row)">日志</ElButton>
            <ElButton link type="primary" @click="openStats(row)">资源</ElButton>
            <ElButton v-auth="'docker:container:exec'" link type="primary" @click="openExec(row)">命令</ElButton>
            <ElButton link type="primary" @click="openInspect(row)">详情</ElButton>
            <ElButton v-auth="'docker:container:delete'" link type="danger" @click="remove(row)">删除</ElButton>
          </template>
        </ElTableColumn>
      </ElTable>
    </ElCard>

    <!-- 日志 -->
    <ElDrawer v-model="logsVisible" :title="`容器日志 - ${currentRow?.name}`" size="70%" @open="loadLogs">
      <div class="mb-2 flex items-center gap-2">
        <ElSelect v-model="logTail" class="!w-32" @change="loadLogs">
          <ElOption :value="100" label="最近 100 行" />
          <ElOption :value="500" label="最近 500 行" />
          <ElOption :value="2000" label="最近 2000 行" />
        </ElSelect>
        <ElCheckbox v-model="logTimestamps" @change="loadLogs">显示时间戳</ElCheckbox>
        <ElButton :loading="logsLoading" @click="loadLogs">刷新</ElButton>
      </div>
      <pre class="docker-log-view">{{ logs || '暂无日志' }}</pre>
    </ElDrawer>

    <!-- 资源 -->
    <ElDialog v-model="statsVisible" title="容器资源快照" width="520px" align-center>
      <div v-if="stats" class="space-y-3">
        <div>
          <div class="flex-cb mb-1 text-sm"><span>CPU</span><span>{{ stats.cpuPercent.toFixed(2) }}%</span></div>
          <ElProgress :percentage="Math.min(100, stats.cpuPercent)" :show-text="false" />
        </div>
        <div>
          <div class="flex-cb mb-1 text-sm">
            <span>内存</span><span>{{ formatSize(stats.memUsage) }} / {{ formatSize(stats.memLimit) }}</span>
          </div>
          <ElProgress :percentage="Math.min(100, stats.memPercent)" :show-text="false" />
        </div>
        <ElDescriptions :column="2" border size="small">
          <ElDescriptionsItem label="网络接收">{{ formatSize(stats.netRx) }}</ElDescriptionsItem>
          <ElDescriptionsItem label="网络发送">{{ formatSize(stats.netTx) }}</ElDescriptionsItem>
          <ElDescriptionsItem label="磁盘读取">{{ formatSize(stats.blockRead) }}</ElDescriptionsItem>
          <ElDescriptionsItem label="磁盘写入">{{ formatSize(stats.blockWrite) }}</ElDescriptionsItem>
          <ElDescriptionsItem label="进程数">{{ stats.pids }}</ElDescriptionsItem>
          <ElDescriptionsItem label="采集时间">{{ now }}</ElDescriptionsItem>
        </ElDescriptions>
      </div>
      <template #footer>
        <ElButton :loading="statsLoading" @click="refreshStats">刷新</ElButton>
        <ElButton type="primary" @click="statsVisible = false">关闭</ElButton>
      </template>
    </ElDialog>

    <!-- 命令 -->
    <ElDialog v-model="execVisible" title="容器内执行命令" width="720px" align-center destroy-on-close>
      <ElAlert type="info" :closable="false" show-icon class="mb-3">
        命令通过 <code>/bin/sh -c</code> 执行，最长 60 秒；非交互式，适用于诊断而非登录终端。
      </ElAlert>
      <ElInput v-model="execCommand" placeholder="如：nginx -v 或 ls -al /data" @keyup.enter="runExec">
        <template #prepend>sh -c</template>
      </ElInput>
      <div v-if="execOutput || execExitCode !== null" class="mt-3">
        <div class="mb-1 text-xs text-g-500">退出码：{{ execExitCode ?? '-' }}</div>
        <pre class="docker-log-view">{{ execOutput || '(无输出)' }}</pre>
      </div>
      <template #footer>
        <ElButton @click="execVisible = false">关闭</ElButton>
        <ElButton type="primary" :loading="execLoading" @click="runExec">执行</ElButton>
      </template>
    </ElDialog>

    <!-- 详情 -->
    <ElDialog v-model="inspectVisible" title="容器详情（Docker inspect）" width="860px" align-center>
      <pre class="docker-log-view max-h-[65vh]">{{ inspectText }}</pre>
    </ElDialog>

    <!-- 创建 -->
    <ElDialog v-model="createVisible" title="创建容器" width="780px" align-center destroy-on-close>
      <ElForm ref="createFormRef" :model="form" :rules="createRules" label-width="100px">
        <ElRow :gutter="12">
          <ElCol :span="12">
            <ElFormItem label="容器名称">
              <ElInput v-model.trim="form.name" placeholder="留空自动生成" />
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
            <ElFormItem label="启动命令">
              <ElInput v-model="form.command" placeholder="可选，如 nginx -g 'daemon off;'" />
            </ElFormItem>
          </ElCol>
          <ElCol :span="12">
            <ElFormItem label="网络">
              <ElSelect v-model="form.networkId" clearable placeholder="默认 bridge" class="w-full">
                <ElOption v-for="n in networks" :key="n.id" :label="n.name" :value="n.id" />
              </ElSelect>
            </ElFormItem>
          </ElCol>
        </ElRow>
        <ElRow :gutter="12">
          <ElCol :span="12">
            <ElFormItem label="重启策略">
              <ElSelect v-model="form.restartPolicy" class="w-full">
                <ElOption label="不自动重启" value="no" />
                <ElOption label="always" value="always" />
                <ElOption label="unless-stopped" value="unless-stopped" />
                <ElOption label="on-failure" value="on-failure" />
              </ElSelect>
            </ElFormItem>
          </ElCol>
          <ElCol :span="12">
            <ElFormItem label="选项">
              <ElCheckbox v-model="form.autoStart">创建后启动</ElCheckbox>
              <ElCheckbox v-model="form.privileged">特权模式</ElCheckbox>
              <ElCheckbox v-model="form.tty">分配 TTY</ElCheckbox>
            </ElFormItem>
          </ElCol>
        </ElRow>

        <ElDivider content-position="left">端口映射</ElDivider>
        <div v-for="(p, i) in form.ports" :key="`p${i}`" class="mb-2 flex items-center gap-2">
          <ElInput v-model="p.hostIp" placeholder="监听 IP（可空）" class="!w-40" />
          <ElInputNumber v-model="p.hostPort" :min="0" :max="65535" placeholder="宿主端口" controls-position="right" />
          <span class="text-g-500">→</span>
          <ElInputNumber v-model="p.containerPort" :min="1" :max="65535" placeholder="容器端口" controls-position="right" />
          <ElSelect v-model="p.protocol" class="!w-24"><ElOption label="TCP" value="tcp" /><ElOption label="UDP" value="udp" /></ElSelect>
          <ElButton link type="danger" @click="form.ports.splice(i, 1)">移除</ElButton>
        </div>
        <ElButton size="small" @click="form.ports.push({ hostIp: '', hostPort: undefined, containerPort: 80, protocol: 'tcp' })">添加端口</ElButton>

        <ElDivider content-position="left">环境变量</ElDivider>
        <div v-for="(e, i) in form.env" :key="`e${i}`" class="mb-2 flex items-center gap-2">
          <ElInput v-model="e.key" placeholder="KEY" class="!w-56" />
          <span class="text-g-500">=</span>
          <ElInput v-model="e.value" placeholder="VALUE" />
          <ElButton link type="danger" @click="form.env.splice(i, 1)">移除</ElButton>
        </div>
        <ElButton size="small" @click="form.env.push({ key: '', value: '' })">添加环境变量</ElButton>

        <ElDivider content-position="left">目录 / 数据卷挂载</ElDivider>
        <div v-for="(v, i) in form.volumes" :key="`v${i}`" class="mb-2 flex items-center gap-2">
          <ElInput v-model="v.source" placeholder="宿主路径或数据卷名" />
          <span class="text-g-500">→</span>
          <ElInput v-model="v.target" placeholder="容器内路径" />
          <ElCheckbox v-model="v.readOnly">只读</ElCheckbox>
          <ElButton link type="danger" @click="form.volumes.splice(i, 1)">移除</ElButton>
        </div>
        <ElButton size="small" @click="form.volumes.push({ source: '', target: '', readOnly: false })">添加挂载</ElButton>
      </ElForm>
      <template #footer>
        <ElButton @click="createVisible = false">取消</ElButton>
        <ElButton type="primary" :loading="creating" @click="submitCreate">创建</ElButton>
      </template>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted, reactive, ref, watch } from 'vue'
  import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
  import {
    createDockerContainer,
    dockerContainerAction,
    execDockerContainer,
    fetchDockerContainerInspect,
    fetchDockerContainerLogs,
    fetchDockerContainers,
    fetchDockerContainerStats,
    fetchDockerNetworks,
    pruneDockerContainers,
    removeDockerContainer
  } from '@/api/docker'
  import { useDockerEnv } from '@/hooks/core/useDockerEnv'
  import { formatSize, formatUnix } from '@/utils/web-basic'
  import DockerEnvBar from '../components/DockerEnvBar.vue'

  defineOptions({ name: 'DockerContainers' })

  const { selectedEnvId } = useDockerEnv()

  const containers = ref<Docker.Container[]>([])
  const networks = ref<Docker.Network[]>([])
  const loading = ref(false)
  const keyword = ref('')
  const state = ref('')
  const now = ref('')

  const currentRow = ref<Docker.Container>()

  /* 日志 */
  const logsVisible = ref(false)
  const logsLoading = ref(false)
  const logs = ref('')
  const logTail = ref(500)
  const logTimestamps = ref(true)

  /* 资源 */
  const statsVisible = ref(false)
  const statsLoading = ref(false)
  const stats = ref<Docker.ContainerStats>()

  /* 命令 */
  const execVisible = ref(false)
  const execLoading = ref(false)
  const execCommand = ref('')
  const execOutput = ref('')
  const execExitCode = ref<number | null>(null)

  /* 详情 */
  const inspectVisible = ref(false)
  const inspectText = ref('')

  /* 创建 */
  const createVisible = ref(false)
  const creating = ref(false)
  const createFormRef = ref<FormInstance>()
  const createRules: FormRules = {
    image: [{ required: true, message: '请输入镜像名称', trigger: 'blur' }]
  }
  const form = reactive({
    name: '',
    image: 'nginx:alpine',
    command: '',
    networkId: '',
    restartPolicy: 'unless-stopped',
    autoStart: true,
    privileged: false,
    tty: false,
    ports: [] as Array<{ hostIp: string; hostPort?: number; containerPort: number; protocol: string }>,
    env: [] as Array<{ key: string; value: string }>,
    volumes: [] as Array<{ source: string; target: string; readOnly: boolean }>
  })

  onMounted(() => load())

  watch(selectedEnvId, () => {
    load()
    loadNetworks()
  })

  async function load() {
    if (!selectedEnvId.value) {
      containers.value = []
      return
    }
    loading.value = true
    try {
      containers.value = await fetchDockerContainers(selectedEnvId.value, {
        all: true,
        keyword: keyword.value || undefined,
        state: state.value || undefined
      })
    } finally {
      loading.value = false
    }
  }

  async function loadNetworks() {
    if (!selectedEnvId.value) {
      networks.value = []
      return
    }
    try {
      networks.value = await fetchDockerNetworks(selectedEnvId.value)
    } catch {
      networks.value = []
    }
  }

  function stateType(s: string) {
    if (s === 'running') return 'success'
    if (s === 'paused') return 'warning'
    if (s === 'exited' || s === 'dead') return 'danger'
    return 'info'
  }
  function stateText(s: string) {
    return (
      {
        running: '运行中',
        exited: '已停止',
        paused: '已暂停',
        created: '已创建',
        restarting: '重启中',
        removing: '删除中',
        dead: '已终止'
      }[s] || s
    )
  }

  async function toggle(row: Docker.Container) {
    await dockerContainerAction(selectedEnvId.value, row.id, row.state === 'running' ? 'stop' : 'start')
    await load()
  }

  async function simpleAction(row: Docker.Container, action: string) {
    await dockerContainerAction(selectedEnvId.value, row.id, action)
    await load()
  }

  async function remove(row: Docker.Container) {
    try {
      await ElMessageBox.confirm(`确定删除容器「${row.name}」吗？`, '删除容器', {
        type: 'warning',
        confirmButtonText: '删除',
        cancelButtonText: '取消'
      })
    } catch {
      return
    }
    await removeDockerContainer(selectedEnvId.value, row.id, true, false)
    await load()
  }

  async function prune() {
    try {
      await ElMessageBox.confirm('将删除所有已停止的容器（不影响数据卷），确定继续吗？', '清理容器', {
        type: 'warning',
        confirmButtonText: '清理',
        cancelButtonText: '取消'
      })
    } catch {
      return
    }
    await pruneDockerContainers(selectedEnvId.value)
    await load()
  }

  function openLogs(row: Docker.Container) {
    currentRow.value = row
    logs.value = ''
    logsVisible.value = true
  }

  async function loadLogs() {
    if (!currentRow.value) return
    logsLoading.value = true
    try {
      const res = await fetchDockerContainerLogs(
        selectedEnvId.value,
        currentRow.value.id,
        logTail.value,
        logTimestamps.value
      )
      logs.value = res.logs || ''
    } finally {
      logsLoading.value = false
    }
  }

  async function openStats(row: Docker.Container) {
    currentRow.value = row
    statsVisible.value = true
    await refreshStats()
  }

  async function refreshStats() {
    if (!currentRow.value) return
    statsLoading.value = true
    try {
      stats.value = await fetchDockerContainerStats(selectedEnvId.value, currentRow.value.id)
      now.value = new Date().toLocaleTimeString()
    } finally {
      statsLoading.value = false
    }
  }

  function openExec(row: Docker.Container) {
    currentRow.value = row
    execCommand.value = ''
    execOutput.value = ''
    execExitCode.value = null
    execVisible.value = true
  }

  async function runExec() {
    if (!currentRow.value || !execCommand.value.trim()) {
      ElMessage.warning('请输入要执行的命令')
      return
    }
    execLoading.value = true
    try {
      const res = await execDockerContainer(selectedEnvId.value, currentRow.value.id, execCommand.value)
      execOutput.value = res.output || ''
      execExitCode.value = res.exitCode
    } finally {
      execLoading.value = false
    }
  }

  async function openInspect(row: Docker.Container) {
    const data = await fetchDockerContainerInspect(selectedEnvId.value, row.id)
    inspectText.value = JSON.stringify(data, null, 2)
    inspectVisible.value = true
  }

  function openCreate() {
    createFormRef.value?.resetFields()
    form.name = ''
    form.image = 'nginx:alpine'
    form.command = ''
    form.networkId = ''
    form.restartPolicy = 'unless-stopped'
    form.autoStart = true
    form.privileged = false
    form.tty = false
    form.ports = [{ hostIp: '', hostPort: 8080, containerPort: 80, protocol: 'tcp' }]
    form.env = []
    form.volumes = []
    loadNetworks()
    createVisible.value = true
  }

  async function submitCreate() {
    if (!createFormRef.value) return
    const valid = await createFormRef.value.validate().catch(() => false)
    if (!valid) return
    creating.value = true
    try {
      await createDockerContainer(selectedEnvId.value, {
        name: form.name,
        image: form.image,
        command: form.command,
        networkId: form.networkId,
        restartPolicy: form.restartPolicy,
        autoStart: form.autoStart,
        privileged: form.privileged,
        tty: form.tty,
        ports: form.ports.filter((p) => p.containerPort > 0),
        env: form.env
          .filter((e) => e.key.trim())
          .map((e) => `${e.key.trim()}=${e.value ?? ''}`),
        volumes: form.volumes.filter((v) => v.source && v.target)
      })
      createVisible.value = false
      await load()
    } finally {
      creating.value = false
    }
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