<!-- 资源授权：按角色/用户授予 Docker 环境的只读/操作/管理权限 -->
<template>
  <div class="docker-page art-full-height">
    <DockerEnvBar>
      <ElButton :loading="loading" @click="load">
        <ArtSvgIcon icon="ep:refresh" class="mr-1" />刷新
      </ElButton>
    </DockerEnvBar>

    <ElEmpty v-if="!selectedEnvId" description="请先选择一个 Docker 环境" />

    <ElCard v-else>
      <template #header>
        <div class="flex-cb">
          <div>
            <span class="font-semibold">「{{ selectedEnv?.name }}」资源授权</span>
            <span class="ml-2 text-xs text-g-500">只读=查看资源；操作=启停/部署；管理=改环境与授权</span>
          </div>
          <ElSpace>
            <ElButton @click="addRow">添加授权</ElButton>
            <ElButton
              v-auth="'docker:environment:grant'"
              type="primary"
              :loading="saving"
              @click="save"
            >
              保存授权
            </ElButton>
          </ElSpace>
        </div>
      </template>

      <ElTable :data="rows" row-key="key" empty-text="还未授权任何角色或用户">
        <ElTableColumn label="授权对象" min-width="280">
          <template #default="{ row }">
            <ElSelect
              v-model="row.key"
              filterable
              placeholder="选择角色或用户"
              class="w-full"
              @change="onSubjectChange(row)"
            >
              <ElOptionGroup label="角色">
                <ElOption
                  v-for="opt in roleOptions"
                  :key="`role:${opt.subjectId}`"
                  :label="`${opt.name}（${opt.detail}）`"
                  :value="`role:${opt.subjectId}`"
                />
              </ElOptionGroup>
              <ElOptionGroup label="用户">
                <ElOption
                  v-for="opt in userOptions"
                  :key="`user:${opt.subjectId}`"
                  :label="`${opt.name}${opt.detail ? '（' + opt.detail + '）' : ''}`"
                  :value="`user:${opt.subjectId}`"
                />
              </ElOptionGroup>
            </ElSelect>
          </template>
        </ElTableColumn>
        <ElTableColumn label="访问级别" width="220">
          <template #default="{ row }">
            <ElSelect v-model="row.accessLevel" class="w-full">
              <ElOption label="只读（查看资源）" :value="1" />
              <ElOption label="操作（启停 / 部署）" :value="2" />
              <ElOption label="管理（环境与授权）" :value="3" />
            </ElSelect>
          </template>
        </ElTableColumn>
        <ElTableColumn label="类型" width="100">
          <template #default="{ row }">
            <ElTag size="small" effect="plain">{{ row.key.startsWith('role:') ? '角色' : '用户' }}</ElTag>
          </template>
        </ElTableColumn>
        <ElTableColumn label="操作" width="90">
          <template #default="{ $index }">
            <ElButton link type="danger" @click="rows.splice($index, 1)">移除</ElButton>
          </template>
        </ElTableColumn>
      </ElTable>

      <ElAlert class="mt-3" type="warning" :closable="false" show-icon>
        <template #title>注意</template>
        超级管理员默认拥有全部环境权限，不在此列表中显示；普通用户需要在「角色管理」中先获得 Docker 功能权限，
        再由这里授予具体环境，缺一不可。删除自己的管理授权会导致无法再进入该环境。
      </ElAlert>
    </ElCard>
  </div>
</template>

<script setup lang="ts">
  import { computed, onMounted, ref, watch } from 'vue'
  import { useRoute } from 'vue-router'
  import { ElMessage } from 'element-plus'
  import {
    fetchDockerGrantOptions,
    fetchDockerGrants,
    setDockerGrants
  } from '@/api/docker'
  import { useDockerEnv } from '@/hooks/core/useDockerEnv'
  import DockerEnvBar from '../components/DockerEnvBar.vue'

  defineOptions({ name: 'DockerGrants' })

  interface Row {
    key: string
    accessLevel: number
  }

  const route = useRoute()
  const { selectedEnvId, selectedEnv, persist } = useDockerEnv()

  const options = ref<Docker.GrantOption[]>([])
  const rows = ref<Row[]>([])
  const loading = ref(false)
  const saving = ref(false)

  const roleOptions = computed(() => options.value.filter((o) => o.subjectType === 'role'))
  const userOptions = computed(() => options.value.filter((o) => o.subjectType === 'user'))

  onMounted(async () => {
    if (typeof route.query.envId === 'string' && route.query.envId) {
      persist(route.query.envId)
    }
    await loadOptions()
  })

  watch(selectedEnvId, load, { immediate: true })

  async function loadOptions() {
    try {
      options.value = await fetchDockerGrantOptions()
    } catch {
      options.value = []
    }
  }

  async function load() {
    if (!selectedEnvId.value) {
      rows.value = []
      return
    }
    loading.value = true
    try {
      const grants = await fetchDockerGrants(selectedEnvId.value)
      rows.value = grants.map((g) => ({
        key: `${g.subjectType}:${g.subjectId}`,
        accessLevel: g.accessLevel
      }))
    } finally {
      loading.value = false
    }
  }

  function addRow() {
    rows.value.push({ key: '', accessLevel: 1 })
  }

  function onSubjectChange(row: Row) {
    if (rows.value.filter((item) => item.key === row.key && row.key).length > 1) {
      ElMessage.warning('该对象已经授权，请勿重复添加')
    }
  }

  async function save() {
    if (!selectedEnvId.value) return
    const grants = rows.value
      .filter((row) => row.key)
      .map((row) => {
        const [subjectType, subjectId] = row.key.split(':')
        return { subjectType, subjectId, accessLevel: row.accessLevel }
      })
    const unique = new Set(grants.map((g) => `${g.subjectType}:${g.subjectId}`))
    if (unique.size !== grants.length) {
      ElMessage.warning('存在重复的授权对象')
      return
    }
    saving.value = true
    try {
      await setDockerGrants(selectedEnvId.value, grants)
      await load()
    } finally {
      saving.value = false
    }
  }
</script>