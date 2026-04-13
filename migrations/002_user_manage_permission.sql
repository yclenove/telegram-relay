-- 为已有库的 super_admin 角色补充 user.manage，避免升级代码后旧库无该权限行。
INSERT INTO role_permissions(role_id, permission_code)
SELECT id, 'user.manage' FROM roles WHERE code = 'super_admin'
ON CONFLICT DO NOTHING;
