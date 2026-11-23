/**
 * Docker 管理接口（/api/v1/docker/**）
 *
 * 约定：
 * - 环境（environment）是资源作用域，所有资源接口都带 envId；
 * - 后端会同时校验「功能权限」与「环境资源授权」，403 文案会说明缺哪一级；
 * - 镜像拉取使用 NDJSON 流式进度，单独用 fetch 实现。
 */
import request, { apiUrl } from '@/utils/http'
import { useUserStore } from '@/store/modules/user'

/* ------------------------------ 环境 ------------------------------ */

export function fetchDockerEnvironments() {
  return request.get<Docker.Environment[]>({ url: '/api/v1/docker/environments' })
}

export function fetchDockerEnvironment(id: string) {
  return request.get<Docker.Environment>({ url: `/api/v1/docker/environments/${id}` })
}

export function createDockerEnvironment(data: Docker.EnvironmentReq) {
  return request.post<{ id: string }>({
    url: '/api/v1/docker/environments',
    data,
    successMessage: '环境已创建',
    showSuccessMessage: true
  })
}

export function updateDockerEnvironment(id: string, data: Docker.EnvironmentReq) {
  return request.put({
    url: `/api/v1/docker/environments/${id}`,
    data,
    successMessage: '环境已保存',
    showSuccessMessage: true
  })
}

export function deleteDockerEnvironment(id: string) {
  return request.del({
    url: `/api/v1/docker/environments/${id}`,
    successMessage: '环境已删除',
    showSuccessMessage: true
  })
}

export function testDockerEnvironment(id: string) {
  return request.post<Docker.Info>({
    url: `/api/v1/docker/environments/${id}/test`,
    successMessage: '连接成功',
    showSuccessMessage: true
  })
}

/* ------------------------------ 资源授权 ------------------------------ */

export function fetchDockerGrantOptions() {
  return request.get<Docker.GrantOption[]>({ url: '/api/v1/docker/grants/options' })
}

export function fetchDockerGrants(envId: string) {
  return request.get<Docker.Grant[]>({ url: `/api/v1/docker/environments/${envId}/grants` })
}

export function setDockerGrants(
  envId: string,
  grants: Array<{ subjectType: string; subjectId: string; accessLevel: number }>
) {
  return request.put({
    url: `/api/v1/docker/environments/${envId}/grants`,
    data: { grants },
    successMessage: '资源授权已保存',
    showSuccessMessage: true
  })
}

/* ------------------------------ 概览 ------------------------------ */

export function fetchDockerOverview(envId: string) {
  return request.get<Docker.Overview>({ url: `/api/v1/docker/environments/${envId}/overview` })
}

/* ------------------------------ 容器 ------------------------------ */

export function fetchDockerContainers(envId: string, params: Record<string, any> = {}) {
  return request.get<Docker.Container[]>({
    url: `/api/v1/docker/environments/${envId}/containers`,
    params
  })
}

export function fetchDockerContainerInspect(envId: string, cid: string) {
  return request.get<any>({ url: `/api/v1/docker/environments/${envId}/containers/${cid}` })
}

export function dockerContainerAction(envId: string, cid: string, action: string, name?: string) {
  return request.post({
    url: `/api/v1/docker/environments/${envId}/containers/${cid}/actions`,
    data: { action, name },
    successMessage: '操作成功',
    showSuccessMessage: true
  })
}

export function removeDockerContainer(envId: string, cid: string, force = true, volumes = false) {
  return request.del({
    url: `/api/v1/docker/environments/${envId}/containers/${cid}?force=${force}&volumes=${volumes}`,
    successMessage: '容器已删除',
    showSuccessMessage: true
  })
}

export function createDockerContainer(envId: string, data: Record<string, any>) {
  return request.post<{ id: string }>({
    url: `/api/v1/docker/environments/${envId}/containers`,
    data,
    successMessage: '容器已创建',
    showSuccessMessage: true
  })
}

