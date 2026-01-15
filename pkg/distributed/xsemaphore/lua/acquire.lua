-- acquire.lua
-- 获取许可的原子操作
--
-- KEYS[1]: 全局许可集合键 {prefix}:{resource}:permits
-- KEYS[2]: 租户许可集合键 {prefix}:{resource}:t:{tenantID}（可选，动态传递）
--
-- ARGV[1]: 当前时间戳（毫秒）
-- ARGV[2]: 许可过期时间戳（毫秒）
-- ARGV[3]: 许可 ID
-- ARGV[4]: 全局容量上限
-- ARGV[5]: 租户配额上限（0 表示不限制）
-- ARGV[6]: 键过期余量（毫秒）
--
-- 返回: {status, globalCount, tenantCount}
--   - status: 0=成功, 1=全局容量满, 2=租户配额满
--   - globalCount: 当前全局许可数
--   - tenantCount: 当前租户许可数（未设置租户时为 0）

local globalKey = KEYS[1]
-- KEYS[2] 动态传递，可能不存在（Redis Cluster 兼容）
local tenantKey = KEYS[2]
local hasTenantKey = tenantKey ~= nil and tenantKey ~= ''

local now = tonumber(ARGV[1])
local expireAt = tonumber(ARGV[2])
local permitID = ARGV[3]
local capacity = tonumber(ARGV[4])
local tenantQuota = tonumber(ARGV[5])
local keyTTLMargin = tonumber(ARGV[6])

-- 1. 清理过期的全局许可
redis.call('ZREMRANGEBYSCORE', globalKey, '-inf', now)

-- 2. 检查全局容量
local globalCount = redis.call('ZCARD', globalKey)
if globalCount >= capacity then
    return {1, globalCount, 0}
end

-- 3. 如果设置了租户配额，检查租户
local tenantCount = 0
if hasTenantKey and tenantQuota > 0 then
    redis.call('ZREMRANGEBYSCORE', tenantKey, '-inf', now)
    tenantCount = redis.call('ZCARD', tenantKey)
    if tenantCount >= tenantQuota then
        return {2, globalCount, tenantCount}
    end
end

-- 4. 添加许可
redis.call('ZADD', globalKey, expireAt, permitID)
if hasTenantKey and tenantQuota > 0 then
    redis.call('ZADD', tenantKey, expireAt, permitID)
end

-- 5. 设置键过期时间（只延长，不缩短，防止短 TTL 许可影响长 TTL 许可）
local ttlMs = expireAt - now + keyTTLMargin
local ttlSec = math.ceil(ttlMs / 1000)
local currentTTL = redis.call('TTL', globalKey)
-- TTL 返回 -1 表示键永不过期，-2 表示键不存在，正数表示剩余秒数
-- 只有当新 TTL 大于当前 TTL 时才更新（或键不存在/永不过期时设置）
if currentTTL < 0 or ttlSec > currentTTL then
    redis.call('EXPIRE', globalKey, ttlSec)
end
if hasTenantKey and tenantQuota > 0 then
    local tenantCurrentTTL = redis.call('TTL', tenantKey)
    if tenantCurrentTTL < 0 or ttlSec > tenantCurrentTTL then
        redis.call('EXPIRE', tenantKey, ttlSec)
    end
end

-- 修正返回值：tenantCount 只有在启用租户配额时才加 1
local newTenantCount = tenantCount
if hasTenantKey and tenantQuota > 0 then
    newTenantCount = tenantCount + 1
end
return {0, globalCount + 1, newTenantCount}
