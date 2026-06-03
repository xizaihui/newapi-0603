/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package service

import (
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// Auto-group cooldown (a lightweight circuit breaker) for the priority-ordered
// multi-group / "auto" routing machinery (feat4).
//
// Background: a multi-group key stores its groups as an ordered, comma-separated
// list (e.g. "openai-aggregate,openai-official"). CacheGetRandomSatisfiedChannel
// already tries those groups strictly in order, only moving to the next group
// once the current one is exhausted. That ordering is per-request, though — every
// new request restarts at the first group, so a group whose upstream is currently
// down keeps getting hit first and failing over again and again.
//
// This file adds cross-request memory: when a request fails over FROM a group
// (its channels were tried and could not serve the request, so routing moved on
// to the next group), that (group, model) pair is put into a short cooldown.
// While it is cooling down, subsequent requests skip it when building the ordered
// candidate list, so traffic goes straight to the next healthy group. After the
// cooldown expires the group is tried again; if it then serves a request the
// cooldown is cleared immediately, so a recovered group returns to the front of
// the priority order.
//
// Storage is Redis when enabled (shared across all nodes), with an in-process
// fallback so it still works on single-node / no-Redis deployments. No database
// schema is involved, which keeps it safe on slave nodes that skip migrations.
//
// Scope is intentionally global per (group, model): a group's upstream being
// down is a property of that upstream, not of one particular key, so a shared
// breaker lets the whole cluster stop hammering a dead group.

const autoGroupCooldownRedisPrefix = "auto_group_cooldown:"

// autoGroupCooldownDuration is how long a failed group is skipped. Defaults to
// 10 minutes; override with AUTO_GROUP_COOLDOWN_SECONDS (0 disables the feature).
var autoGroupCooldownDuration = func() time.Duration {
	if v := os.Getenv("AUTO_GROUP_COOLDOWN_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 10 * time.Minute
}()

// in-process fallback store: key -> expiry time (used only when Redis is disabled)
var (
	autoGroupCooldownMu    sync.Mutex
	autoGroupCooldownStore = make(map[string]time.Time)
)

// autoGroupCooldownKey builds the storage key for a (group, model) pair. \x1f
// (unit separator) cannot appear in a group or model name, so there is no
// risk of two distinct pairs colliding onto the same key.
func autoGroupCooldownKey(group, model string) string {
	return group + "\x1f" + model
}

// MarkAutoGroupCooldown puts (group, model) into cooldown for
// autoGroupCooldownDuration. No-op when the feature is disabled or group is empty.
func MarkAutoGroupCooldown(group, model string) {
	if group == "" || autoGroupCooldownDuration <= 0 {
		return
	}
	key := autoGroupCooldownKey(group, model)
	if common.RedisEnabled {
		if err := common.RedisSet(autoGroupCooldownRedisPrefix+key, "1", autoGroupCooldownDuration); err == nil {
			return
		}
		// Redis write failed → fall through to the in-process store so the
		// breaker still works for this node.
	}
	autoGroupCooldownMu.Lock()
	autoGroupCooldownStore[key] = time.Now().Add(autoGroupCooldownDuration)
	autoGroupCooldownMu.Unlock()
}

// IsAutoGroupInCooldown reports whether (group, model) is currently cooling down.
func IsAutoGroupInCooldown(group, model string) bool {
	if group == "" || autoGroupCooldownDuration <= 0 {
		return false
	}
	key := autoGroupCooldownKey(group, model)
	if common.RedisEnabled {
		if _, err := common.RedisGet(autoGroupCooldownRedisPrefix + key); err == nil {
			return true
		}
		// Key missing or Redis error → not cooling down (also check the local
		// fallback in case it was written there during a Redis outage).
	}
	autoGroupCooldownMu.Lock()
	defer autoGroupCooldownMu.Unlock()
	exp, ok := autoGroupCooldownStore[key]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(autoGroupCooldownStore, key)
		return false
	}
	return true
}

// ClearAutoGroupCooldown removes any cooldown on (group, model). Called when a
// group serves a request successfully, so a recovered group is preferred again.
func ClearAutoGroupCooldown(group, model string) {
	if group == "" {
		return
	}
	key := autoGroupCooldownKey(group, model)
	if common.RedisEnabled {
		_ = common.RedisDel(autoGroupCooldownRedisPrefix + key)
	}
	autoGroupCooldownMu.Lock()
	delete(autoGroupCooldownStore, key)
	autoGroupCooldownMu.Unlock()
}

// FilterAvailableAutoGroups returns the subset of groups (preserving the priority
// order) that are not currently in cooldown for the given model. If every group
// is cooling down it returns the original list unchanged, so a request never
// fails merely because all candidates happen to be in cooldown — in that case the
// normal in-request retry/failover still applies.
func FilterAvailableAutoGroups(groups []string, model string) []string {
	if len(groups) <= 1 || autoGroupCooldownDuration <= 0 {
		return groups
	}
	available := make([]string, 0, len(groups))
	for _, g := range groups {
		if !IsAutoGroupInCooldown(g, model) {
			available = append(available, g)
		}
	}
	if len(available) == 0 {
		return groups
	}
	return available
}