export function fetchDockerContainerLogs(
  envId: string,
  cid: string,
  tail = 200,
  timestamps = true
) {
  return request.get<{ logs: string }>({
    url: `/api/v1/docker/environments/${envId}/containers/${cid}/logs`,
    params: { tail, timestamps }
  })
}

export function fetchDockerContainerStats(envId: string, cid: string) {
  return request.get<Docker.ContainerStats>({
    url: `/api/v1/docker/environments/${envId}/containers/${cid}/stats`
  })
}

export function execDockerContainer(envId: string, cid: string, command: string) {
  return request.post<{ output: string; exitCode: number }>({
    url: `/api/v1/docker/environments/${envId}/containers/${cid}/exec`,
    data: { command },
    // 后端最长执行 60s，默认 15s 会先超时
    timeout: 70000
  })
}

export function pruneDockerContainers(envId: string) {
  return request.post<{ deleted: number; spaceReclaimed: number }>({
    url: `/api/v1/docker/environments/${envId}/containers/prune`,
    successMessage: '已清理停止的容器',
    showSuccessMessage: true
  })
}

/* ------------------------------ 镜像 ------------------------------ */

export function fetchDockerImages(envId: string, params: Record<string, any> = {}) {
  return request.get<Docker.Image[]>({
    url: `/api/v1/docker/environments/${envId}/images`,
    params
  })
}

export function fetchDockerImageInspect(envId: string, imageId: string) {
  return request.get<any>({
    url: `/api/v1/docker/environments/${envId}/images/${encodeURIComponent(imageId)}`
  })
}

export function removeDockerImage(envId: string, imageId: string, force = true) {
  return request.del({
    url: `/api/v1/docker/environments/${envId}/images/${encodeURIComponent(imageId)}?force=${force}`,
    successMessage: '镜像已删除',
    showSuccessMessage: true,
    timeout: 2 * 60 * 1000
  })
}

export function pruneDockerImages(envId: string) {
  return request.post<{ deleted: number; spaceReclaimed: number }>({
    url: `/api/v1/docker/environments/${envId}/images/prune`,
    successMessage: '已清理悬空镜像',
    showSuccessMessage: true
  })
}

/**
 * 拉取镜像：读取 NDJSON 进度流。
 * 返回最终的进度文本；失败时抛出后端/Docker 的可读错误。
 */
export async function pullDockerImage(
  envId: string,
  ref: string,
  onProgress?: (text: string) => void,
  signal?: AbortSignal
): Promise<void> {
  const token = useUserStore().accessToken
  const resp = await fetch(
    apiUrl(`/api/v1/docker/environments/${envId}/images/pull?ref=${encodeURIComponent(ref)}`),
    { headers: token ? { Authorization: `Bearer ${token}` } : {}, signal }
  )
  if (!resp.ok) {
    const text = await resp.text()
    let message = text || `镜像拉取请求失败（HTTP ${resp.status}）`
    try {
      const body = JSON.parse(text)
      if (body?.message) message = body.message
    } catch {
      /* 非 JSON 错误，保留原文 */
    }
    throw new Error(message)
  }
  if (!resp.body) throw new Error('浏览器不支持流式读取进度')
  const reader = resp.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let lastLine = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split('\n')
    buffer = lines.pop() || ''
    for (const line of lines) {
      const trimmed = line.trim()
      if (!trimmed) continue
      let text = trimmed
      try {
        const msg = JSON.parse(trimmed)
        if (msg.error) throw new Error(msg.error)
        if (msg.errorDetail?.message) throw new Error(msg.errorDetail.message)
        const status = String(msg.status || '')
        const progress = String(msg.progress || '')
        const id = msg.id ? `${msg.id}: ` : ''
        text = status + (progress ? ` ${progress}` : '')
        if (text) text = id + text
      } catch (err) {
        if (err instanceof SyntaxError) {
          // 非 JSON 行原样展示
        } else {
          throw err
        }
      }
      lastLine = text
      if (text) onProgress?.(text)
    }
  }
  if (buffer.trim()) {
    lastLine = buffer.trim()
    onProgress?.(lastLine)
  }
  onProgress?.('拉取完成')
}

