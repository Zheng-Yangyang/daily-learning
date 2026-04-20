package main

import (
	"fmt"
	"sync"
	"time"
)

// TokenBucket 令牌桶结构体
type TokenBucket struct {
	rate       float64    // 每秒补充的令牌数
	capacity   float64    // 桶的最大容量
	tokens     float64    // 当前令牌数
	lastRefill time.Time  // 上次补充时间
	mu         sync.Mutex // 并发保护
}

func NewTokenBucket(rate, capacity float64) *TokenBucket {
	return &TokenBucket{
		rate:       rate,
		capacity:   capacity,
		tokens:     capacity, // 初始时桶是满的
		lastRefill: time.Now(),
	}
}

// Allow 尝试消耗一个令牌，返回是否放行
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()

	// 核心逻辑：按照流逝时间"懒补充"令牌
	// 不需要定时器，每次请求进来时才计算应该补多少
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.rate
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity // 上限是桶容量，多余的溢出丢弃
	}
	tb.lastRefill = now

	if tb.tokens >= 1 {
		tb.tokens-- // 消耗一个令牌，放行
		return true
	}
	return false // 桶空了，拒绝这次请求
}

func main() {
	// 每秒补充 5 个令牌，桶最多装 5 个
	limiter := NewTokenBucket(5, 5)

	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed, denied := 0, 0

	fmt.Println("========================================")
	fmt.Println(" 场景一：瞬间打入 20 个并发请求")
	fmt.Println(" 桶容量=5，速率=5/s，预期只有5个能通过")
	fmt.Println("========================================")

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if limiter.Allow() {
				mu.Lock()
				allowed++
				mu.Unlock()
				fmt.Printf("  请求 %2d: ✓ 通过\n", id)
			} else {
				mu.Lock()
				denied++
				mu.Unlock()
				fmt.Printf("  请求 %2d: ✗ 被限流\n", id)
			}
		}(i + 1)
	}
	wg.Wait()
	fmt.Printf("\n  结果：通过 %d 个，拒绝 %d 个\n", allowed, denied)

	// ---- 等 1 秒，桶自动补充令牌 ----
	fmt.Println("\n========================================")
	fmt.Println(" 场景二：等待 1 秒后再打 8 个请求")
	fmt.Println(" 1秒内补充了5个令牌，预期5个通过，3个被拒")
	fmt.Println("========================================")
	time.Sleep(1 * time.Second)

	allowed2, denied2 := 0, 0
	for i := 0; i < 8; i++ {
		if limiter.Allow() {
			allowed2++
			fmt.Printf("  请求 %2d: ✓ 通过\n", i+1)
		} else {
			denied2++
			fmt.Printf("  请求 %2d: ✗ 被限流\n", i+1)
		}
	}
	fmt.Printf("\n  结果：通过 %d 个，拒绝 %d 个\n", allowed2, denied2)

	// ---- 慢速请求，始终不超过速率 ----
	fmt.Println("\n========================================")
	fmt.Println(" 场景三：匀速每 300ms 一个请求，共 6 个")
	fmt.Println(" 速率=5/s，300ms能补0.5个，不会被限流前几个")
	fmt.Println("========================================")
	time.Sleep(1 * time.Second) // 先让桶补满

	for i := 0; i < 6; i++ {
		if limiter.Allow() {
			fmt.Printf("  请求 %2d: ✓ 通过\n", i+1)
		} else {
			fmt.Printf("  请求 %2d: ✗ 被限流\n", i+1)
		}
		time.Sleep(300 * time.Millisecond)
	}

	fmt.Println("\n全部演示完毕。")
	fmt.Println("\n思考题：如果 rate=5 但 capacity=10，场景一会有什么不同？")
}
