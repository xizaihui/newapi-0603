/*
Per-user group ratio admin endpoints (Phase 2)

Routes (registered in router/api-router.go under adminRoute):
  GET    /api/user/:id/group_ratios            列出用户的所有覆盖
  PUT    /api/user/:id/group_ratios            批量设置（map[group]ratio）
  DELETE /api/user/:id/group_ratios/:group     删除某分组覆盖
*/
package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetUserGroupRatios GET /api/user/:id/group_ratios
func GetUserGroupRatios(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	m, err := model.GetUserGroupRatioMap(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if m == nil {
		m = map[string]float64{}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    m,
	})
}

// SetUserGroupRatiosRequest body: {"ratios": {"group_a": 0.8, "group_b": 0.9}}
// 空 map 等同清空所有覆盖。
type SetUserGroupRatiosRequest struct {
	Ratios map[string]float64 `json:"ratios"`
}

// SetUserGroupRatios PUT /api/user/:id/group_ratios
func SetUserGroupRatios(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	var req SetUserGroupRatiosRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	// 清洗：trim group name，去空 key，校验 ratio >= 0
	cleaned := make(map[string]float64)
	for g, r := range req.Ratios {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		if r < 0 {
			common.ApiError(c, errors.New("ratio must be >= 0 for group: "+g))
			return
		}
		cleaned[g] = r
	}
	if err := model.SetUserGroupRatios(id, cleaned); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// DeleteUserGroupRatio DELETE /api/user/:id/group_ratios/:group
func DeleteUserGroupRatio(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	group := strings.TrimSpace(c.Param("group"))
	if group == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := model.DeleteUserGroupRatio(id, group); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}
