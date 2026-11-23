<!-- 镜像管理：拉取（流式进度）、删除、详情与悬空镜像清理 -->
<template>
  <div class="docker-page art-full-height">
    <DockerEnvBar>
      <ElInput v-model="keyword" placeholder="搜索镜像 / TAG" clearable class="!w-56" @keyup.enter="load" @clear="load" />
      <ElCheckbox v-model="all" @change="load">包含中间层镜像</ElCheckbox>
      <ElButton v-auth="'docker:image:pull'" type="primary" @click="openPull">
        <ArtSvgIcon icon="ep:download" class="mr-1" />拉取镜像
      </ElButton>
      <ElButton :loading="loading" @click="load">
        <ArtSvgIcon icon="ep:refresh" class="mr-1" />刷新
      </ElButton>
      <ElButton v-auth="'docker:image:delete'" @click="prune">清理悬空镜像</ElButton>
    </DockerEnvBar>

    <ElCard class="art-table-card">
      <ElTable v-loading="loading" :data="images" row-key="id" empty-text="当前环境没有镜像">
        <ElTableColumn label="镜像" min-width="320">
          <template #default="{ row }">
            <div class="flex flex-col gap-1">
              <div v-if="row.repoTags?.length">
                <ElTag v-for="tag in row.repoTags" :key="tag" size="small" effect="plain" class="mr-1">
                  {{ tag }}
                </ElTag>
              </div>
              <span v-else class="text-xs text-g-500">&lt;none&gt;</span>
              <code class="text-xs text-g-500">{{ row.shortId }}</code>
            </div>
          </template>
        </ElTableColumn>
        <ElTableColumn label="大小" width="110">
          <template #default="{ row }">{{ formatSize(row.size) }}</template>
        </ElTableColumn>
        <ElTableColumn label="容器数" width="90" prop="containers" />
        <ElTableColumn label="创建时间" width="170">
          <template #default="{ row }">{{ formatUnix(row.created) }}</template>
        </ElTableColumn>
        <ElTableColumn label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <ElButton link type="primary" @click="inspect(row)">详情</ElButton>
            <ElButton v-auth="'docker:image:delete'" link type="danger" @click="remove(row)">删除</ElButton>
          </template>
        </ElTableColumn>
      </ElTable>
    </ElCard>

    <ElDialog v-model="pullVisible" title="拉取镜像" width="640px" align-center @closed="abortPull">
      <ElForm label-width="90px">
        <ElFormItem label="镜像名称">
          <ElInput v-model.trim="pullRef" placeholder="nginx:alpine / registry.example.com/app:v1" @keyup.enter="startPull" />
        </ElFormItem>
      </ElForm>
      <ElAlert type="info" :closable="false" show-icon class="mb-3">
        私有仓库需要目标 Docker 主机已登录；拉取进度会实时显示在下方。大镜像可能需要数分钟。
      </ElAlert>
      <pre class="docker-log-view max-h-[36vh]">{{ pullLog || '等待开始...' }}</pre>
      <template #footer>
        <ElButton @click="pullVisible = false" :disabled="pulling">关闭</ElButton>
        <ElButton type="primary" :loading="pulling" @click="startPull">开始拉取</ElButton>
      </template>
    </ElDialog>

    <ElDialog v-model="inspectVisible" title="镜像详情（Docker inspect）" width="860px" align-center>
      <pre class="docker-log-view max-h-[65vh]">{{ inspectText }}</pre>
    </ElDialog>
  </div>
</template>

<script setup lang="ts">
  import { onMounted, ref, watch } from 'vue'
  import { ElMessage, ElMessageBox } from 'element-plus'
  import {
    fetchDockerImageInspect,
    fetchDockerImages,
    pruneDockerImages,
    pullDockerImage,
    removeDockerImage
  } from '@/api/docker'
  import { useDockerEnv } from '@/hooks/core/useDockerEnv'
  import { formatSize, formatUnix } from '@/utils/web-basic'
  import DockerEnvBar from '../components/DockerEnvBar.vue'

  defineOptions({ name: 'DockerImages' })

  const { selectedEnvId } = useDockerEnv()
  const images = ref<Docker.Image[]>([])
  const loading = ref(false)
  const keyword = ref('')
  const all = ref(false)

  const pullVisible = ref(false)
  const pullRef = ref('nginx:alpine')
  const pullLog = ref('')
  const pulling = ref(false)
  let abortController: AbortController | null = null

  const inspectVisible = ref(false)
  const inspectText = ref('')

  onMounted(() => load())
  watch(selectedEnvId, load)

  async function load() {
    if (!selectedEnvId.value) {
      images.value = []
      return
    }
    loading.value = true
    try {
      images.value = await fetchDockerImages(selectedEnvId.value, {
        all: all.value,
        keyword: keyword.value || undefined
      })
    } finally {
      loading.value = false
    }
  }

  function openPull() {
    pullLog.value = ''
    pullVisible.value = true
  }

  function abortPull() {
    abortController?.abort()
    abortController = null
    pulling.value = false
  }

  async function startPull() {
    if (!pullRef.value.trim()) {
      ElMessage.warning('请输入镜像名称')
      return
    }
    pulling.value = true
    pullLog.value = ''
    abortController = new AbortController()
    try {
      await pullDockerImage(
        selectedEnvId.value,
        pullRef.value.trim(),
        (line) => {
          pullLog.value = (pullLog.value ? pullLog.value + '\n' : '') + line
        },
        abortController.signal
      )
      ElMessage.success('镜像拉取完成')
      await load()
    } catch (error) {
      if (error instanceof DOMException && error.name === 'AbortError') {
        ElMessage.info('已取消拉取')
      } else {
        ElMessage.error(error instanceof Error ? error.message : '拉取失败')
      }
    } finally {
      pulling.value = false
      abortController = null
    }
  }

  async function remove(row: Docker.Image) {
    const label = row.repoTags?.[0] || row.shortId
    try {
      await ElMessageBox.confirm(`确定删除镜像「${label}」吗？被容器使用时需要先删除容器。`, '删除镜像', {
        type: 'warning',
        confirmButtonText: '删除',
        cancelButtonText: '取消'
      })
    } catch {
      return
    }
    await removeDockerImage(selectedEnvId.value, row.id, true)
    await load()
  }

  async function prune() {
    try {
      await ElMessageBox.confirm('将清理所有没有 TAG 的悬空镜像，确定继续吗？', '清理镜像', {
        type: 'warning',
        confirmButtonText: '清理',
        cancelButtonText: '取消'
      })
    } catch {
      return
    }
    await pruneDockerImages(selectedEnvId.value)
    await load()
  }

  async function inspect(row: Docker.Image) {
    const data = await fetchDockerImageInspect(selectedEnvId.value, row.id)
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