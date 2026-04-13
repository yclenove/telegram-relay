package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/yclenove/telegram-relay/internal/domain"
)

// ListRoles 返回全部角色，供管理端用户表单多选。
func (s *Store) ListRoles(ctx context.Context) ([]domain.Role, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, code, name, created_at, updated_at FROM roles ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Role
	for rows.Next() {
		var r domain.Role
		if err := rows.Scan(&r.ID, &r.Code, &r.Name, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// ListUserSummaries 返回用户及角色 id 列表（不含密码）。
func (s *Store) ListUserSummaries(ctx context.Context) ([]domain.UserSummary, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, username, is_enabled, created_at, updated_at FROM users ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var summaries []domain.UserSummary
	for rows.Next() {
		var u domain.UserSummary
		if err := rows.Scan(&u.ID, &u.Username, &u.IsEnabled, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		u.RoleIDs = []int64{}
		summaries = append(summaries, u)
	}
	for i := range summaries {
		rids, err := s.listRoleIDsForUser(ctx, summaries[i].ID)
		if err != nil {
			return nil, err
		}
		summaries[i].RoleIDs = rids
	}
	return summaries, nil
}

func (s *Store) listRoleIDsForUser(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `SELECT role_id FROM user_roles WHERE user_id=$1 ORDER BY role_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var rid int64
		if err := rows.Scan(&rid); err != nil {
			return nil, err
		}
		out = append(out, rid)
	}
	return out, nil
}

func (s *Store) countUsersWithSuperAdmin(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `
SELECT COUNT(DISTINCT ur.user_id) FROM user_roles ur
JOIN roles r ON r.id = ur.role_id WHERE r.code = 'super_admin'`).Scan(&n)
	return n, err
}

func (s *Store) userHasSuperAdmin(ctx context.Context, userID int64) (bool, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `
SELECT COUNT(1) FROM user_roles ur
JOIN roles r ON r.id = ur.role_id
WHERE ur.user_id=$1 AND r.code='super_admin'`, userID).Scan(&n)
	return n > 0, err
}

// CreateUserWithRoles 创建用户并绑定角色；passwordHash 为已哈希后的口令。
func (s *Store) CreateUserWithRoles(ctx context.Context, username, passwordHash string, isEnabled bool, roleIDs []int64) (domain.UserSummary, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.UserSummary{}, err
	}
	defer tx.Rollback(ctx)
	var uid int64
	err = tx.QueryRow(ctx, `INSERT INTO users(username, password_hash, is_enabled) VALUES($1,$2,$3) RETURNING id`,
		username, passwordHash, isEnabled).Scan(&uid)
	if err != nil {
		return domain.UserSummary{}, err
	}
	for _, rid := range roleIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO user_roles(user_id, role_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, uid, rid); err != nil {
			return domain.UserSummary{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.UserSummary{}, err
	}
	return s.getUserSummary(ctx, uid)
}

func (s *Store) getUserSummary(ctx context.Context, userID int64) (domain.UserSummary, error) {
	var u domain.UserSummary
	err := s.pool.QueryRow(ctx, `SELECT id, username, is_enabled, created_at, updated_at FROM users WHERE id=$1`, userID).
		Scan(&u.ID, &u.Username, &u.IsEnabled, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return domain.UserSummary{}, err
	}
	u.RoleIDs, err = s.listRoleIDsForUser(ctx, userID)
	return u, err
}

// UpdateUserPatch 更新用户启停、角色、可选密码哈希。
func (s *Store) UpdateUserPatch(ctx context.Context, userID int64, isEnabled *bool, passwordHash *string, roleIDs *[]int64) (domain.UserSummary, error) {
	hadSuper, err := s.userHasSuperAdmin(ctx, userID)
	if err != nil {
		return domain.UserSummary{}, err
	}
	cnt, err := s.countUsersWithSuperAdmin(ctx)
	if err != nil {
		return domain.UserSummary{}, err
	}
	if roleIDs != nil && hadSuper && cnt == 1 {
		newHas := false
		for _, rid := range *roleIDs {
			var code string
			if err := s.pool.QueryRow(ctx, `SELECT code FROM roles WHERE id=$1`, rid).Scan(&code); err != nil {
				return domain.UserSummary{}, err
			}
			if code == "super_admin" {
				newHas = true
				break
			}
		}
		if !newHas {
			return domain.UserSummary{}, errors.New("不能移除最后一个超级管理员角色")
		}
	}
	if isEnabled != nil && !*isEnabled && hadSuper && cnt == 1 {
		return domain.UserSummary{}, errors.New("不能禁用最后一个超级管理员账号")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.UserSummary{}, err
	}
	defer tx.Rollback(ctx)
	if isEnabled != nil || passwordHash != nil {
		if passwordHash != nil && isEnabled != nil {
			_, err = tx.Exec(ctx, `UPDATE users SET is_enabled=$2, password_hash=$3, updated_at=NOW() WHERE id=$1`, userID, *isEnabled, *passwordHash)
		} else if passwordHash != nil {
			_, err = tx.Exec(ctx, `UPDATE users SET password_hash=$2, updated_at=NOW() WHERE id=$1`, userID, *passwordHash)
		} else {
			_, err = tx.Exec(ctx, `UPDATE users SET is_enabled=$2, updated_at=NOW() WHERE id=$1`, userID, *isEnabled)
		}
		if err != nil {
			return domain.UserSummary{}, err
		}
	}
	if roleIDs != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id=$1`, userID); err != nil {
			return domain.UserSummary{}, err
		}
		for _, rid := range *roleIDs {
			if _, err := tx.Exec(ctx, `INSERT INTO user_roles(user_id, role_id) VALUES($1,$2)`, userID, rid); err != nil {
				return domain.UserSummary{}, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.UserSummary{}, err
	}
	return s.getUserSummary(ctx, userID)
}

// DeleteUser 删除用户及其角色关联；禁止删除最后一个超级管理员。
func (s *Store) DeleteUser(ctx context.Context, userID int64) error {
	hadSuper, err := s.userHasSuperAdmin(ctx, userID)
	if err != nil {
		return err
	}
	cnt, err := s.countUsersWithSuperAdmin(ctx)
	if err != nil {
		return err
	}
	if hadSuper && cnt <= 1 {
		return errors.New("不能删除最后一个超级管理员账号")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id=$1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ValidateRoleIDsExist 校验角色 id 均存在，防止伪造 id。
func (s *Store) ValidateRoleIDsExist(ctx context.Context, roleIDs []int64) error {
	if len(roleIDs) == 0 {
		return nil
	}
	for _, rid := range roleIDs {
		var n int64
		if err := s.pool.QueryRow(ctx, `SELECT COUNT(1) FROM roles WHERE id=$1`, rid).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("无效的角色 id: %d", rid)
		}
	}
	return nil
}

// GetUserByUsername 用于创建前检查重名。
func (s *Store) GetUserByUsername(ctx context.Context, username string) (domain.User, error) {
	var u domain.User
	err := s.pool.QueryRow(ctx, `SELECT id, username, password_hash, is_enabled, created_at, updated_at FROM users WHERE username=$1`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsEnabled, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return domain.User{}, err
	}
	return u, nil
}

// GetUserByID 按主键读取用户（含密码哈希，仅服务层使用）。
func (s *Store) GetUserByID(ctx context.Context, userID int64) (domain.User, error) {
	var u domain.User
	err := s.pool.QueryRow(ctx, `SELECT id, username, password_hash, is_enabled, created_at, updated_at FROM users WHERE id=$1`, userID).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsEnabled, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return domain.User{}, err
	}
	return u, nil
}

