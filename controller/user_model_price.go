/*
Per-user model price admin endpoints (Phase 3)

Routes (registered in router/api-router.go under adminRoute):
  GET    /api/user/:id/model_prices            列出用户的所有模型价格覆盖
  PUT    /api/user/:id/model_prices            批量设置 [{model_name, price, note}, ...]
  DELETE /api/user/:id/model_prices/:model     删除某模型覆盖
*/
package controller

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetUserModelPrices GET /api/user/:id/model_prices
// Response: {"success":true,"data":[{model_name, price, note, created_at, updated_at}, ...]}
func GetUserModelPrices(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	items, err := model.GetUserModelPrices(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if items == nil {
		items = []model.UserModelPrice{}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
	})
}

// SetUserModelPricesRequest body: {"prices":[{"model_name":"...","price":0.25,"note":"VIP"}, ...]}
// 空数组等同清空所有覆盖。
type SetUserModelPricesRequest struct {
	Prices []model.SetUserModelPriceItem `json:"prices"`
}

// SetUserModelPrices PUT /api/user/:id/model_prices
func SetUserModelPrices(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	var req SetUserModelPricesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	// 清洗：trim model name，去空 key，校验 price >= 0
	cleaned := make([]model.SetUserModelPriceItem, 0, len(req.Prices))
	for _, it := range req.Prices {
		name := strings.TrimSpace(it.ModelName)
		if name == "" {
			continue
		}
		if it.Price < 0 {
			common.ApiError(c, errors.New("price must be >= 0 for model: "+name))
			return
		}
		cleaned = append(cleaned, model.SetUserModelPriceItem{
			ModelName: name,
			Price:     it.Price,
			Note:      strings.TrimSpace(it.Note),
		})
	}
	if err := model.SetUserModelPrices(id, cleaned); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// DeleteUserModelPrice DELETE /api/user/:id/model_prices/:model
// model 名可能带特殊字符（如 :compact），需 URL-decode 由 gin 自动处理 + 防御性 trim
func DeleteUserModelPrice(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	raw := c.Param("model")
	// gin route 参数已经 url-decode，但防御性再做一次以兼容客户端双重编码
	if decoded, err := url.QueryUnescape(raw); err == nil {
		raw = decoded
	}
	modelName := strings.TrimSpace(raw)
	if modelName == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := model.DeleteUserModelPrice(id, modelName); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}
