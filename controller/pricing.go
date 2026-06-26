package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func filterPricingByUsableGroups(pricing []model.Pricing, usableGroup map[string]string) []model.Pricing {
	if len(pricing) == 0 {
		return pricing
	}
	if len(usableGroup) == 0 {
		return []model.Pricing{}
	}

	filtered := make([]model.Pricing, 0, len(pricing))
	for _, item := range pricing {
		if common.StringsContains(item.EnableGroup, "all") {
			filtered = append(filtered, item)
			continue
		}
		for _, group := range item.EnableGroup {
			if _, ok := usableGroup[group]; ok {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered
}

func GetPricing(c *gin.Context) {
	pricing := model.GetPricing()
	userId, exists := c.Get("id")
	usableGroup := map[string]string{}
	groupRatio := map[string]float64{}
	for s, f := range ratio_setting.GetGroupRatioCopy() {
		groupRatio[s] = f
	}
	var group string
	isAdmin := false
	uid := 0
	// 登录用户的「专属分组倍率」覆盖表（一次性取出，避免逐分组查库）
	var userGroupRatioMap map[string]float64
	if exists {
		uid = userId.(int)
		user, err := model.GetUserCache(uid)
		if err == nil {
			group = user.Group
			isAdmin = c.GetInt("role") >= common.RoleAdminUser
			userGroupRatioMap, _ = model.GetUserGroupRatioCached(uid)
			// 价格展示必须与计费链路 (relay/helper.HandleGroupRatio) 完全一致，
			// 分组倍率优先级：UserGroupRatio[uid][g]（专属分组倍率）
			//   > GroupGroupRatio[userGroup][g]（用户组级覆盖）
			//   > 全局 GroupRatio[g]
			for g := range groupRatio {
				if r, ok := userGroupRatioMap[g]; ok {
					groupRatio[g] = r
					continue
				}
				if ratio, ok := ratio_setting.GetGroupGroupRatio(group, g); ok {
					groupRatio[g] = ratio
				}
			}
		}
	}

	// Phase 1.5 严格语义：
	//   - 管理员 → 看所有分组
	//   - 普通用户 → 严格按 user.Group 多分组
	//   - 匿名 → default 分组（公开预览）
	if isAdmin {
		usableGroup = service.GetAllGroupsForAdmin()
	} else {
		usableGroup = service.GetUserUsableGroups(group)
	}
	pricing = filterPricingByUsableGroups(pricing, usableGroup)
	// check groupRatio contains usableGroup
	for group := range ratio_setting.GetGroupRatioCopy() {
		if _, ok := usableGroup[group]; !ok {
			delete(groupRatio, group)
		}
	}

	// 应用「专属模型倍率」（用户级按次价格覆盖），与计费链路 relay/helper.GetEffectiveModelPrice 一致。
	// filterPricingByUsableGroups 已返回结构体副本，可安全就地修改，不会污染全局 pricing 缓存。
	if uid > 0 {
		if priceMap, err := model.GetUserModelPriceCached(uid); err == nil && len(priceMap) > 0 {
			for i := range pricing {
				name := pricing[i].ModelName
				normalized := ratio_setting.FormatMatchingModelName(name)
				price, ok := priceMap[normalized]
				if !ok && normalized != name {
					price, ok = priceMap[name]
				}
				if ok {
					// 用户专属为按次价格：切换为按次计费展示。
					pricing[i].ModelPrice = price
					pricing[i].QuotaType = 1
					// 仅当该模型不存在「按 token」分组时才清空按量倍率，使其与全局按次价格
					// 模型在 model/pricing.go 中的表示保持一致。
					// 方案A 渠道级覆盖可能使部分分组按 token 计费(group_billing_modes 含 per_token)，
					// 且渠道级 per_token 覆盖在计费链路中优先于用户专属按次价格
					// (见 relay/helper.ModelPriceHelper：override=per_token 时 usePrice=false)，
					// 这些分组实际仍按 model_ratio 计费，必须保留 model_ratio/completion_ratio，
					// 否则前端按 token 分组价格会显示为 $0。
					hasPerTokenGroup := false
					for _, mode := range pricing[i].GroupBillingModes {
						if mode == dto.ChannelBillingModePerToken {
							hasPerTokenGroup = true
							break
						}
					}
					if !hasPerTokenGroup {
						pricing[i].ModelRatio = 0
						pricing[i].CompletionRatio = 0
					}
				}
			}
		}
	}

	c.JSON(200, gin.H{
		"success":            true,
		"data":               pricing,
		"vendors":            model.GetVendors(),
		"group_ratio":        groupRatio,
		"usable_group":       usableGroup,
		"supported_endpoint": model.GetSupportedEndpointMap(),
		"auto_groups":        service.GetUserAutoGroup(group),
		"pricing_version":    "a42d372ccf0b5dd13ecf71203521f9d2",
	})
}

func ResetModelRatio(c *gin.Context) {
	defaultStr := ratio_setting.DefaultModelRatio2JSONString()
	err := model.UpdateOption("ModelRatio", defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	err = ratio_setting.UpdateModelRatioByJSONString(defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "重置模型倍率成功",
	})
}
