<!-- Swarm 密钥：只写不读，创建后 Docker 不再返回明文 -->
<template>
  <div class="docker-page art-full-height">
    <DockerEnvBar>
      <ElButton v-auth="'docker:secret:create'" type="primary" @click="openCreate">
        <ArtSvgIcon icon="ep:plus" class="mr-1" />创建密钥
      </ElButton>
      <ElButton :loading="loading" @click="load">
        <ArtSvgIcon icon="ep:refresh" class="mr-1" />刷新
      </ElButton>
    </DockerEnvBar>

    <ElAlert v-if="!state?.active" class="mb-3" type="warning" :closable="false" show-icon>
      当前环境未启用 Swarm。密钥仅在 Swarm 管理节点可用。
    </ElAlert>

    <ElCard class="art-table-card">
      <ElTable v-loading="loading" :data="secrets" row-key="id" empty-text="没有密钥">
        <ElTableColumn label="名称" min-width="220">
          <template #default="{ row }">
            <div class="flex items-center gap-2">
              <span class="font-medium">{{ row.name }}</span>
              <ElTag v-if="row.stackName" size="small" type="primary" effect="plain">{{ row.stackName }}</ElTag>
            </div>
            <div class="mt-1 text-xs text-g-500"><code>{{ row.id.slice(0, 12) }}</code></div>
          </template>
        </ElTableColumn>
        <ElTableColumn label="标签" min-width="180">
          <template #default="{ row }">{{ labelText(row.labels) }}</template>
        </ElTableColumn>
        <ElTableColumn label="创建时间" width="170">
          <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
        </ElTableColumn>
        <ElTableColumn label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <ElButton v-auth="'docker:secret:delete'" link type="danger" @click="remove(row)">删除</ElButton>
          </template>
        </ElTableColumn>
      </ElTable>

      <ElAlert class="mt-3" type="info" :closable="false" show-icon>
        <template #title>为什么看不到内容？</template>
        Docker 的密钥接口不返回明文（安全设计）。需要更新时请创建新密钥并重新部署服务；Stack 使用 external 密钥时，
        请先在<strong>当前页面</strong>创建同名密钥。
      </ElAlert>
    </ElCard>

    <ElDialog v-model="createVisible" title="创建 Swarm 密钥" width="640px" align-center>
      <ElForm ref="formRef" :model="form" :rules="rules" label-width="90px">
        <ElFormItem label="名称" prop="name">
          <ElInput v-model.trim="form.name" placeholder="如 db_password" />
        </ElFormItem>
        <ElFormItem label="内容" prop="data">
          <ElInput v-model="form.data" type="textarea" :rows="6" class="font-mono" placeholder="密钥明文（不会被保存到 SQLite）" />
        </ElFormItem>
      </ElForm>
      <ElAlert type="warning" :closable="false" show-icon>
        密钥内容只提交给 Docker Engine，本系统的 SQLite 不保存明文；创建后无法再次查看。
      </ElAlert>
      <template #footer>
        <ElButton @click="createVisible = false">取消</ElButton>
        <ElButton type="primary" :loading="submitting" @click="submit">创建</ElButton>
      </template>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, reactive, ref, watch } from 'vue'
  import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
  import {
    createDockerSecret,
    fetchDockerSecrets,
    fetchDockerSwarm,
    removeDockerSecret
  } from '@/api/docker'
  import { useDockerEnv } from '@/hooks/core/useDockerEnv'
  import { formatTime } from '@/utils/web-basic'
  import DockerEnvBar from '../../components/DockerEnvBar.vue'

  defineOptions({ name: 'DockerSwarmSecrets' })

  const { selectedEnvId } = useDockerEnv()
  const secrets = ref<Docker.Secret[]>([])
  const state = ref<Docker.SwarmState>()
  const loading = ref(false)

  const createVisible = ref(false)
  const submitting = ref(false)
  const formRef = ref<FormInstance>()
  const rules: FormRules = {
    name: [{ required: true, message: '请输入密钥名称', trigger: 'blur' }],
    data: [{ required: true, message: '请输入密钥内容', trigger: 'blur' }]
  }
  const form = reactive({ name: '', data: '' })

  onMounted(() => load())
  watch(selectedEnvId, load)

  async function load() {
    if (!selectedEnvId.value) {
      secrets.value = []
      return
    }
    loading.value = true
    try {
      try {
        state.value = await fetchDockerSwarm(selectedEnvId.value)
      } catch {
        state.value = undefined
      }
      secrets.value = state.value?.active ? await fetchDockerSecrets(selectedEnvId.value) : []
    } finally {
      loading.value = false
    }
  }

  function labelText(labels: Record<string, string>) {
    if (!labels || !Object.keys(labels).length) return '-'
    return Object.entries(labels).map(([k, v]) => `${k}=${v}`).join(', ')
  }

  function openCreate() {
    formRef.value?.resetFields()
    form.name = ''
    form.data = ''
    createVisible.value = true
  }

  async function submit() {
    if (!formRef.value) return
    if (!(await formRef.value.validate().catch(() => false))) return
    submitting.value = true
    try {
      await createDockerSecret(selectedEnvId.value, { ...form })
      createVisible.value = false
      await load()
    } finally {
      submitting.value = false
    }
  }

  async function remove(row: Docker.Secret) {
    try {
      await ElMessageBox.confirm(`确定删除密钥「${row.name}」吗？使用它的服务可能无法更新。`, '删除密钥', {
        type: 'warning',
        confirmButtonText: '删除',
        cancelButtonText: '取消'
      })
    } catch {
      return
    }
    await removeDockerSecret(selectedEnvId.value, row.id)
    await load()
  }
</script>