-- 修正 Stack 编辑器隐藏路由的父级：
-- 原实现把它挂在「堆栈」页面下（父级是页面组件，没有 router-view），
-- 导致 /docker/stacks/editor 子路由无法渲染；改为 Docker 目录下的同级隐藏菜单。
UPDATE sys_menu
SET parent_id = 1600,
    path = 'stacks/editor',
    permission = 'docker:stack:list'
WHERE id = 1679;