/* ------------------------------ 网络 ------------------------------ */

export function fetchDockerNetworks(envId: string, params: Record<string, any> = {}) {
  return request.get<Docker.Network[]>({
    url: `/api/v1/docker/environments/${envId}/networks`,
    params
  })
}

export function fetchDockerNetworkInspect(envId: string, networkId: string) {
  return request.get<any>({
    url: `/api/v1/docker/environments/${envId}/networks/${networkId}`
  })
}

export function createDockerNetwork(envId: string, data: Record<string, any>) {
  return request.post<{ id: string }>({
    url: `/api/v1/docker/environments/${envId}/networks`,
    data,
    successMessage: '网络已创建',
    showSuccessMessage: true
  })
}

export function removeDockerNetwork(envId: string, networkId: string) {
  return request.del({
    url: `/api/v1/docker/environments/${envId}/networks/${networkId}`,
    successMessage: '网络已删除',
    showSuccessMessage: true
  })
}

export function connectDockerNetwork(envId: string, networkId: string, containerId: string, aliases: string[] = []) {
  return request.post({
    url: `/api/v1/docker/environments/${envId}/networks/${networkId}/connect`,
    data: { containerId, aliases },
    successMessage: '已接入网络',
    showSuccessMessage: true
  })
}

export function disconnectDockerNetwork(envId: string, networkId: string, containerId: string) {
  return request.post({
    url: `/api/v1/docker/environments/${envId}/networks/${networkId}/disconnect`,
    data: { containerId, force: true },
    successMessage: '已断开网络',
    showSuccessMessage: true
  })
}

export function pruneDockerNetworks(envId: string) {
  return request.post<{ deleted: number }>({
    url: `/api/v1/docker/environments/${envId}/networks/prune`,
    successMessage: '已清理未使用网络',
    showSuccessMessage: true
  })
}

/* ------------------------------ 数据卷 ------------------------------ */

export function fetchDockerVolumes(envId: string, params: Record<string, any> = {}) {
  return request.get<Docker.Volume[]>({
    url: `/api/v1/docker/environments/${envId}/volumes`,
    params
  })
}

export function fetchDockerVolumeInspect(envId: string, name: string) {
  return request.get<any>({
    url: `/api/v1/docker/environments/${envId}/volumes/${encodeURIComponent(name)}`
  })
}

export function createDockerVolume(envId: string, data: Record<string, any>) {
  return request.post({
    url: `/api/v1/docker/environments/${envId}/volumes`,
    data,
    successMessage: '数据卷已创建',
    showSuccessMessage: true
  })
}

export function removeDockerVolume(envId: string, name: string, force = false) {
  return request.del({
    url: `/api/v1/docker/environments/${envId}/volumes/${encodeURIComponent(name)}?force=${force}`,
    successMessage: '数据卷已删除',
    showSuccessMessage: true
  })
}

export function pruneDockerVolumes(envId: string) {
  return request.post<{ deleted: number; spaceReclaimed: number }>({
    url: `/api/v1/docker/environments/${envId}/volumes/prune`,
    successMessage: '已清理未使用数据卷',
    showSuccessMessage: true
  })
}

/* ------------------------------ Swarm ------------------------------ */

export function fetchDockerSwarm(envId: string) {
  return request.get<Docker.SwarmState>({ url: `/api/v1/docker/environments/${envId}/swarm` })
}

export function initDockerSwarm(envId: string, data: { advertiseAddr?: string; listenAddr?: string }) {
  return request.post<{ nodeId: string }>({
    url: `/api/v1/docker/environments/${envId}/swarm/init`,
    data,
    successMessage: 'Swarm 已初始化',
    showSuccessMessage: true
  })
}

