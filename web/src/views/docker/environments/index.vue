<!-- Docker 环境管理：本地 socket / TCP / TLS / SSH，多环境切换与连接测试 -->
<template>
  <div class="docker-page art-full-height">
    <ElCard class="art-table-card">
      <template #header>
        <div class="flex-cb">
          <div>
            <span class="font-semibold">Docker 环境</span>
            <span class="ml-2 text-xs text-g-500">连接信息与凭据加密存储在独立 SQLite</span>
          </div>
          <ElSpace>
            <ElButton v-auth="'docker:environment:create'" type="primary" @click="openDialog()">
              <ArtSvgIcon icon="ep:plus" class="mr-1" />新增环境
            </ElButton>
            <ElButton :loading="loading" @click="reload">
              <ArtSvgIcon icon="ep:refresh" class="mr-1" />刷新
            </ElButton>
          </ElSpace>
        </div>
      </template>

      <ElTable v-loading="loading" :data="environments" row-key="id" empty-text="还没有 Docker 环境">
        <ElTableColumn label="名称" min-width="180">
          <template #default="{ row }">
            <div class="flex items-center gap-2">
              <span class="font-medium">{{ row.name }}</span>
              <ElTag v-if="row.isDefault" size="small" type="primary" effect="plain">默认</ElTag>
            </div>
            <div v-if="row.description" class="mt-1 text-xs text-g-500">{{ row.description }}</div>
          </template>
        </ElTableColumn>
        <ElTableColumn prop="type" label="类型" width="100">
          <template #default="{ row }">
            <ElTag size="small" effect="plain">{{ typeLabel(row.type) }}</ElTag>
          </template>
        </ElTableColumn>
        <ElTableColumn label="地址" min-width="240" show-overflow-tooltip>
          <template #default="{ row }">
            <code class="text-xs">{{ row.type === 'local' ? row.socketPath : row.host }}</code>
            <span v-if="row.type === 'ssh'" class="ml-2 text-xs text-g-500">用户 {{ row.sshUser }}</span>
          </template>
        </ElTableColumn>
        <ElTableColumn label="状态" width="130">
          <template #default="{ row }">
            <ElTooltip :content="row.statusMessage || '连接正常'" :disabled="!row.statusMessage">
              <ElTag :type="row.status === 'online' ? 'success' : row.status === 'offline' ? 'danger' : 'info'" size="small">
                {{ row.status === 'online' ? '在线' : row.status === 'offline' ? '离线' : '未知' }}
              </ElTag>
            </ElTooltip>
            <ElTag v-if="row.swarmActive" class="ml-1" size="small" type="primary" effect="plain">Swarm</ElTag>
          </template>
        </ElTableColumn>
        <ElTableColumn label="我的权限" width="110">
          <template #default="{ row }">
            <ElTag size="small" :type="accessTag(row.accessLevel)" effect="plain">{{ accessText(row.accessLevel) }}</ElTag>
          </template>
        </ElTableColumn>
        <ElTableColumn prop="updatedAt" label="更新时间" width="170">
          <template #default="{ row }">{{ formatTime(row.updatedAt) }}</template>
        </ElTableColumn>
        <ElTableColumn label="操作" width="270" fixed="right">
          <template #default="{ row }">
            <ElButton v-auth="'docker:environment:test'" link type="primary" @click="testEnv(row)">测试</ElButton>
            <ElButton v-auth="'docker:environment:update'" link type="primary" @click="openDialog(row)">编辑</ElButton>
            <ElButton
              v-auth="'docker:environment:grant'"
              link
              type="primary"
              @click="goGrants(row)"
            >
              授权
            </ElButton>
            <ElButton v-auth="'docker:environment:delete'" link type="danger" @click="removeEnv(row)">删除</ElButton>
          </template>
        </ElTableColumn>
      </ElTable>

      <ElAlert class="mt-3" type="info" :closable="false" show-icon>
        <template #title>权限说明</template>
        环境列表只展示当前账号被授权的环境。功能权限（能否进入页面/点按钮）与环境资源授权（只读 / 可操作 / 管理）必须同时满足；
        非超级管理员创建环境后会自动获得该环境的管理权限。
      </ElAlert>
    </ElCard>

    <ElDialog
      v-model="dialogVisible"
      :title="editingId ? '编辑 Docker 环境' : '新增 Docker 环境'"
      width="720px"
      align-center
      destroy-on-close
    >
      <ElForm ref="formRef" :model="form" :rules="rules" label-width="110px">
        <ElFormItem label="环境名称" prop="name">
          <ElInput v-model.trim="form.name" maxlength="64" placeholder="如：本地 Docker / 生产 Swarm" />
        </ElFormItem>
        <ElFormItem label="连接方式" prop="type">
          <ElRadioGroup v-model="form.type" @change="onTypeChange">
            <ElRadio value="local">本地 Socket</ElRadio>
            <ElRadio value="tcp">远程 TCP</ElRadio>
            <ElRadio value="tls">远程 TLS</ElRadio>
            <ElRadio value="ssh">SSH</ElRadio>
          </ElRadioGroup>
        </ElFormItem>

        <ElFormItem v-if="form.type === 'local'" label="Socket 地址">
          <ElInput v-model.trim="form.socketPath" placeholder="留空自动探测（Windows npipe / Linux /var/run/docker.sock）" />
        </ElFormItem>

        <template v-if="form.type === 'tcp' || form.type === 'tls' || form.type === 'ssh'">
          <ElFormItem label="主机地址" prop="host">
            <ElInput
              v-model.trim="form.host"
              :placeholder="form.type === 'ssh' ? '192.168.1.10:22' : form.type === 'tls' ? '192.168.1.10:2376' : '192.168.1.10:2375'"
            />
          </ElFormItem>
        </template>

        <template v-if="form.type === 'tls'">
          <ElFormItem label="CA 证书">
            <ElInput v-model="form.tlsCa" type="textarea" :rows="3" placeholder="-----BEGIN CERTIFICATE-----" />
          </ElFormItem>
          <ElFormItem label="客户端证书">
            <ElInput v-model="form.tlsCert" type="textarea" :rows="3" placeholder="-----BEGIN CERTIFICATE-----" />
          </ElFormItem>
          <ElFormItem label="客户端私钥">
            <ElInput v-model="form.tlsKey" type="textarea" :rows="3" placeholder="编辑时留空表示保持原值" />
          </ElFormItem>
        </template>

        <template v-if="form.type === 'ssh'">
          <ElFormItem label="SSH 用户" prop="sshUser">
            <ElInput v-model.trim="form.sshUser" placeholder="root" />
          </ElFormItem>
          <ElFormItem label="SSH 密码">
            <ElInput v-model="form.sshPassword" type="password" show-password placeholder="编辑时留空表示保持原值" />
          </ElFormItem>
          <ElFormItem label="SSH 私钥">
            <ElInput
              v-model="form.sshKey"
              type="textarea"
              :rows="4"
              placeholder="-----BEGIN OPENSSH PRIVATE KEY-----（密码与私钥至少填一个）"
            />
          </ElFormItem>
          <ElFormItem label="远程 Socket">
            <ElInput v-model.trim="form.socketPath" placeholder="默认 /var/run/docker.sock" />
          </ElFormItem>
        </template>

        <ElFormItem label="备注">
          <ElInput v-model="form.description" type="textarea" :rows="2" maxlength="500" />
        </ElFormItem>
        <ElFormItem label="设为默认">
          <ElSwitch v-model="form.isDefault" />
        </ElFormItem>
        <ElFormItem v-if="editingId" label="凭据">
          <ElCheckbox v-model="form.clearCredentials">清空已保存的 TLS / SSH 凭据</ElCheckbox>
        </ElFormItem>
      </ElForm>
      <template #footer>
        <ElButton @click="dialogVisible = false">取消</ElButton>
        <ElButton type="primary" :loading="submitting" @click="submit">保存</ElButton>
      </template>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, reactive, ref } from 'vue'
  import { useRouter } from 'vue-router'
  import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
  import {
    createDockerEnvironment,
    deleteDockerEnvironment,
    testDockerEnvironment,
    updateDockerEnvironment
  } from '@/api/docker'
  import { useDockerEnv } from '@/hooks/core/useDockerEnv'
  import { formatTime } from '@/utils/web-basic'

  defineOptions({ name: 'DockerEnvironments' })

  const router = useRouter()
  const { environments, loading, loadEnvironments } = useDockerEnv()

  const dialogVisible = ref(false)
  const submitting = ref(false)
  const editingId = ref<string>()
  const formRef = ref<FormInstance>()

  const form = reactive({
    name: '',
    description: '',
    type: 'local',
    host: '',
    socketPath: '',
    tlsCa: '',
    tlsCert: '',
    tlsKey: '',
    sshUser: '',
    sshPassword: '',
    sshKey: '',
    isDefault: false,
    clearCredentials: false
  })

  const rules: FormRules = {
    name: [{ required: true, message: '请输入环境名称', trigger: 'blur' }],
    type: [{ required: true, message: '请选择连接方式', trigger: 'change' }],
    host: [{ required: true, message: '请输入主机地址', trigger: 'blur' }],
    sshUser: [{ required: true, message: '请输入 SSH 用户', trigger: 'blur' }]
  }

  onMounted(() => loadEnvironments(true))

  function reload() {
    loadEnvironments(true)
  }

  function typeLabel(type: string) {
    return { local: '本地 Socket', tcp: 'TCP', tls: 'TLS', ssh: 'SSH' }[type] || type
  }
  function accessText(level: number) {
    if (level >= 3) return '管理'
    if (level >= 2) return '操作'
    if (level >= 1) return '只读'
    return '无权限'
  }
  function accessTag(level: number) {
    if (level >= 3) return 'danger'
    if (level >= 2) return 'warning'
    return 'info'
  }

  function onTypeChange() {
    formRef.value?.clearValidate(['host', 'sshUser'])
    if (form.type === 'local') {
      form.host = ''
    }
  }

  function openDialog(row?: Docker.Environment) {
    editingId.value = row?.id
    formRef.value?.clearValidate()
    Object.assign(form, {
      name: row?.name ?? '',
      description: row?.description ?? '',
      type: row?.type ?? 'local',
      host: row?.host ?? '',
      socketPath: row?.socketPath ?? '',
      tlsCa: '',
      tlsCert: '',
      tlsKey: '',
      sshUser: row?.sshUser ?? '',
      sshPassword: '',
      sshKey: '',
      isDefault: row?.isDefault ?? false,
      clearCredentials: false
    })
    dialogVisible.value = true
  }

  async function submit() {
    if (!formRef.value) return
    const valid = await formRef.value.validate().catch(() => false)
    if (!valid) return
    if (form.type === 'tls' && !editingId.value && (!form.tlsCert || !form.tlsKey)) {
      ElMessage.warning('TLS 模式需要填写客户端证书与私钥')
      return
    }
    if (form.type === 'ssh' && !editingId.value && !form.sshPassword && !form.sshKey) {
      ElMessage.warning('SSH 模式需要填写密码或私钥')
      return
    }
    submitting.value = true
    try {
      if (editingId.value) {
        await updateDockerEnvironment(editingId.value, { ...form })
      } else {
        await createDockerEnvironment({ ...form })
      }
      dialogVisible.value = false
      await loadEnvironments(true)
    } finally {
      submitting.value = false
    }
  }

  async function testEnv(row: Docker.Environment) {
    const info = await testDockerEnvironment(row.id)
    ElMessage.success(
      `连接成功：Docker ${info.serverVersion}，容器 ${info.running}/${info.containers} 运行中`
    )
    await loadEnvironments(true)
  }

  async function removeEnv(row: Docker.Environment) {
    try {
      await ElMessageBox.confirm(
        `删除环境「${row.name}」会同时删除该环境下保存的所有 Stack 定义与资源授权，但不会删除 Docker 主机上的容器/服务。确定继续吗？`,
        '删除环境',
        { type: 'warning', confirmButtonText: '确定删除', cancelButtonText: '取消' }
      )
    } catch {
      return
    }
    await deleteDockerEnvironment(row.id)
    await loadEnvironments(true)
  }

  function goGrants(row: Docker.Environment) {
    router.push({ path: '/docker/grants', query: { envId: row.id } })
  }
</script>