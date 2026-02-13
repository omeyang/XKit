-- release.lua
-- 释放许可的原子操作
--
-- KEYS[1]: 全局许可集合键
-- KEYS[2]: 租户许可集合键（可选，动态传递）
--
-- ARGV[1]: 许可 ID
--
-- 返回: {status, removed}
--   - status: 0=成功, 3=未持有
--   - removed: 删除的许可数

local globalKey = KEYS[1]
-- KEYS[2] 动态传递，可能不存在（Redis Cluster 兼容）
local tenantKey = KEYS[2]
local hasTenantKey = tenantKey ~= nil and tenantKey ~= ''

local permitID = ARGV[1]

-- 从全局集合删除
local removed = redis.call('ZREM', globalKey, permitID)

-- 从租户集合删除
if hasTenantKey then
    redis.call('ZREM', tenantKey, permitID)
end

if removed == 0 then
    return {3, 0}
end

return {0, removed}