export function leaveDockerSwarm(envId: string, force = false) {
  return request.post({
    url: `/api/v1/docker/environments/${envId}/swarm/leave?force=${force}`,
    successMessage: '已退出 Swarm',
    showSuccessMessage: true
  })
}

export function fetchDockerNodes(envId: string) {
  return request.get<Docker.Node[]>({ url: `/api/v1/docker/environments/${envId}/nodes` })
}

export function updateDockerNode(
  envId: string,
  nodeId: string,
  data: { role?: string; availability?: string; labels?: Record<string, string> }
) {
  return request.put({
    url: `/api/v1/docker/environments/${envId}/nodes/${nodeId}`,
    data,
    successMessage: '节点已更新',
    showSuccessMessage: true
  })
}

export function removeDockerNode(envId: string, nodeId: string, force = false) {
  return request.del({
    url: `/api/v1/docker/environments/${envId}/nodes/${nodeId}?force=${force}`,
    successMessage: '节点已移除',
    showSuccessMessage: true
  })
}

export function fetchDockerServices(envId: string, params: Record<string, any> = {}) {
  return request.get<Docker.Service[]>({
    url: `/api/v1/docker/environments/${envId}/services`,
    params
  })
}

export function fetchDockerServiceDetail(envId: string, sid: string) {
  return request.get<any>({
    url: `/api/v1/docker/environments/${envId}/services/${sid}`
  })
}

export function createDockerService(envId: string, data: Record<string, any>) {
  return request.post<{ id: string }>({
    url: `/api/v1/docker/environments/${envId}/services`,
    data,
    successMessage: '服务已创建',
    showSuccessMessage: true
  })
}

export function updateDockerService(envId: string, sid: string, data: Record<string, any>) {
  return request.put({
    url: `/api/v1/docker/environments/${envId}/services/${sid}`,
    data,
    successMessage: '服务已更新',
    showSuccessMessage: true
  })
}

export function scaleDockerService(envId: string, sid: string, replicas: number) {
  return request.post({
    url: `/api/v1/docker/environments/${envId}/services/${sid}/scale`,
    data: { replicas },
    successMessage: '副本数已调整',
    showSuccessMessage: true
  })
}

export function removeDockerService(envId: string, sid: string) {
  return request.del({
    url: `/api/v1/docker/environments/${envId}/services/${sid}`,
    successMessage: '服务已删除',
    showSuccessMessage: true
  })
}

export function fetchDockerServiceLogs(envId: string, sid: string, tail = 200) {
  return request.get<{ logs: string }>({
    url: `/api/v1/docker/environments/${envId}/services/${sid}/logs`,
    params: { tail }
  })
}

export function fetchDockerTasks(envId: string) {
  return request.get<Docker.Task[]>({ url: `/api/v1/docker/environments/${envId}/tasks` })
}

export function fetchDockerSecrets(envId: string) {
  return request.get<Docker.Secret[]>({ url: `/api/v1/docker/environments/${envId}/secrets` })
}

export function createDockerSecret(envId: string, data: { name: string; data: string }) {
  return request.post<{ id: string }>({
    url: `/api/v1/docker/environments/${envId}/secrets`,
    data,
    successMessage: '密钥已创建',
    showSuccessMessage: true
  })
}

export function removeDockerSecret(envId: string, sid: string) {
  return request.del({
    url: `/api/v1/docker/environments/${envId}/secrets/${sid}`,
    successMessage: '密钥已删除',
    showSuccessMessage: true
  })
}

export function fetchDockerConfigs(envId: string) {
  return request.get<Docker.ConfigObj[]>({ url: `/api/v1/docker/environments/${envId}/configs` })
}

export function createDockerConfig(envId: string, data: { name: string; data: string }) {
  return request.post<{ id: string }>({
    url: `/api/v1/docker/environments/${envId}/configs`,
    data,
    successMessage: '配置已创建',
    showSuccessMessage: true
  })
}

