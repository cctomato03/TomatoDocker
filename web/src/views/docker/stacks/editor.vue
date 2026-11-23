<!-- Stack 编辑器：自定义 compose / swarm 文件，保存到 SQLite 并部署 -->
<template>
  <div class="docker-stack-editor art-full-height">
    <ElCard class="mb-3">
      <div class="flex flex-wrap items-center gap-3">
        <ElButton text @click="goBack">
          <ArtSvgIcon icon="ep:arrow-left" class="mr-1" />返回
        </ElButton>
        <span class="font-semibold">{{ stackId ? '编辑 Stack' : '新建 Stack' }}</span>
        <ElTag v-if="stackId" size="small" effect="plain">v{{ form.version || 1 }}</ElTag>
        <ElTag v-if="detail?.pendingChanges" size="small" type="warning">有未部署变更</ElTag>

        <div class="ml-auto flex flex-wrap items-center gap-2">
          <ElButton :loading="saving" @click="save(false)">保存</ElButton>
          <ElButton type="primary" :loading="deploying" @click="save(true)">
            {{ stackId ? '保存并部署' : '创建并部署' }}
          </ElButton>
          <ElButton v-if="stackId" :loading="deploying" @click="deployCurrent">仅部署</ElButton>
        </div>
      </div>

      <ElForm class="mt-3" label-width="90px">
        <ElRow :gutter="12">
          <ElCol :xs="24" :md="8">
            <ElFormItem label="Stack 名称">
              <ElInput
                v-model.trim="form.name"
                :disabled="!!stackId"
                maxlength="64"
                placeholder="小写字母/数字/-/_，如 my-app"
              />
            </ElFormItem>
          </ElCol>
          <ElCol :xs="24" :md="6">
            <ElFormItem label="部署类型">
              <ElSelect v-model="form.type" class="w-full">
                <ElOption label="Docker Compose（单机）" value="compose" />
                <ElOption label="Docker Swarm Stack" value="swarm" />
              </ElSelect>
            </ElFormItem>
          </ElCol>
          <ElCol :xs="24" :md="10">
            <ElFormItem label="目标环境">
              <ElSelect
                v-model="selectedEnvId"
                :disabled="!!stackId"
                filterable
                placeholder="选择 Docker 环境"
                class="w-full"
              >
                <ElOption v-for="env in environments" :key="env.id" :label="env.name" :value="env.id" />
              </ElSelect>
            </ElFormItem>
          </ElCol>
        </ElRow>
      </ElForm>

      <ElAlert type="info" :closable="false" show-icon>
        <template #title>编写提示</template>
        绑定挂载必须使用宿主机绝对路径（如 <code>/data/app</code>）；不支持 <code>build</code>（请先构建镜像）；
        Swarm 模式暂只支持 <code>external: true</code> 的 secret/config（先在 Swarm 页面创建）。
      </ElAlert>
    </ElCard>

    <ElCard class="editor-card">
      <ElTabs v-model="activeTab">
        <ElTabPane label="docker-compose.yml" name="compose">
          <DockerCodeEditor v-model="form.composeContent" height="calc(100vh - 420px)" />
        </ElTabPane>
        <ElTabPane label=".env（变量插值）" name="env">
          <DockerCodeEditor v-model="form.envContent" language="bash" height="calc(100vh - 420px)" />
        </ElTabPane>
        <ElTabPane v-if="stackId" label="历史版本" name="history">
          <ElTable :data="detail?.revisions ?? []" size="small" empty-text="暂无历史版本">
            <ElTableColumn prop="version" label="版本" width="80">
              <template #default="{ row }">v{{ row.version }}</template>
            </ElTableColumn>
            <ElTableColumn prop="message" label="说明" min-width="220" />
            <ElTableColumn prop="createdBy" label="操作人" width="110" />
            <ElTableColumn label="时间" width="180">
              <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
            </ElTableColumn>
            <ElTableColumn label="操作" width="240">
              <template #default="{ row }">
                <ElButton link type="primary" @click="loadRevision(row)">载入编辑器</ElButton>
                <ElButton v-auth="'docker:stack:deploy'" link type="primary" @click="rollback(row.version, true)">
                  回滚并部署
                </ElButton>
              </template>
            </ElTableColumn>
          </ElTable>
        </ElTabPane>
        <ElTabPane v-if="stackId" label="操作记录" name="activities">
          <ElTable :data="detail?.activities ?? []" size="small" empty-text="暂无操作记录">
            <ElTableColumn prop="action" label="操作" width="100" />
            <ElTableColumn label="结果" width="90">
              <template #default="{ row }">
                <ElTag size="small" :type="row.status === 'success' ? 'success' : 'danger'">
                  {{ row.status === 'success' ? '成功' : '失败' }}
                </ElTag>
              </template>
            </ElTableColumn>
            <ElTableColumn prop="message" label="说明" min-width="260" show-overflow-tooltip />
            <ElTableColumn prop="operator" label="操作人" width="110" />
            <ElTableColumn label="时间" width="180">
              <template #default="{ row }">{{ formatTime(row.createdAt) }}</template>
            </ElTableColumn>
          </ElTable>
        </ElTabPane>
      </ElTabs>
    </ElCard>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, reactive, ref, watch } from 'vue'
  import { useRoute, useRouter } from 'vue-router'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import {
    createDockerStack,
    deployDockerStack,
    fetchDockerStack,
    rollbackDockerStack,
    updateDockerStack
  } from '@/api/docker'
  import { useDockerEnv } from '@/hooks/core/useDockerEnv'
  import { formatTime } from '@/utils/web-basic'
  import DockerCodeEditor from '../components/DockerCodeEditor.vue'

  defineOptions({ name: 'DockerStackEditor' })

  const route = useRoute()
  const router = useRouter()
  const { environments, selectedEnvId, selectedEnv, loadEnvironments, persist } = useDockerEnv()

  const stackId = ref<string>()
  const detail = ref<Docker.StackDetail>()
  const saving = ref(false)
  const deploying = ref(false)
  const activeTab = ref('compose')

  const form = reactive({
    name: '',
    type: 'compose',
    composeContent: defaultCompose(),
    envContent: '',
    version: 0
  })

  const nameRe = /^[a-z0-9][a-z0-9_-]{0,63}$/

  function defaultCompose() {
    return `services:
  web:
    image: nginx:alpine
    restart: unless-stopped
    ports:
      - "8080:80"
    environment:
      TZ: Asia/Shanghai
`
  }

  onMounted(async () => {
    await loadEnvironments()
    if (typeof route.query.envId === 'string' && route.query.envId) {
      persist(route.query.envId)
    }
    if (typeof route.query.id === 'string' && route.query.id) {
      stackId.value = route.query.id
      await loadDetail()
    } else {
      form.type = selectedEnv.value?.swarmActive ? 'swarm' : 'compose'
    }
  })

  watch(selectedEnvId, (id) => {
    if (!stackId.value && id) {
      form.type = selectedEnv.value?.swarmActive ? 'swarm' : 'compose'
    }
  })

  async function loadDetail() {
    if (!stackId.value) return
    const data = await fetchDockerStack(stackId.value)
    detail.value = data
    Object.assign(form, {
      name: data.name,
      type: data.type,
      composeContent: data.composeContent,
      envContent: data.envContent,
      version: data.version
    })
  }

  async function save(deployNow: boolean) {
    if (!selectedEnvId.value) {
      ElMessage.warning('请先选择 Docker 环境')
      return
    }
    if (!stackId.value && !nameRe.test(form.name)) {
      ElMessage.warning('Stack 名称只能使用小写字母、数字、- 和 _，并以字母/数字开头')
      return
    }
    if (!form.composeContent.trim()) {
      ElMessage.warning('compose 文件内容不能为空')
      return
    }
    saving.value = true
    try {
      if (!stackId.value) {
        const res = await createDockerStack({
          environmentId: selectedEnvId.value,
          name: form.name,
          type: form.type,
          composeContent: form.composeContent,
          envContent: form.envContent
        })
        stackId.value = res.id
        await router.replace({ path: '/docker/stacks/editor', query: { id: res.id } })
        detail.value = await fetchDockerStack(res.id)
        form.version = detail.value.version
        ElMessage.success('Stack 已创建')
      } else {
        await updateDockerStack(stackId.value, {
          type: form.type,
          composeContent: form.composeContent,
          envContent: form.envContent
        })
        await loadDetail()
      }
      if (deployNow) {
        await deployCurrent()
      }
    } finally {
      saving.value = false
    }
  }

  async function deployCurrent() {
    if (!stackId.value) return
    deploying.value = true
    try {
      await deployDockerStack(stackId.value, false)
      await loadDetail()
      activeTab.value = 'compose'
    } finally {
      deploying.value = false
    }
  }

  function loadRevision(row: Docker.Revision) {
    form.composeContent = row.composeContent
    form.envContent = row.envContent
    activeTab.value = 'compose'
    ElMessage.info(`已载入 v${row.version} 内容，点击「保存」生成新版本`)
  }

  async function rollback(version: number, deployNow: boolean) {
    if (!stackId.value) return
    try {
      await ElMessageBox.confirm(`确定回滚到 v${version} 吗？${deployNow ? '将立即重新部署。' : ''}`, '回滚 Stack', {
        type: 'warning',
        confirmButtonText: '回滚',
        cancelButtonText: '取消'
      })
    } catch {
      return
    }
    await rollbackDockerStack(stackId.value, version, deployNow)
    await loadDetail()
  }

  function goBack() {
    router.push('/docker/stacks')
  }
</script>

<style scoped>
.editor-card :deep(.el-card__body) {
  padding-top: 6px;
}
</style>