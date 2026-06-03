package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func GetGroups(c *gin.Context) {
	groupNames := make([]string, 0)
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		groupNames = append(groupNames, groupName)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groupNames,
	})
}

func GetUserGroups(c *gin.Context) {
	usableGroups := make(map[string]map[string]interface{})
	userGroup := ""
	userId := c.GetInt("id")
	userRole := c.GetInt("role")
	userGroup, _ = model.GetUserGroup(userId, false)
	// Phase 1.5: 管理员看全部分组；普通用户严格按 user.Group
	var userUsableGroups map[string]string
	if userRole >= common.RoleAdminUser {
		userUsableGroups = service.GetAllGroupsForAdmin()
	} else {
		userUsableGroups = service.GetUserUsableGroups(userGroup)
	}
	for groupName, _ := range ratio_setting.GetGroupRatioCopy() {
		// UserUsableGroups contains the groups that the user can use
		if desc, ok := userUsableGroups[groupName]; ok {
			// Phase 2: 用 GetUserGroupRatioWithUser 走完整 3 层查询
			// 让用户在 key 创建页能看到自己的专属倍率
			usableGroups[groupName] = map[string]interface{}{
				"ratio": service.GetUserGroupRatioWithUser(userId, userGroup, groupName),
				"desc":  desc,
			}
		}
	}
	if _, ok := userUsableGroups["auto"]; ok {
		usableGroups["auto"] = map[string]interface{}{
			"ratio": "自动",
			"desc":  setting.GetUsableGroupDescription("auto"),
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    usableGroups,
	})
}
