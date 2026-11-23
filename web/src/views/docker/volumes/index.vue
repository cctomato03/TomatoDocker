<!-- 数据卷管理：创建、详情、删除与清理 -->
<template>
  <div class="docker-page art-full-height">
    <DockerEnvBar>
      <ElInput v-model="keyword" placeholder="搜索数据卷名称" clearable class="!w-56" @keyup.enter="load" @clear="load" />
      <ElButton v-auth="'docker:volume:create'" type="primary" @click="openCreate">
        <ArtSvgIcon icon="ep:plus" class="mr-1" />创建数据卷
      </ElButton>
      <ElButton :loading="loading" @click="load">
        <ArtSvgIcon icon="ep:refresh" class="mr-1" />刷新
      </ElButton>
      <ElButton v-auth="'docker:volume:delete'" @click="prune">清理未使用</ElButton>
    </DockerEnvBar>

    <ElCard class="art-table-card">
      <ElTable v-loading="loading" :data="volumes" row-key="name" empty-text="当前环境没有数据卷">
        <ElTableColumn label="名称" min-width="220">
          <template #default="{ row }">
            <span class="font-medium">{{ row.name }}</span>
            <ElTag v-if="row.inUse" class="ml-2" size="small" type="success" effect="plain">使用中</ElTag>
            <ElTag v-if="row.stackName" class="ml-2" size="small" type="primary" effect="plain">{{ row.stackName }}</ElTag>
          </template>
        </ElTableColumn>
        <ElTableColumn prop="driver" label="驱动" width="100" />
        <ElTableColumn prop="scope" label="范围" width="90" />
        <ElTableColumn label="挂载点" min-width="280" show-overflow-tooltip>
          <template #default="{ row }"><code class="text-xs">{{ row.mountpoint }}</code></template>
        </ElTableColumn>
        <ElTableColumn label="创建时间" width="170">
          <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
        </ElTableColumn>
        <ElTableColumn label="操作" width="170" fixed="right">
          <template #default="{ row }">
            <ElButton link type="primary" @click="inspect(row)">详情</ElButton>
            <ElButton v-auth="'docker:volume:delete'" link type="danger" @click="remove(row)">删除</ElButton>
          </template>
        </ElTableColumn>
      </ElTable>
    </ElCard>

    <ElDialog v-model="createVisible" title="创建数据卷" width="560px" align-center destroy-on-close>
      <ElForm ref="formRef" :model="form" :rules="rules" label-width="100px">
        <ElFormItem label="名称" prop="name">
          <ElInput v-model.trim="form.name" placeholder="my-data" />
        </ElFormItem>
        <ElFormItem label="驱动">
          <ElInput v-model.trim="form.driver" placeholder="留空使用 local" />
        </ElFormItem>
        <ElFormItem label="驱动参数">
          <ElInput v-model="driverOptsText" type="textarea" :rows="2" placeholder='每行 key=value，如 type=nfs' />
        </ElFormItem>
      </ElForm>
      <template #footer>
        <ElButton @click="createVisible = false">取消</ElButton>
        <ElButton type="primary" :loading="submitting" @click="submit">创建</ElButton>
      </template>
    </ElDialog>

    <ElDialog v-model="inspectVisible" title="数据卷详情（Docker inspect）" width="760px" align-center>
      <pre class="docker-log-view max-h-[60vh]">{{ inspectText }}</pre>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, reactive, ref, watch } from 'vue'
  import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
  import {
    createDockerVolume,
    fetchDockerVolumeInspect,
    fetchDockerVolumes,
    pruneDockerVolumes,
    removeDockerVolume
  } from '@/api/docker'
  import { useDockerEnv } from '@/hooks/core/useDockerEnv'
  import { formatTime } from '@/utils/web-basic'
  import DockerEnvBar from '../components/DockerEnvBar.vue'

  defineOptions({ name: 'DockerVolumes' })

  const { selectedEnvId } = useDockerEnv()
  const volumes = ref<Docker.Volume[]>([])
  const loading = ref(false)
  const keyword = ref('')

  const createVisible = ref(false)
  const submitting = ref(false)
  const formRef = ref<FormInstance>()
  const rules: FormRules = { name: [{ required: true, message: '请输入数据卷名称', trigger: 'blur' }] }
  const form = reactive({ name: '', driver: '' })
  const driverOptsText = ref('')

  const inspectVisible = ref(false)
  const inspectText = ref('')

  onMounted(() => load())
  watch(selectedEnvId, load)

  async function load() {
    if (!selectedEnvId.value) {
      volumes.value = []
      return
    }
    loading.value = true
    try {
      volumes.value = await fetchDockerVolumes(selectedEnvId.value, { keyword: keyword.value || undefined })
    } finally {
      loading.value = false
    }
  }

  function openCreate() {
    form.name = ''
    form.driver = ''
    driverOptsText.value = ''
    createVisible.value = true
  }

  async function submit() {
    if (!formRef.value) return
    if (!(await formRef.value.validate().catch(() => false))) return
    const driverOpts: Record<string, string> = {}
    for (const line of driverOptsText.value.split('\n')) {
      const idx = line.indexOf('=')
      if (idx > 0) driverOpts[line.slice(0, idx).trim()] = line.slice(idx + 1).trim()
    }
    submitting.value = true
    try {
      await createDockerVolume(selectedEnvId.value, {
        name: form.name,
        driver: form.driver || undefined,
        driverOpts
      })
      createVisible.value = false
      await load()
    } finally {
      submitting.value = false
    }
  }

  async function remove(row: Docker.Volume) {
    try {
      await ElMessageBox.confirm(
        `确定删除数据卷「${row.name}」吗？${row.inUse ? '该卷正在被容器使用，删除可能失败。' : ''}`,
        '删除数据卷',
        { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' }
      )
    } catch {
      return
    }
    await removeDockerVolume(selectedEnvId.value, row.name, false)
    await load()
  }

  async function prune() {
    try {
      await ElMessageBox.confirm('将清理所有未被容器使用的数据卷，数据不可恢复，确定继续吗？', '清理数据卷', {
        type: 'warning',
        confirmButtonText: '清理',
        cancelButtonText: '取消'
      })
    } catch {
      return
    }
    await pruneDockerVolumes(selectedEnvId.value)
    await load()
  }

  async function inspect(row: Docker.Volume) {
    const data = await fetchDockerVolumeInspect(selectedEnvId.value, row.name)
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