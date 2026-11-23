-- Docker 管理模块菜单与权限点
--
-- 说明：
--   * 页面（type=2）的权限码控制"能看到哪个页面"；
--   * 按钮（type=3）的权限码控制"能点哪些按钮"；
--   * 环境级资源授权（只读/操作/管理）在 docker_environment_grant（SQLite）中单独维护，
--     与这里的功能权限是"与"关系，缺一不可；
--   * Stack 定义、环境连接信息存放在独立 SQLite，MySQL 只维护菜单/权限。

-- ---------- 1. 一级目录：Docker 管理 ----------
INSERT IGNORE INTO sys_menu
  (id, parent_id, name, name_en, type, permission, path, component, icon, sort, visible, keep_alive, status, tenant_id, create_time, remark)
VALUES
  (1600, 0, 'Docker 管理', 'Docker', 1, '', '/docker', 'Layout', 'ep:box', 15, 1, 1, 1, 1, NOW(), '本地 Docker 与 Docker Swarm 管理');

-- ---------- 2. 页面 ----------
INSERT IGNORE INTO sys_menu
  (id, parent_id, name, name_en, type, permission, path, component, icon, sort, visible, keep_alive, status, tenant_id, create_time, remark)
VALUES
  (1610, 1600, 'Docker 概览', 'Docker Overview',      2, 'docker:environment:list', 'overview',     'docker/overview/index',     'ep:odometer',   1, 1, 1, 1, 1, NOW(), '环境主机信息、资源计数与 Stack 概览'),
  (1615, 1600, '环境管理',     'Environments',         2, 'docker:environment:list', 'environments', 'docker/environments/index', 'ep:setting',    2, 1, 1, 1, 1, NOW(), '管理本地/远程 Docker 连接'),
  (1620, 1600, '资源授权',     'Resource Permissions', 2, 'docker:environment:grant','grants',       'docker/grants/index',       'ep:key',        3, 1, 1, 1, 1, NOW(), '按角色/用户授权环境访问级别'),
  (1630, 1600, '容器',         'Containers',           2, 'docker:container:list',   'containers',   'docker/containers/index',   'ep:box',        4, 1, 1, 1, 1, NOW(), '容器生命周期、日志、资源与命令执行'),
  (1640, 1600, '镜像',         'Images',               2, 'docker:image:list',       'images',       'docker/images/index',       'ep:files',      5, 1, 1, 1, 1, NOW(), '镜像拉取、删除与清理'),
  (1650, 1600, '网络',         'Networks',             2, 'docker:network:list',     'networks',     'docker/networks/index',     'ep:share',      6, 1, 1, 1, 1, NOW(), 'Docker 网络创建、删除与容器连接'),
  (1660, 1600, '数据卷',       'Volumes',              2, 'docker:volume:list',      'volumes',      'docker/volumes/index',      'ep:folder',     7, 1, 1, 1, 1, NOW(), '数据卷创建、删除与清理'),
  (1670, 1600, '堆栈',         'Stacks',               2, 'docker:stack:list',       'stacks',       'docker/stacks/index',       'ep:files',      8, 1, 1, 1, 1, NOW(), '自定义 compose/swarm 文件部署与管理'),
  (1679, 1600, 'Stack 编辑器',  'Stack Editor',         2, 'docker:stack:list',       'stacks/editor','docker/stacks/editor',      'ep:edit',       9, 0, 0, 1, 1, NOW(), 'Stack 创建/编辑页（从列表进入，不显示在菜单）'),
  (1680, 1600, 'Swarm 集群',   'Swarm',                1, '',                        'swarm',        'Layout',                    'ep:connection', 10, 1, 1, 1, 1, NOW(), 'Swarm 集群管理'),
  (1690, 1680, '集群状态',     'Cluster Status',       2, 'docker:swarm:view',       'cluster',      'docker/swarm/index',        'ep:monitor',    1, 1, 1, 1, 1, NOW(), '初始化/退出 Swarm，查看集群信息'),
  (1681, 1680, '节点',         'Nodes',                2, 'docker:node:list',        'nodes',        'docker/swarm/nodes/index',  'ep:cpu',        2, 1, 1, 1, 1, NOW(), 'Swarm 节点角色、可用性与标签'),
  (1682, 1680, '服务',         'Services',             2, 'docker:service:list',     'services',     'docker/swarm/services/index','ep:management',3, 1, 1, 1, 1, NOW(), 'Swarm 服务创建、扩缩容与更新'),
  (1683, 1680, '任务',         'Tasks',                2, 'docker:task:list',        'tasks',        'docker/swarm/tasks/index',  'ep:list',       4, 1, 1, 1, 1, NOW(), 'Swarm 任务运行状态'),
  (1684, 1680, '密钥',         'Secrets',              2, 'docker:secret:list',      'secrets',      'docker/swarm/secrets/index','ep:key',        5, 1, 1, 1, 1, NOW(), 'Swarm 密钥（只写不读，Data 不返回）'),
  (1685, 1680, '配置',         'Configs',              2, 'docker:config:list',      'configs',      'docker/swarm/configs/index','ep:document',   6, 1, 1, 1, 1, NOW(), 'Swarm 配置（Data 不返回）');

