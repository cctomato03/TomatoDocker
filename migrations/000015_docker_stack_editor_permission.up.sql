-- Stack 编辑器是隐藏页面：只要能看到 Stack 列表就允许进入编辑器，
-- 具体「新建/保存/部署」按钮仍由 docker:stack:create/update/deploy 控制。
UPDATE sys_menu SET permission = 'docker:stack:list' WHERE id = 1679;
