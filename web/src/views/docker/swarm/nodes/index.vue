<!-- Swarm 节点：角色、可用性、标签与移除 -->
<template>
  <div class="docker-page art-full-height">
    <DockerEnvBar>
      <ElButton :loading="loading" @click="load">
        <ArtSvgIcon icon="ep:refresh" class="mr-1" />刷新
      </ElButton>
    </DockerEnvBar>

    <ElAlert v-if="!state?.active" class="mb-3" type="warning" :closable="false" show-icon>
      当前环境未启用 Swarm，请先在「Swarm 集群」初始化或加入集群。
    </ElAlert>

    <ElCard class="art-table-card">
      <ElTable v-loading="loading" :data="nodes" row-key="id" empty-text="没有节点">
        <ElTableColumn label="节点" min-width="210">
          <template #default="{ row }">
            <div class="flex items-center gap-2">
              <span class="font-medium">{{ row.hostname }}</span>
              <ElTag v-if="row.leader" size="small" type="danger" effect="plain">Leader</ElTag>
            </div>
            <div class="mt-1 text-xs text-g-500"><code>{{ row.id.slice(0, 12) }}</code> · {{ row.addr }}</div>
          </template>
        </ElTableColumn>
        <ElTableColumn label="角色" width="100">
          <template #default="{ row }">
            <ElTag size="small" :type="row.role === 'manager' ? 'primary' : 'info'">
              {{ row.role === 'manager' ? '管理者' : '工作者' }}
            </ElTag>
          </template>
        </ElTableColumn>
        <ElTableColumn label="状态" width="120">
          <template #default="{ row }">
            <ElTag :type="row.status === 'ready' ? 'success' : 'danger'" size="small">{{ row.status }}</ElTag>
            <span class="ml-1 text-xs text-g-500">{{ row.managerStatus }}</span>
          </template>
        </ElTableColumn>
        <ElTableColumn label="可用性" width="110">
          <template #default="{ row }">
            <ElTag size="small" :type="row.availability === 'active' ? 'success' : 'warning'" effect="plain">
              {{ row.availability }}
            </ElTag>
          </template>
        </ElTableColumn>
        <ElTableColumn label="规格" width="170">
          <template #default="{ row }">{{ row.nCpu }} CPU / {{ formatSize(row.memTotal) }}</template>
        </ElTableColumn>
        <ElTableColumn label="平台 / 版本" min-width="170">
          <template #default="{ row }">{{ row.platformOs }}/{{ row.platformArch }} · Docker {{ row.version }}</template>
        </ElTableColumn>
        <ElTableColumn label="标签" min-width="150" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="Object.keys(row.labels || {}).length">{{ labelText(row.labels) }}</span>
            <span v-else class="text-g-500">-</span>
          </template>
        </ElTableColumn>
        <ElTableColumn label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <ElButton v-auth="'docker:node:update'" link type="primary" @click="openEdit(row)">编辑</ElButton>
            <ElButton v-auth="'docker:node:delete'" link type="danger" @click="remove(row)">移除</ElButton>
          </template>
        </ElTableColumn>
      </ElTable>
    </ElCard>

    <ElDialog v-model="editVisible" :title="`编辑节点 - ${current?.hostname}`" width="560px" align-center>
      <ElForm label-width="100px">
        <ElFormItem label="角色">
          <ElRadioGroup v-model="editForm.role">
            <ElRadio value="worker">工作者</ElRadio>
            <ElRadio value="manager">管理者</ElRadio>
          </ElRadioGroup>
        </ElFormItem>
        <ElFormItem label="可用性">
          <ElRadioGroup v-model="editForm.availability">
            <ElRadio value="active">可调度</ElRadio>
            <ElRadio value="pause">暂停调度</ElRadio>
            <ElRadio value="drain">排空（drain）</ElRadio>
          </ElRadioGroup>
        </ElFormItem>
        <ElFormItem label="节点标签">
          <ElInput v-model="editForm.labelsText" type="textarea" :rows="4" placeholder='JSON，如 {"zone":"cn-east"}' />
        </ElFormItem>
      </ElForm>
      <ElAlert type="info" :closable="false" show-icon>
        修改角色会改变 manager/worker 数量；drain 会把该节点上的任务迁移到其它节点。
      </ElAlert>
      <template #footer>
        <ElButton @click="editVisible = false">取消</ElButton>
        <ElButton type="primary" :loading="submitting" @click="submitEdit">保存</ElButton>
      </template>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, reactive, ref, watch } from 'vue'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import {
    fetchDockerNodes,
    fetchDockerSwarm,
    removeDockerNode,
    updateDockerNode
  } from '@/api/docker'
  import { useDockerEnv } from '@/hooks/core/useDockerEnv'
  import { formatSize } from '@/utils/web-basic'
  import DockerEnvBar from '../../components/DockerEnvBar.vue'

  defineOptions({ name: 'DockerSwarmNodes' })

  const { selectedEnvId } = useDockerEnv()
  const nodes = ref<Docker.Node[]>([])
  const state = ref<Docker.SwarmState>()
  const loading = ref(false)
  const editVisible = ref(false)
  const submitting = ref(false)
  const current = ref<Docker.Node>()
  const editForm = reactive({ role: 'worker', availability: 'active', labelsText: '{}' })

  onMounted(() => load())
  watch(selectedEnvId, load)

  async function load() {
    if (!selectedEnvId.value) {
      nodes.value = []
      return
    }
    loading.value = true
    try {
      try {
        state.value = await fetchDockerSwarm(selectedEnvId.value)
      } catch {
        state.value = undefined
      }
      nodes.value = state.value?.active ? await fetchDockerNodes(selectedEnvId.value) : []
    } finally {
      loading.value = false
    }
  }

  function labelText(labels: Record<string, string>) {
    return Object.entries(labels).map(([k, v]) => `${k}=${v}`).join(', ')
  }

  function openEdit(row: Docker.Node) {
    current.value = row
    editForm.role = row.role
    editForm.availability = row.availability
    editForm.labelsText = JSON.stringify(row.labels || {}, null, 2)
    editVisible.value = true
  }

  async function submitEdit() {
    if (!current.value) return
    let labels: Record<string, string> | undefined
    if (editForm.labelsText.trim()) {
      try {
        labels = JSON.parse(editForm.labelsText)
        if (typeof labels !== 'object' || Array.isArray(labels)) throw new Error('not object')
      } catch {
        ElMessage.error('节点标签必须是 JSON 对象')
        return
      }
    }
    submitting.value = true
    try {
      await updateDockerNode(selectedEnvId.value, current.value.id, {
        role: editForm.role,
        availability: editForm.availability,
        labels
      })
      editVisible.value = false
      await load()
    } finally {
      submitting.value = false
    }
  }

  async function remove(row: Docker.Node) {
    try {
      await ElMessageBox.confirm(
        `确定从集群移除节点「${row.hostname}」吗？建议先把节点 drain；强制移除可能导致任务异常。`,
        '移除节点',
        { type: 'warning', confirmButtonText: '移除', cancelButtonText: '取消' }
      )
    } catch {
      return
    }
    await removeDockerNode(selectedEnvId.value, row.id, true)
    await load()
  }
</script>