-- ---------- 3. 按钮/操作权限点 ----------
INSERT IGNORE INTO sys_menu
  (id, parent_id, name, name_en, type, permission, path, component, icon, sort, visible, keep_alive, status, tenant_id, create_time, remark)
VALUES
  (1616, 1615, '新增环境', 'Create Environment', 3, 'docker:environment:create', '', '', '', 1, 1, 1, 1, 1, NOW(), ''),
  (1617, 1615, '修改环境', 'Update Environment', 3, 'docker:environment:update', '', '', '', 2, 1, 1, 1, 1, NOW(), ''),
  (1618, 1615, '删除环境', 'Delete Environment', 3, 'docker:environment:delete', '', '', '', 3, 1, 1, 1, 1, NOW(), ''),
  (1619, 1615, '测试连接', 'Test Connection',    3, 'docker:environment:test',   '', '', '', 4, 1, 1, 1, 1, NOW(), ''),
  (1621, 1620, '保存授权', 'Save Permissions',   3, 'docker:environment:grant',  '', '', '', 1, 1, 1, 1, 1, NOW(), ''),

  (1631, 1630, '创建容器', 'Create Container', 3, 'docker:container:create',  '', '', '', 1, 1, 1, 1, 1, NOW(), ''),
  (1632, 1630, '容器操作', 'Operate Container',3, 'docker:container:operate', '', '', '', 2, 1, 1, 1, 1, NOW(), '启动/停止/重启/暂停'),
  (1633, 1630, '删除容器', 'Delete Container', 3, 'docker:container:delete',  '', '', '', 3, 1, 1, 1, 1, NOW(), ''),
  (1634, 1630, '命令执行', 'Exec Container',    3, 'docker:container:exec',    '', '', '', 4, 1, 1, 1, 1, NOW(), '在容器内执行非交互命令'),

  (1641, 1640, '拉取镜像', 'Pull Image',   3, 'docker:image:pull',   '', '', '', 1, 1, 1, 1, 1, NOW(), ''),
  (1642, 1640, '删除镜像', 'Delete Image', 3, 'docker:image:delete', '', '', '', 2, 1, 1, 1, 1, NOW(), ''),

  (1651, 1650, '创建网络', 'Create Network', 3, 'docker:network:create', '', '', '', 1, 1, 1, 1, 1, NOW(), ''),
  (1652, 1650, '删除网络', 'Delete Network', 3, 'docker:network:delete', '', '', '', 2, 1, 1, 1, 1, NOW(), ''),
  (1653, 1650, '连接管理', 'Connect Network', 3, 'docker:network:update', '', '', '', 3, 1, 1, 1, 1, NOW(), ''),

  (1661, 1660, '创建数据卷', 'Create Volume', 3, 'docker:volume:create', '', '', '', 1, 1, 1, 1, 1, NOW(), ''),
  (1662, 1660, '删除数据卷', 'Delete Volume', 3, 'docker:volume:delete', '', '', '', 2, 1, 1, 1, 1, NOW(), ''),

  (1671, 1670, '新建 Stack',  'Create Stack',   3, 'docker:stack:create', '', '', '', 1, 1, 1, 1, 1, NOW(), ''),
  (1672, 1670, '编辑 Stack',  'Update Stack',   3, 'docker:stack:update', '', '', '', 2, 1, 1, 1, 1, NOW(), ''),
  (1673, 1670, '删除 Stack',  'Delete Stack',   3, 'docker:stack:delete', '', '', '', 3, 1, 1, 1, 1, NOW(), ''),
  (1674, 1670, '部署 Stack',  'Deploy Stack',   3, 'docker:stack:deploy', '', '', '', 4, 1, 1, 1, 1, NOW(), ''),
  (1675, 1670, '停止 Stack',  'Stop Stack',     3, 'docker:stack:stop',   '', '', '', 5, 1, 1, 1, 1, NOW(), ''),
  (1676, 1670, '导出 Stack',  'Export Stacks',  3, 'docker:stack:export', '', '', '', 6, 1, 1, 1, 1, NOW(), 'JSON 导出 + SQLite 备份下载'),
  (1677, 1670, '导入 Stack',  'Import Stacks',  3, 'docker:stack:import', '', '', '', 7, 1, 1, 1, 1, NOW(), ''),

  (1701, 1682, '创建服务', 'Create Service', 3, 'docker:service:create', '', '', '', 1, 1, 1, 1, 1, NOW(), ''),
  (1702, 1682, '更新服务', 'Update Service', 3, 'docker:service:update', '', '', '', 2, 1, 1, 1, 1, NOW(), ''),
  (1703, 1682, '删除服务', 'Delete Service', 3, 'docker:service:delete', '', '', '', 3, 1, 1, 1, 1, NOW(), ''),
  (1704, 1682, '扩缩容',   'Scale Service',  3, 'docker:service:scale',  '', '', '', 4, 1, 1, 1, 1, NOW(), ''),
  (1705, 1684, '创建密钥', 'Create Secret',  3, 'docker:secret:create',  '', '', '', 1, 1, 1, 1, 1, NOW(), ''),
  (1706, 1684, '删除密钥', 'Delete Secret',  3, 'docker:secret:delete',  '', '', '', 2, 1, 1, 1, 1, NOW(), ''),
  (1707, 1685, '创建配置', 'Create Config',  3, 'docker:config:create',  '', '', '', 1, 1, 1, 1, 1, NOW(), ''),
  (1708, 1685, '删除配置', 'Delete Config',  3, 'docker:config:delete',  '', '', '', 2, 1, 1, 1, 1, NOW(), ''),
  (1709, 1681, '修改节点', 'Update Node',    3, 'docker:node:update',    '', '', '', 1, 1, 1, 1, 1, NOW(), ''),
  (1710, 1681, '移除节点', 'Remove Node',    3, 'docker:node:delete',    '', '', '', 2, 1, 1, 1, 1, NOW(), ''),
  (1711, 1690, '初始化集群', 'Init Swarm',   3, 'docker:swarm:init',     '', '', '', 1, 1, 1, 1, 1, NOW(), ''),
  (1712, 1690, '退出集群', 'Leave Swarm',    3, 'docker:swarm:leave',    '', '', '', 2, 1, 1, 1, 1, NOW(), '');

-- ---------- 4. 内置超级管理员角色授权（普通角色由管理员按需勾选） ----------
INSERT IGNORE INTO sys_role_menu (role_id, menu_id)
SELECT 1, id FROM sys_menu WHERE id BETWEEN 1600 AND 1712;