-- extend.lua
-- 续期许可的原子操作
--
-- KEYS[1]: 全局许可集合键
-- KEYS[2]: 租户许可集合键（可选，动态传递）
--
-- ARGV[1]: 当前时间戳（毫秒）
-- ARGV[2]: 新的过期时间戳（毫秒）
-- ARGV[3]: 许可 ID
-- ARGV[4]: 键过期余量（毫秒）
--
-- 返回: {status}
--   - status: 0=成功, 3=未持有

local globalKey = KEYS[1]
-- KEYS[2] 动态传递，可能不存在（Redis Cluster 兼容）
local tenantKey = KEYS[2]
local hasTenantKey = tenantKey ~= nil and tenantKey ~= ''

local now = tonumber(ARGV[1])
local newExpireAt = tonumber(ARGV[2])
local permitID = ARGV[3]
local keyTTLMargin = tonumber(ARGV[4])

-- 检查许可是否存在
local score = redis.call('ZSCORE', globalKey, permitID)
if not score then
    return {3}
end

-- 检查是否已过期（使用 <= 语义，与 local.go 保持一致）
if tonumber(score) <= now then
    redis.call('ZREM', globalKey, permitID)
    if hasTenantKey then
        redis.call('ZREM', tenantKey, permitID)
    end
    return {3}
end

-- 防御性检查：新的过期时间必须在当前时间之后
-- 正常情况下 Go 侧保证 newExpireAt > now，但时钟极端回拨时可能违反
if newExpireAt <= now then
    return {3}
end

-- 更新过期时间
redis.call('ZADD', globalKey, newExpireAt, permitID)
if hasTenantKey then
    redis.call('ZADD', tenantKey, newExpireAt, permitID)
end

-- 更新键过期时间（只延长，不缩短，防止短 TTL 许可影响长 TTL 许可）
local ttlMs = newExpireAt - now + keyTTLMargin
local ttlSec = math.ceil(ttlMs / 1000)
local currentTTL = redis.call('TTL', globalKey)
-- TTL 返回 -1 表示键永不过期，-2 表示键不存在，正数表示剩余秒数
-- 只有当新 TTL 大于当前 TTL 时才更新（或键不存在/永不过期时设置）
if currentTTL < 0 or ttlSec > currentTTL then
    redis.call('EXPIRE', globalKey, ttlSec)
end
if hasTenantKey then
    local tenantCurrentTTL = redis.call('TTL', tenantKey)
    if tenantCurrentTTL < 0 or ttlSec > tenantCurrentTTL then
        redis.call('EXPIRE', tenantKey, ttlSec)
    end
end

return {0}
