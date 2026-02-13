-- query.lua
-- 查询许可状态的只读操作
--
-- KEYS[1]: 全局许可集合键
-- KEYS[2]: 租户许可集合键（可选，动态传递）
--
-- ARGV[1]: 当前时间戳（毫秒）
--
-- 返回: {globalCount, tenantCount}
--
-- 注意：此脚本为纯只读，不执行清理操作。
-- 过期许可的清理由 acquire/extend 的写路径负责。

local globalKey = KEYS[1]
-- KEYS[2] 动态传递，可能不存在（Redis Cluster 兼容）
local tenantKey = KEYS[2]
local hasTenantKey = tenantKey ~= nil and tenantKey ~= ''

local now = tonumber(ARGV[1])

-- 统计未过期的全局许可（score > now 表示未过期）
-- 使用 '(' .. now 表示开区间，排除恰好等于 now 的过期条目
local globalCount = redis.call('ZCOUNT', globalKey, '(' .. now, '+inf')

-- 统计未过期的租户许可
local tenantCount = 0
if hasTenantKey then
    tenantCount = redis.call('ZCOUNT', tenantKey, '(' .. now, '+inf')
end

return {globalCount, tenantCount}
