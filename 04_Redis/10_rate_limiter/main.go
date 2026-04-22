// 案例10：Redis 限流器
// 知识点：固定窗口 / 滑动窗口 / 令牌桶
//
// 为什么需要限流：
//
//	防止恶意请求、保护下游服务、保证系统稳定性
//
// 三种算法对比：
//
//	① 固定窗口：实现简单，但窗口边界有突刺问题
//	② 滑动窗口：更平滑，用 ZSet 实现，内存占用稍高
//	③ 令牌桶：允许一定突发流量，最接近真实业务场景
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()
	rdb := newClient()
	defer rdb.Close()

	fmt.Println("=== case1: 固定窗口限流 ===")
	case1_fixedWindow(ctx, rdb)

	fmt.Println("\n=== case2: 滑动窗口限流 ===")
	case2_slidingWindow(ctx, rdb)

	fmt.Println("\n=== case3: 令牌桶限流 ===")
	case3_tokenBucket(ctx, rdb)

	fmt.Println("\n=== case4: 综合对比 ===")
	case4_compare(ctx, rdb)
}

type FixedWindowLimiter struct {
	rdb    *redis.Client
	limit  int64
	window time.Duration
}

func NewFixedWindowLimiter(rdb *redis.Client, limit int64, window time.Duration) *FixedWindowLimiter {
	return &FixedWindowLimiter{rdb: rdb, limit: limit, window: window}
}

func (l *FixedWindowLimiter) Allow(ctx context.Context, key string) (bool, int64) {
	now := time.Now().Unix()
	windowKey := fmt.Sprintf("%s:%d", key, now/int64(l.window.Seconds()))

	pipe := l.rdb.Pipeline()
	incrCmd := pipe.Incr(ctx, windowKey)
	pipe.Expire(ctx, windowKey, l.window*2)
	pipe.Exec(ctx)

	count := incrCmd.Val()
	return count <= l.limit, count
}

func case1_fixedWindow(ctx context.Context, rdb *redis.Client) {
	limiter := NewFixedWindowLimiter(rdb, 5, 10*time.Second)
	key := "limit:fixed:user:1001"
	rdb.Del(ctx, key)

	fmt.Println("固定窗口限流（10秒内最多5次）:")
	for i := 1; i <= 8; i++ {
		allowed, count := limiter.Allow(ctx, key)
		status := "✅ 允许"
		if !allowed {
			status = "❌ 拒绝"
		}
		fmt.Printf("  第%d次请求 → 当前计数:%d %s\n", i, count, status)
	}
}

type SlidingWindowLimiter struct {
	rdb    *redis.Client
	limit  int64
	window time.Duration
}

func NewSlidingWindowLimiter(rdb *redis.Client, limit int64, window time.Duration) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{rdb: rdb, limit: limit, window: window}
}

func (l *SlidingWindowLimiter) Allow(ctx context.Context, key string) (bool, int64) {
	now := time.Now().UnixMilli()
	windowStart := now - l.window.Milliseconds()

	script := `
		local key = KEYS[1]
		local now = tonumber(ARGV[1])
		local window_start = tonumber(ARGV[2])
		local limit = tonumber(ARGV[3])
		local expire = tonumber(ARGV[4])

		redis.call("ZREMRANGEBYSCORE", key, "-inf", window_start)

		local count = redis.call("ZCARD", key)

		if count < limit then
			redis.call("ZADD", key, now, now .. "-" .. math.random(1000000))
			redis.call("EXPIRE", key, expire)
			return count + 1
		else
			return -1
		end
	`

	result, _ := l.rdb.Eval(ctx, script, []string{key},
		now, windowStart, l.limit, int64(l.window.Seconds())+1,
	).Int64()

	if result == -1 {
		count, _ := l.rdb.ZCard(ctx, key).Result()
		return false, count
	}
	return true, result
}

func case2_slidingWindow(ctx context.Context, rdb *redis.Client) {
	limiter := NewSlidingWindowLimiter(rdb, 5, 10*time.Second)
	key := "limit:sliding:user:1001"
	rdb.Del(ctx, key)

	fmt.Println("滑动窗口限流（10秒内最多5次）:")
	for i := 1; i <= 8; i++ {
		allowed, count := limiter.Allow(ctx, key)
		status := "✅ 允许"
		if !allowed {
			status = "❌ 拒绝"
		}
		fmt.Printf("  第%d次请求 → 当前计数:%d %s\n", i, count, status)
	}

	rdb.Del(ctx, key)
}

