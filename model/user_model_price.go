/*
Copyright (C) 2023-2026 QuantumNous + xizaihui (Phase 3 fork)

Per-user model price override.
新语义: 同一模型对不同用户给不同按次价格
  例: 全局 gemini-3-pro-image-preview = 0.1/次
      VIP-user-X = 0.25/次   (对 X 单独覆盖)
  原 NewAPI 只支持全局 modelPriceMap[model] → price
  现新增 UserModelPrice[userId][model] → price 优先级最高
*/

package model

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type UserModelPrice struct {
	UserId    int     `json:"user_id" gorm:"primaryKey;autoIncrement:false;index"`
	ModelName string  `json:"model_name" gorm:"primaryKey;autoIncrement:false;type:varchar(128)"`
	Price     float64 `json:"price" gorm:"type:double precision;not null;default:0"`
	Note      string  `json:"note" gorm:"type:text;default:''"`
	CreatedAt int64   `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt int64   `json:"updated_at" gorm:"autoUpdateTime"`
}

func (UserModelPrice) TableName() string {
	return "user_model_prices"
}

// ========================================
// CRUD
// ========================================

// GetUserModelPrices 拿某用户所有模型价格覆盖
func GetUserModelPrices(userId int) ([]UserModelPrice, error) {
	var prices []UserModelPrice
	err := DB.Where("user_id = ?", userId).Order("model_name").Find(&prices).Error
	return prices, err
}

// GetUserModelPriceMap 返回 map[model]price，方便 caller 查找
func GetUserModelPriceMap(userId int) (map[string]float64, error) {
	prices, err := GetUserModelPrices(userId)
	if err != nil {
		return nil, err
	}
	m := make(map[string]float64, len(prices))
	for _, p := range prices {
		m[p.ModelName] = p.Price
	}
	return m, nil
}

// SetUserModelPricesRequest 单条
type SetUserModelPriceItem struct {
	ModelName string  `json:"model_name"`
	Price     float64 `json:"price"`
	Note      string  `json:"note"`
}

// SetUserModelPrices 用 upsert + delete 不在新集合的方式批量设置
// items 为 nil 或空时清空该用户的所有覆盖
func SetUserModelPrices(userId int, items []SetUserModelPriceItem) error {
	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 全量替换语义：先删旧的
	if err := tx.Where("user_id = ?", userId).Delete(&UserModelPrice{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	if len(items) > 0 {
		records := make([]UserModelPrice, 0, len(items))
		for _, it := range items {
			if it.ModelName == "" {
				continue
			}
			if it.Price < 0 {
				tx.Rollback()
				return fmt.Errorf("price for model %q must be >= 0", it.ModelName)
			}
			records = append(records, UserModelPrice{
				UserId:    userId,
				ModelName: it.ModelName,
				Price:     it.Price,
				Note:      it.Note,
			})
		}
		if len(records) > 0 {
			if err := tx.Create(&records).Error; err != nil {
				tx.Rollback()
				return err
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		return err
	}

	invalidateUserModelPriceCache(userId)
	return nil
}

// DeleteUserModelPrice 删除某用户某模型的覆盖
func DeleteUserModelPrice(userId int, modelName string) error {
	if err := DB.Where("user_id = ? AND model_name = ?", userId, modelName).Delete(&UserModelPrice{}).Error; err != nil {
		return err
	}
	invalidateUserModelPriceCache(userId)
	return nil
}

// ========================================
// Cache (Redis, TTL 5min)
// ========================================

const userModelPriceCacheTTL = 5 * time.Minute

func userModelPriceCacheKey(userId int) string {
	return fmt.Sprintf("user_model_price:%d", userId)
}

// GetUserModelPriceCached 计费链路高频路径调用，先查 Redis，未命中查 DB 并回填
// 返回 (map, error)；如果用户没设任何覆盖，返回 (空 map, nil)
func GetUserModelPriceCached(userId int) (map[string]float64, error) {
	if !common.RedisEnabled {
		return GetUserModelPriceMap(userId)
	}

	key := userModelPriceCacheKey(userId)
	cached, err := common.RedisGet(key)
	if err == nil && cached != "" {
		m := make(map[string]float64)
		if cached == "{}" {
			return m, nil
		}
		if err := common.Unmarshal([]byte(cached), &m); err == nil {
			return m, nil
		}
		// 反序列化失败，fallthrough
	}

	m, err := GetUserModelPriceMap(userId)
	if err != nil {
		return nil, err
	}

	bs, _ := common.Marshal(m)
	if bs == nil {
		bs = []byte("{}")
	}
	_ = common.RedisSet(key, string(bs), userModelPriceCacheTTL)

	return m, nil
}

func invalidateUserModelPriceCache(userId int) {
	if !common.RedisEnabled {
		return
	}
	_ = common.RedisDel(userModelPriceCacheKey(userId))
}

// LookupUserModelPrice 一键查 (userId, model) → (price, ok)
// 计费链路最常用，先尝试精确匹配；调用方负责做 FormatMatchingModelName 等规范化（
// 这里直接按规范化后的模型名查询）
func LookupUserModelPrice(userId int, modelName string) (float64, bool) {
	if userId <= 0 || modelName == "" {
		return 0, false
	}
	m, err := GetUserModelPriceCached(userId)
	if err != nil || m == nil {
		return 0, false
	}
	p, ok := m[modelName]
	return p, ok
}
