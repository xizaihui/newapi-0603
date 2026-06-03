package service

import (
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// SplitUserGroups 把 user.Group 字符串按逗号分隔成多分组列表。
// 空白会被 trim 掉；纯空字符串返回空切片。
// 多分组语义：一个用户可被授权使用多个分组的模型；第一个分组为主分组（用于默认 UsingGroup）。
func SplitUserGroups(userGroup string) []string {
	if userGroup == "" {
		return nil
	}
	parts := strings.Split(userGroup, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		g := strings.TrimSpace(p)
		if g == "" {
			continue
		}
		if _, dup := seen[g]; dup {
			continue
		}
		seen[g] = struct{}{}
		result = append(result, g)
	}
	return result
}

// GetUserPrimaryGroup 获取用户的主分组（逗号分隔列表的第一项）。
// 用于计费的默认 UsingGroup。
func GetUserPrimaryGroup(userGroup string) string {
	groups := SplitUserGroups(userGroup)
	if len(groups) == 0 {
		return ""
	}
	return groups[0]
}

// applyGroupSpecialUsable 把单个分组的 GroupSpecialUsableGroup 配置 merge 到 groupsCopy。
// 用法：先把用户授权分组放入 groupsCopy，再对每个授权分组应用 special 规则
// (+: 添加 / -: 移除 / 直接添加)。
func applyGroupSpecialUsable(groupsCopy map[string]string, userGroup string) {
	specialSettings, b := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Get(userGroup)
	if !b {
		return
	}
	for specialGroup, desc := range specialSettings {
		if strings.HasPrefix(specialGroup, "-:") {
			groupToRemove := strings.TrimPrefix(specialGroup, "-:")
			delete(groupsCopy, groupToRemove)
		} else if strings.HasPrefix(specialGroup, "+:") {
			groupToAdd := strings.TrimPrefix(specialGroup, "+:")
			groupsCopy[groupToAdd] = desc
		} else {
			groupsCopy[specialGroup] = desc
		}
	}
}

// GetUserUsableGroups 返回用户严格可用的分组及描述。
//
// **新语义 (2026-05-22, Phase 1.5)**:
//   - userGroup 为空（匿名/未配置）→ 返回 default 分组（公开预览 fallback）
//   - userGroup 非空 → 严格只返回该用户被显式授权的分组
//     （逗号分隔多分组），再叠加 GroupSpecialUsableGroup 的 +:/-: 规则
//
// 不再依赖全局 setting.UserUsableGroupsCopy 作为基础集合 —— 那个配置
// 现在只作为 setting 元数据（描述/排序），不决定用户可见性。
//
// 管理员特权由 caller (controller) 自行加（不在这层处理）。
func GetUserUsableGroups(userGroup string) map[string]string {
	result := make(map[string]string)
	settingGroups := setting.GetUserUsableGroupsCopy()

	userGroups := SplitUserGroups(userGroup)
	if len(userGroups) == 0 {
		// 匿名/无配置 fallback：default 分组（如果 setting 里有描述就用，否则给个默认）
		desc, ok := settingGroups["default"]
		if !ok {
			desc = "默认分组"
		}
		result["default"] = desc
		return result
	}

	for _, g := range userGroups {
		// 1) 加入授权分组本身
		desc, ok := settingGroups[g]
		if !ok {
			desc = "用户分组"
		}
		result[g] = desc
		// 2) 应用该分组的 special 规则（+: / -: / 直接添加）
		applyGroupSpecialUsable(result, g)
	}

	return result
}

// GetAllGroupsForAdmin 返回所有全局已知分组（用于管理员特权 view-all）。
// 数据源：GroupRatio 配置（全局所有定义了倍率的分组），描述从 UserUsableGroups 取，没有则用分组名。
func GetAllGroupsForAdmin() map[string]string {
	result := make(map[string]string)
	settingGroups := setting.GetUserUsableGroupsCopy()
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		if desc, ok := settingGroups[groupName]; ok {
			result[groupName] = desc
		} else {
			result[groupName] = groupName
		}
	}
	return result
}

func GroupInUserUsableGroups(userGroup, groupName string) bool {
	_, ok := GetUserUsableGroups(userGroup)[groupName]
	return ok
}

// GetUserAutoGroup 根据用户分组获取自动分组设置
func GetUserAutoGroup(userGroup string) []string {
	groups := GetUserUsableGroups(userGroup)
	autoGroups := make([]string, 0)
	for _, group := range setting.GetAutoGroups() {
		if _, ok := groups[group]; ok {
			autoGroups = append(autoGroups, group)
		}
	}
	return autoGroups
}

// GetUserGroupRatio 获取用户使用某个分组的倍率（旧签名，保留兼容）
// 优先级：用户组级覆盖 > 全局；不查用户级覆盖（无 userId 信息）
// 计费链路应该用 GetUserGroupRatioWithUser 走完整 3 层查询。
func GetUserGroupRatio(userGroup, group string) float64 {
	for _, g := range SplitUserGroups(userGroup) {
		if ratio, ok := ratio_setting.GetGroupGroupRatio(g, group); ok {
			return ratio
		}
	}
	return ratio_setting.GetGroupRatio(group)
}

// GetUserGroupRatioWithUser Phase 2 完整查询，加用户级覆盖层
// 查找顺序：
//  1. UserGroupRatio[userId][group] (用户级覆盖，新)
//  2. GroupGroupRatio[userGroup][group] (用户组级覆盖，原有)
//  3. GroupRatio[group] (全局)
//
// userId <= 0 时跳过第 1 层；行为完全等价 GetUserGroupRatio。
// 计费链路必须走这个函数（不要走旧的 GetUserGroupRatio）。
func GetUserGroupRatioWithUser(userId int, userGroup, group string) float64 {
	if userId > 0 {
		if ratio, ok := model.LookupUserGroupRatio(userId, group); ok {
			return ratio
		}
	}
	for _, g := range SplitUserGroups(userGroup) {
		if ratio, ok := ratio_setting.GetGroupGroupRatio(g, group); ok {
			return ratio
		}
	}
	return ratio_setting.GetGroupRatio(group)
}
