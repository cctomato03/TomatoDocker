<!-- Stack 列表：compose/swarm 文件部署、运行态、历史回滚与导入导出 -->
<template>
  <div class="docker-page art-full-height">
    <DockerEnvBar>
      <ElButton v-auth="'docker:stack:create'" type="primary" @click="openCreate">
        <ArtSvgIcon icon="ep:plus" class="mr-1" />新建 Stack
      </ElButton>
      <ElButton :loading="loading" @click="load">
        <ArtSvgIcon icon="ep:refresh" class="mr-1" />刷新
      </ElButton>
      <ElButton v-auth="'docker:stack:import'" @click="importVisible = true">导入</ElButton>
      <ElButton v-auth="'docker:stack:export'" @click="exportStacks">导出</ElButton>
      <ElButton v-auth="'docker:stack:export'" @click="downloadBackup">下载 SQLite 备份</ElButton>
    </DockerEnvBar>

    <ElCard class="art-table-card">
      <ElTable v-loading="loading" :data="stacks" row-key="id" empty-text="当前环境还没有 Stack">
        <ElTableColumn label="Stack" min-width="220">
          <template #default="{ row }">
            <div class="flex items-center gap-2">
              <span class="font-medium">{{ row.name }}</span>
              <ElTag size="small" :type="row.type === 'swarm' ? 'primary' : 'success'" effect="plain">
                {{ row.type === 'swarm' ? 'Swarm' : 'Compose' }}
              </ElTag>
              <ElTag v-if="row.pendingChanges" size="small" type="warning">未部署变更</ElTag>
            </div>
            <div class="mt-1 text-xs text-g-500">
              环境：{{ row.environmentName }} · 版本 v{{ row.version }}
            </div>
          </template>
        </ElTableColumn>
        <ElTableColumn label="状态" width="110">
          <template #default="{ row }">
            <ElTag :type="statusType(row.status)" size="small">{{ statusText(row.status) }}</ElTag>
          </template>
        </ElTableColumn>
        <ElTableColumn label="运行态" min-width="190">
          <template #default="{ row }">
            <div v-if="row.runtimeError" class="text-xs text-red-500">连接失败</div>
            <div v-else class="text-sm">
              {{ row.type === 'swarm'
                ? `${row.runtime?.servicesRunning ?? 0}/${row.runtime?.servicesTotal ?? 0} 服务`
                : `${row.runtime?.containersRunning ?? 0}/${row.runtime?.containersTotal ?? 0} 容器` }}
              <span class="text-xs text-g-500">运行中 / 全部</span>
            </div>
          </template>
        </ElTableColumn>
        <ElTableColumn label="最近部署" width="170">
          <template #default="{ row }">{{ row.lastDeployedAt ? formatTime(row.lastDeployedAt) : '从未部署' }}</template>
        </ElTableColumn>
        <ElTableColumn label="操作" width="330" fixed="right">
          <template #default="{ row }">
            <ElButton link type="primary" @click="openDetail(row)">详情</ElButton>
            <ElButton v-auth="'docker:stack:update'" link type="primary" @click="editStack(row)">编辑</ElButton>
            <ElButton v-auth="'docker:stack:deploy'" link type="primary" @click="openDeploy(row)">部署</ElButton>
            <ElButton v-auth="'docker:stack:stop'" link type="warning" @click="stop(row)">停止</ElButton>
            <ElButton v-auth="'docker:stack:delete'" link type="danger" @click="openDelete(row)">删除</ElButton>
          </template>
        </ElTableColumn>
      </ElTable>
    </ElCard>

    <!-- 详情 -->
    <ElDrawer v-model="detailVisible" :title="`Stack 详情 - ${detail?.name}`" size="72%">
      <div v-if="detail" class="space-y-3">
        <ElDescriptions :column="3" border size="small">
          <ElDescriptionsItem label="环境">{{ detail.environmentName }}</ElDescriptionsItem>
          <ElDescriptionsItem label="类型">{{ detail.type }}</ElDescriptionsItem>
          <ElDescriptionsItem label="版本">v{{ detail.version }}</ElDescriptionsItem>
          <ElDescriptionsItem label="状态">{{ statusText(detail.status) }}</ElDescriptionsItem>
          <ElDescriptionsItem label="最近部署">{{ detail.lastDeployedAt ? formatTime(detail.lastDeployedAt) : '-' }}</ElDescriptionsItem>
          <ElDescriptionsItem label="更新时间">{{ formatTime(detail.updatedAt) }}</ElDescriptionsItem>
        </ElDescriptions>

        <ElAlert v-if="detail.statusMessage" :type="detail.status === 'error' ? 'error' : 'info'" :closable="false" show-icon>
          {{ detail.statusMessage }}
        </ElAlert>

        <ElTabs>
          <ElTabPane label="运行资源">
            <ElTable :data="detail.runtime?.resources ?? []" size="small" empty-text="尚未部署或运行时资源已不存在">
              <ElTableColumn prop="name" label="名称" min-width="200" />
              <ElTableColumn prop="kind" label="类型" width="100">
                <template #default="{ row }">{{ row.kind === 'service' ? '服务' : '容器' }}</template>
              </ElTableColumn>
              <ElTableColumn prop="state" label="状态" width="110" />
              <ElTableColumn prop="detail" label="说明" min-width="160" show-overflow-tooltip />
            </ElTable>
          </ElTabPane>
          <ElTabPane label="历史版本">
            <ElTable :data="detail.revisions" size="small">
              <ElTableColumn prop="version" label="版本" width="80">
                <template #default="{ row }">v{{ row.version }}</template>
              </ElTableColumn>
              <ElTableColumn prop="message" label="说明" min-width="200" />
              <ElTableColumn prop="createdBy" label="操作人" width="110" />
              <ElTableColumn label="时间" width="170">
                <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
              </ElTableColumn>
              <ElTableColumn label="操作" width="200">
                <template #default="{ row }">
                  <ElButton v-auth="'docker:stack:deploy'" link type="primary" @click="rollback(row.version, true)">
                    回滚并部署
                  </ElButton>
                  <ElButton v-auth="'docker:stack:update'" link @click="rollback(row.version, false)">仅回滚文件</ElButton>
                </template>
              </ElTableColumn>
            </ElTable>
          </ElTabPane>
          <ElTabPane label="操作记录">
            <ElTable :data="detail.activities" size="small" empty-text="暂无操作记录">
              <ElTableColumn prop="action" label="操作" width="110" />
              <ElTableColumn label="结果" width="90">
                <template #default="{ row }">
                  <ElTag size="small" :type="row.status === 'success' ? 'success' : 'danger'">
                    {{ row.status === 'success' ? '成功' : '失败' }}
                  </ElTag>
                </template>
              </ElTableColumn>
              <ElTableColumn prop="message" label="说明" min-width="240" show-overflow-tooltip />
              <ElTableColumn prop="operator" label="操作人" width="110" />
              <ElTableColumn label="时间" width="170">
                <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
              </ElTableColumn>
            </ElTable>
          </ElTabPane>
          <ElTabPane label="compose 文件">
            <DockerCodeEditor v-model="detail.composeContent" height="420px" readonly />
            <template v-if="detail.envContent">
              <ElDivider content-position="left">.env</ElDivider>
              <DockerCodeEditor v-model="detail.envContent" height="150px" readonly />
            </template>
          </ElTabPane>
        </ElTabs>
      </div>
    </ElDrawer>

    <!-- 部署 -->
    <ElDialog v-model="deployVisible" :title="`部署 Stack - ${current?.name}`" width="520px" align-center>
      <ElCheckbox v-model="pullImages">部署前拉取最新镜像（pull_policy=always）</ElCheckbox>
      <ElAlert type="warning" :closable="false" show-icon class="mt-3">
        重新部署会先移除该 Stack 的旧容器/服务，再按最新 compose 文件创建；数据卷默认保留。
      </ElAlert>
      <template #footer>
        <ElButton @click="deployVisible = false">取消</ElButton>
        <ElButton type="primary" :loading="deploying" @click="deploy">开始部署</ElButton>
      </template>
    </ElDialog>

    <!-- 删除 -->
    <ElDialog v-model="deleteVisible" :title="`删除 Stack - ${current?.name}`" width="520px" align-center>
      <ElCheckbox v-model="deleteRuntime">同时删除该 Stack 的运行资源（容器/服务/网络）</ElCheckbox>
      <ElCheckbox v-model="removeVolumes" class="ml-2">并删除数据卷（不可恢复）</ElCheckbox>
      <ElAlert type="warning" :closable="false" show-icon class="mt-3">
        删除后 SQLite 中的 compose 文件与历史版本会一并删除；如需保留请先导出。
      </ElAlert>
      <template #footer>
        <ElButton @click="deleteVisible = false">取消</ElButton>
        <ElButton type="danger" :loading="deleting" @click="removeStack">删除</ElButton>
      </template>
    </ElDialog>

    <!-- 导入 -->
    <ElDialog v-model="importVisible" title="导入 Stack（JSON）" width="720px" align-center>
      <ElAlert type="info" :closable="false" show-icon class="mb-3">
        按「环境名称」匹配目标环境；可选择覆盖同名 Stack。可先导出备份文件再修改。
      </ElAlert>
      <ElUpload :auto-upload="false" :show-file-list="false" accept=".json,application/json" :on-change="readImportFile">
        <ElButton>选择 JSON 文件</ElButton>
      </ElUpload>
      <ElInput v-model="importText" type="textarea" :rows="12" class="mt-3 font-mono" placeholder="粘贴导出的 JSON 内容" />
      <ElCheckbox v-model="importOverwrite" class="mt-2">覆盖同名 Stack（生成新版本）</ElCheckbox>
      <template #footer>
        <ElButton @click="importVisible = false">取消</ElButton>
        <ElButton type="primary" :loading="importing" @click="submitImport">导入</ElButton>
      </template>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, ref, watch } from 'vue'
  import { useRouter } from 'vue-router'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import type { UploadFile } from 'element-plus'
  import {
    deleteDockerStack,
    deployDockerStack,
    downloadDockerBackup,
    exportDockerStacks,
    fetchDockerStack,
    fetchDockerStacks,
    importDockerStacks,
    rollbackDockerStack,
    stopDockerStack
  } from '@/api/docker'
  import { useDockerEnv } from '@/hooks/core/useDockerEnv'
  import { formatTime } from '@/utils/web-basic'
  import DockerEnvBar from '../components/DockerEnvBar.vue'
  import DockerCodeEditor from '../components/DockerCodeEditor.vue'

  defineOptions({ name: 'DockerStacks' })

  const router = useRouter()
  const { selectedEnvId } = useDockerEnv()

  const stacks = ref<Docker.Stack[]>([])
  const loading = ref(false)

  const detailVisible = ref(false)
  const detail = ref<Docker.StackDetail>()
  const current = ref<Docker.Stack>()

  const deployVisible = ref(false)
  const deploying = ref(false)
  const pullImages = ref(false)

  const deleteVisible = ref(false)
  const deleting = ref(false)
  const deleteRuntime = ref(true)
  const removeVolumes = ref(false)

  const importVisible = ref(false)
  const importText = ref('')
  const importOverwrite = ref(false)
  const importing = ref(false)

  onMounted(() => load())
  watch(selectedEnvId, load)

  async function load() {
    if (!selectedEnvId.value) {
      stacks.value = []
      return
    }
    loading.value = true
    try {
      stacks.value = await fetchDockerStacks(selectedEnvId.value)
    } finally {
      loading.value = false
    }
  }

  function statusType(status: string) {
    if (status === 'active') return 'success'
    if (status === 'error') return 'danger'
    if (status === 'deploying') return 'warning'
    return 'info'
  }
  function statusText(status: string) {
    return (
      { active: '运行中', inactive: '已停止', error: '部署失败', deploying: '部署中' }[status] || status
    )
  }

  function openCreate() {
    router.push({ path: '/docker/stacks/editor', query: { envId: selectedEnvId.value } })
  }

  function editStack(row: Docker.Stack) {
    router.push({ path: '/docker/stacks/editor', query: { id: row.id } })
  }

  function openDeploy(row: Docker.Stack) {
    current.value = row
    pullImages.value = false
    deployVisible.value = true
  }

  async function deploy() {
    if (!current.value) return
    deploying.value = true
    try {
      await deployDockerStack(current.value.id, pullImages.value)
      deployVisible.value = false
      await load()
      if (detail.value?.id === current.value.id) await refreshDetail()
    } finally {
      deploying.value = false
    }
  }

  async function stop(row: Docker.Stack) {
    try {
      await ElMessageBox.confirm(`确定停止 Stack「${row.name}」吗？`, '停止 Stack', {
        type: 'warning',
        confirmButtonText: '停止',
        cancelButtonText: '取消'
      })
    } catch {
      return
    }
    await stopDockerStack(row.id)
    await load()
  }

  function openDelete(row: Docker.Stack) {
    current.value = row
    deleteRuntime.value = true
    removeVolumes.value = false
    deleteVisible.value = true
  }

  async function removeStack() {
    if (!current.value) return
    deleting.value = true
    try {
      await deleteDockerStack(current.value.id, deleteRuntime.value, removeVolumes.value)
      deleteVisible.value = false
      detailVisible.value = false
      await load()
    } finally {
      deleting.value = false
    }
  }

  async function openDetail(row: Docker.Stack) {
    current.value = row
    detailVisible.value = true
    await refreshDetail()
  }

  async function refreshDetail() {
    if (!current.value) return
    try {
      detail.value = await fetchDockerStack(current.value.id)
    } catch {
      detailVisible.value = false
    }
  }

  async function rollback(version: number, deployNow: boolean) {
    if (!detail.value) return
    try {
      await ElMessageBox.confirm(
        `确定回滚「${detail.value.name}」到 v${version} 吗？${deployNow ? '回滚后会立即重新部署。' : ''}`,
        '回滚 Stack',
        { type: 'warning', confirmButtonText: '回滚', cancelButtonText: '取消' }
      )
    } catch {
      return
    }
    await rollbackDockerStack(detail.value.id, version, deployNow)
    await refreshDetail()
    await load()
  }

  async function exportStacks() {
    const data = await exportDockerStacks()
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `tomato-stacks-${new Date().toISOString().slice(0, 10)}.json`
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
    ElMessage.success(`已导出 ${data.stacks?.length ?? 0} 个 Stack`)
  }

  async function downloadBackup() {
    await downloadDockerBackup()
    ElMessage.success('SQLite 备份已开始下载')
  }

  function readImportFile(file: UploadFile) {
    const raw = file.raw
    if (!raw) return
    const reader = new FileReader()
    reader.onload = () => {
      importText.value = String(reader.result || '')
    }
    reader.readAsText(raw)
  }

  async function submitImport() {
    if (!importText.value.trim()) {
      ElMessage.warning('请选择文件或粘贴 JSON 内容')
      return
    }
    let parsed: any
    try {
      parsed = JSON.parse(importText.value)
    } catch {
      ElMessage.error('JSON 格式不正确')
      return
    }
    const list = Array.isArray(parsed) ? parsed : parsed?.stacks
    if (!Array.isArray(list) || !list.length) {
      ElMessage.error('没有可导入的 Stack 数据（需要 stacks 数组）')
      return
    }
    importing.value = true
    try {
      const result = await importDockerStacks({ overwrite: importOverwrite.value, stacks: list })
      ElMessage.success(
        `导入完成：新建 ${result.created ?? 0}，更新 ${result.updated ?? 0}，跳过 ${result.skipped ?? 0}`
      )
      importVisible.value = false
      importText.value = ''
      await load()
    } finally {
      importing.value = false
    }
  }
</script>