type TokenBucketLimiter struct {
	rdb      *redis.Client
	capacity int64
	rate     float64
}

func NewTokenBucketLimiter(rdb *redis.Client, capacity int64, rate float64) *TokenBucketLimiter {
	return &TokenBucketLimiter{rdb: rdb, capacity: capacity, rate: rate}
}

func (l *TokenBucketLimiter) Allow(ctx context.Context, key string) (bool, float64) {
	script := `
		local key = KEYS[1]
		local capacity = tonumber(ARGV[1])
		local rate = tonumber(ARGV[2])
		local now = tonumber(ARGV[3])

		local info = redis.call("HMGET", key, "tokens", "last_time")
		local tokens = tonumber(info[1])
		local last_time = tonumber(info[2])

		if tokens == nil then
			tokens = capacity
			last_time = now
		else
			local elapsed = (now - last_time) / 1000.0
			local add_tokens = elapsed * rate
			tokens = math.min(capacity, tokens + add_tokens)
			last_time = now
		end

		if tokens >= 1 then
			tokens = tokens - 1
			redis.call("HSET", key, "tokens", tokens, "last_time", last_time)
			redis.call("EXPIRE", key, 3600)
			return tokens
		else
			redis.call("HSET", key, "tokens", tokens, "last_time", last_time)
			redis.call("EXPIRE", key, 3600)
			return -1
		end
	`

	now := time.Now().UnixMilli()
	result, _ := l.rdb.Eval(ctx, script, []string{key},
		l.capacity, l.rate, now,
	).Float64()

	if result == -1 {
		return false, 0
	}
	return true, result
}

func case3_tokenBucket(ctx context.Context, rdb *redis.Client) {
	limiter := NewTokenBucketLimiter(rdb, 5, 2)
	key := "limit:token:user:1001"
	rdb.Del(ctx, key)

	fmt.Println("令牌桶限流（容量5，每秒补充2个）:")
	fmt.Println("第一批：快速消耗桶内令牌")
	for i := 1; i <= 7; i++ {
		allowed, remaining := limiter.Allow(ctx, key)
		status := "✅ 允许"
		if !allowed {
			status = "❌ 拒绝"
		}
		fmt.Printf("  第%d次请求 → 剩余令牌:%.0f %s\n", i, remaining, status)
	}

	fmt.Println("等待1秒，令牌补充...")
	time.Sleep(time.Second)

	fmt.Println("第二批：令牌已补充")
	for i := 1; i <= 3; i++ {
		allowed, remaining := limiter.Allow(ctx, key)
		status := "✅ 允许"
		if !allowed {
			status = "❌ 拒绝"
		}
		fmt.Printf("  第%d次请求 → 剩余令牌:%.0f %s\n", i, remaining, status)
	}

	rdb.Del(ctx, key)
}

func case4_compare(ctx context.Context, rdb *redis.Client) {
	fmt.Println("三种限流算法对比:")
	fmt.Println()
	fmt.Printf("  %-16s %-12s %-12s %-16s\n", "算法", "实现复杂度", "突发处理", "边界问题")
	fmt.Println("  " + "─────────────────────────────────────────────────────")
	fmt.Printf("  %-16s %-12s %-12s %-16s\n", "固定窗口", "简单", "不支持", "有边界突刺")
	fmt.Printf("  %-16s %-12s %-12s %-16s\n", "滑动窗口", "中等", "不支持", "无边界问题")
	fmt.Printf("  %-16s %-12s %-12s %-16s\n", "令牌桶", "复杂", "支持", "无边界问题")
	fmt.Println()
	fmt.Println("  选型建议:")
	fmt.Println("  - API 接口限流     → 滑动窗口（精确、无突刺）")
	fmt.Println("  - 消息队列消费限速 → 令牌桶（允许适当突发）")
	fmt.Println("  - 简单计数限流     → 固定窗口（够用就行）")
}

func newClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", DB: 0})
}