export function removeDockerConfig(envId: string, cid: string) {
  return request.del({
    url: `/api/v1/docker/environments/${envId}/configs/${cid}`,
    successMessage: '配置已删除',
    showSuccessMessage: true
  })
}

/* ------------------------------ Stack ------------------------------ */

export function fetchDockerStacks(envId?: string) {
  return request.get<Docker.Stack[]>({
    url: '/api/v1/docker/stacks',
    params: envId ? { envId } : undefined
  })
}

export function fetchDockerStack(id: string) {
  return request.get<Docker.StackDetail>({ url: `/api/v1/docker/stacks/${id}` })
}

export function createDockerStack(data: {
  environmentId: string
  name: string
  type: string
  composeContent: string
  envContent: string
}) {
  return request.post<{ id: string }>({
    url: '/api/v1/docker/stacks',
    data,
    showSuccessMessage: false
  })
}

export function updateDockerStack(
  id: string,
  data: { type: string; composeContent: string; envContent: string }
) {
  return request.put({
    url: `/api/v1/docker/stacks/${id}`,
    data,
    successMessage: 'Stack 已保存',
    showSuccessMessage: true
  })
}

export function deleteDockerStack(id: string, runtime = true, volumes = false) {
  return request.del({
    url: `/api/v1/docker/stacks/${id}?runtime=${runtime}&volumes=${volumes}`,
    successMessage: 'Stack 已删除',
    showSuccessMessage: true,
    timeout: 5 * 60 * 1000
  })
}

export function deployDockerStack(id: string, pullImages = false) {
  return request.post({
    url: `/api/v1/docker/stacks/${id}/deploy`,
    data: { pullImages },
    successMessage: '部署成功',
    showSuccessMessage: true,
    // 拉镜像 + 创建服务可能远超默认 15s；后端部署超时为 20 分钟
    timeout: 20 * 60 * 1000
  })
}

export function stopDockerStack(id: string) {
  return request.post({
    url: `/api/v1/docker/stacks/${id}/stop`,
    successMessage: 'Stack 已停止',
    showSuccessMessage: true,
    timeout: 5 * 60 * 1000
  })
}

export function rollbackDockerStack(id: string, version: number, deployNow = false) {
  return request.post({
    url: `/api/v1/docker/stacks/${id}/rollback`,
    data: { version, deployNow },
    successMessage: '已回滚到指定版本',
    showSuccessMessage: true,
    timeout: deployNow ? 20 * 60 * 1000 : 60000
  })
}

export function exportDockerStacks() {
  return request.get<{ stacks: Docker.StackExport[]; exportedAt: string }>({
    url: '/api/v1/docker/stacks/export'
  })
}

export function importDockerStacks(data: { overwrite: boolean; stacks: Docker.StackExport[] }) {
  return request.post<{ created: number; updated: number; skipped: number }>({
    url: '/api/v1/docker/stacks/import',
    data,
    successMessage: 'Stack 导入完成',
    showSuccessMessage: true
  })
}

/** 下载 SQLite 备份（整库恢复用）。 */
export async function downloadDockerBackup(): Promise<void> {
  const token = useUserStore().accessToken
  const resp = await fetch(apiUrl('/api/v1/docker/stacks/backup'), {
    headers: token ? { Authorization: `Bearer ${token}` } : {}
  })
  if (!resp.ok) {
    const text = await resp.text()
    let message = text || '备份下载失败'
    try {
      const body = JSON.parse(text)
      if (body?.message) message = body.message
    } catch {
      /* 非 JSON 错误，保留原文 */
    }
    throw new Error(message)
  }
  const blob = await resp.blob()
  const disposition = resp.headers.get('content-disposition') || ''
  const match = disposition.match(/filename="?([^";]+)"?/)
  const name = match?.[1] || `tomato-docker-${Date.now()}.db`
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}