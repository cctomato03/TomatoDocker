/**
 * 本地存储键与版本配置。
 *
 * 说明：脚手架原目录缺少 storage 工具（store/index.ts 与 App.vue 已引用），
 * 这里补齐最小实现，保持键名稳定，避免升级后用户被强制退出。
 */
export const StorageConfig = {
  /** 应用版本号（用于存储兼容性检查） */
  APP_VERSION_KEY: 'sys-app-version',
  /** 上一次登录的用户 ID（切换账号时清理工作台标签） */
  LAST_USER_ID_KEY: 'sys-last-user-id',
  /** 主题模式（light / dark / system） */
  THEME_KEY: 'sys-theme',
  /** 响应式菜单布局选择 */
  RESPONSIVE_MENU_TYPE_KEY: 'sys-responsive-menu-type',
  /** 存储键前缀 */
  KEY_PREFIX: 'sys',
  /** 当前存储结构版本 */
  STORAGE_VERSION: 1
} as const

export type StorageConfigType = typeof StorageConfig