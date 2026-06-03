/*
Copyright (C) 2023-2026 QuantumNous + xizaihui (Phase 2 fork)

Per-user group ratio override.
新语义: 同一分组对不同用户给不同倍率
  例: user1 用 企业kiro200 → 8 折
      user2 用 企业kiro200 → 9 折
  原 NewAPI 只支持 GroupGroupRatio[userGroup][group] (按用户组覆盖)
  现新增 UserGroupRatio[userId][group] (按单用户覆盖) 在最前面查
*/

package model

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type UserGroupRatio struct {
	UserId    int     `json:"user_id" gorm:"primaryKey;autoIncrement:false;index"`
	GroupName string  `json:"group_name" gorm:"primaryKey;autoIncrement:false;type:varchar(64)"`
	Ratio     float64 `json:"ratio" gorm:"type:double precision;not null;default:1"`
	CreatedAt int64   `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt int64   `json:"updated_at" gorm:"autoUpdateTime"`
}

func (UserGroupRatio) TableName() string {
	return "user_group_ratios"
}

// ========================================
// CRUD
// ========================================

// GetUserGroupRatios 拿某用户所有分组覆盖
func GetUserGroupRatios(userId int) ([]UserGroupRatio, error) {
	var ratios []UserGroupRatio
	err := DB.Where("user_id = ?", userId).Find(&ratios).Error
	return ratios, err
}

// GetUserGroupRatioMap 返回 map[group]ratio，方便 caller 查找
func GetUserGroupRatioMap(userId int) (map[string]float64, error) {
	ratios, err := GetUserGroupRatios(userId)
	if err != nil {
		return nil, err
	}
	m := make(map[string]float64, len(ratios))
	for _, r := range ratios {
		m[r.GroupName] = r.Ratio
	}
	return m, nil
}

// SetUserGroupRatios 用 upsert + delete 不在新集合的方式批量设置
// ratios 为 nil 或空 map 时清空该用户的所有覆盖
func SetUserGroupRatios(userId int, ratios map[string]float64) error {
	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 先删旧的全量
	if err := tx.Where("user_id = ?", userId).Delete(&UserGroupRatio{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	// 写新的
	if len(ratios) > 0 {
		records := make([]UserGroupRatio, 0, len(ratios))
		for g, r := range ratios {
			if r < 0 {
				tx.Rollback()
				return fmt.Errorf("ratio for group %q must be >= 0", g)
			}
			records = append(records, UserGroupRatio{
				UserId:    userId,
				GroupName: g,
				Ratio:     r,
			})
		}
		if err := tx.Create(&records).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	if err := tx.Commit().Error; err != nil {
		return err
	}

	// 失效缓存
	invalidateUserGroupRatioCache(userId)
	return nil
}

// DeleteUserGroupRatio 删除某用户某分组的覆盖
func DeleteUserGroupRatio(userId int, groupName string) error {
	if err := DB.Where("user_id = ? AND group_name = ?", userId, groupName).Delete(&UserGroupRatio{}).Error; err != nil {
		return err
	}
	invalidateUserGroupRatioCache(userId)
	return nil
}

// ========================================
// Cache (Redis, TTL 5min)
// ========================================

const userGroupRatioCacheTTL = 5 * time.Minute

func userGroupRatioCacheKey(userId int) string {
	return fmt.Sprintf("user_group_ratio:%d", userId)
}

// GetUserGroupRatioCached 计费链路高频路径调用，先查 Redis，未命中查 DB 并回填
// 返回 (map, error)；如果用户没设任何覆盖，返回 (空 map, nil)
func GetUserGroupRatioCached(userId int) (map[string]float64, error) {
	if !common.RedisEnabled {
		return GetUserGroupRatioMap(userId)
	}

	key := userGroupRatioCacheKey(userId)
	cached, err := common.RedisGet(key)
	if err == nil && cached != "" {
		// hit
		m := make(map[string]float64)
		if cached == "{}" {
			return m, nil
		}
		if err := common.Unmarshal([]byte(cached), &m); err == nil {
			return m, nil
		}
		// 反序列化失败，fallthrough 重查 DB
	}

	// miss
	m, err := GetUserGroupRatioMap(userId)
	if err != nil {
		return nil, err
	}

	// 回填缓存（即使是空 map 也缓存，避免雪崩）
	bs, _ := common.Marshal(m)
	if bs == nil {
		bs = []byte("{}")
	}
	_ = common.RedisSet(key, string(bs), userGroupRatioCacheTTL)

	return m, nil
}

func invalidateUserGroupRatioCache(userId int) {
	if !common.RedisEnabled {
		return
	}
	_ = common.RedisDel(userGroupRatioCacheKey(userId))
}

// LookupUserGroupRatio 一键查 (userId, group) → (ratio, ok)
// 计费链路最常用，避免每次都拿到完整 map 再查
func LookupUserGroupRatio(userId int, group string) (float64, bool) {
	if userId <= 0 || group == "" {
		return 0, false
	}
	m, err := GetUserGroupRatioCached(userId)
	if err != nil || m == nil {
		return 0, false
	}
	r, ok := m[group]
	return r, ok
}
