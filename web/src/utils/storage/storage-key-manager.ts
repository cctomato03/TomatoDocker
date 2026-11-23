import { StorageConfig } from './storage-config'

/**
 * Pinia 持久化键管理器：`sys-v{version}-{storeId}`。
 *
 * 版本号只影响存储键前缀，不做破坏性清空；需要迁移时在 migrateStorage 中处理。
 */
export class StorageKeyManager {
  private readonly version: number

  constructor(version = StorageConfig.STORAGE_VERSION) {
    this.version = version
  }

  getStorageKey(storeId: string): string {
    return `${StorageConfig.KEY_PREFIX}-v${this.version}-${storeId}`
  }
}