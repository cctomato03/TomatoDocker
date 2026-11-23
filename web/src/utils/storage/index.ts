import { StorageConfig } from './storage-config'

export { StorageConfig } from './storage-config'
export { StorageKeyManager } from './storage-key-manager'

/**
 * 检查本地存储与当前应用版本是否兼容。
 *
 * 只记录版本并输出提示，不主动清空数据：旧版本残留的无关键不会影响登录态，
 * 贸然清空会把用户的 token/主题设置一起丢掉。
 */
export function checkStorageCompatibility(): void {
  try {
    const current = String(import.meta.env.VITE_VERSION || 'dev')
    const stored = localStorage.getItem(StorageConfig.APP_VERSION_KEY)
    if (stored && stored !== current) {
      console.info(`[storage] 应用版本已从 ${stored} 更新到 ${current}，本地设置将尽量保留`)
    }
    localStorage.setItem(StorageConfig.APP_VERSION_KEY, current)
  } catch {
    // localStorage 被禁用（隐私模式）时不阻塞启动
  }
}