// 案例07：缓存穿透 / 击穿 / 雪崩防护
//
// 三大缓存问题：
//
//	① 缓存穿透：查询不存在的数据，每次都打到DB
//	   解决：缓存空值 or 布隆过滤器
//	② 缓存击穿：热点key过期，大量请求同时打到DB
//	   解决：分布式锁 + 单飞(singleflight)
//	③ 缓存雪崩：大量key同时过期，DB被打垮
//	   解决：TTL加随机抖动
package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// 模拟数据库
var mockDB = map[string]string{
	"product:1001": "iPhone 15",
	"product:1002": "MacBook Pro",
	"product:1003": "iPad Air",
}

// 统计 DB 查询次数
var dbQueryCount int64

func queryDB(key string) (string, error) {
	atomic.AddInt64(&dbQueryCount, 1)
	time.Sleep(50 * time.Millisecond) // 模拟 DB 查询耗时
	val, ok := mockDB[key]
	if !ok {
		return "", errors.New("not found")
	}
	return val, nil
}

func main() {
	ctx := context.Background()
	rdb := newClient()
	defer rdb.Close()

	fmt.Println("=== case1: 缓存穿透 —— 缓存空值方案 ===")
	case1_cachePenetration(ctx, rdb)

	fmt.Println("\n=== case2: 缓存击穿 —— singleflight 方案 ===")
	case2_cacheBreakdown(ctx, rdb)

	fmt.Println("\n=== case3: 缓存雪崩 —— TTL 随机抖动 ===")
	case3_cacheAvalanche(ctx, rdb)

	fmt.Println("\n=== case4: 综合方案 —— 三种问题一起防护 ===")
	case4_combined(ctx, rdb)
}

// ─────────────────────────────────────────────
// case1: 缓存穿透
// 问题：查询一个根本不存在的 key，缓存永远miss，每次都查DB
// 方案：查DB返回空时，缓存一个空值，设短TTL
// ─────────────────────────────────────────────
func case1_cachePenetration(ctx context.Context, rdb *redis.Client) {
	const nullValue = "__NULL__" // 空值占位符
	const nullTTL = 30 * time.Second

	getWithNullCache := func(key string) (string, error) {
		// 1. 查缓存
		val, err := rdb.Get(ctx, key).Result()
		if err == nil {
			if val == nullValue {
				fmt.Printf("  [缓存] %s → 命中空值缓存，直接返回\n", key)
				return "", errors.New("not found")
			}
			fmt.Printf("  [缓存] %s → 命中: %s\n", key, val)
			return val, nil
		}

		// 2. 缓存 miss，查 DB
		fmt.Printf("  [DB]   %s → 查询数据库\n", key)
		dbVal, dbErr := queryDB(key)
		if dbErr != nil {
			// 3. DB 也没有，缓存空值，防止下次继续穿透
			rdb.Set(ctx, key, nullValue, nullTTL)
			fmt.Printf("  [缓存] %s → 写入空值缓存\n", key)
			return "", errors.New("not found")
		}

		// 4. DB 有数据，写入缓存
		rdb.Set(ctx, key, dbVal, 5*time.Minute)
		return dbVal, nil
	}

	rdb.Del(ctx, "product:9999")
	atomic.StoreInt64(&dbQueryCount, 0)

	fmt.Println("第1次查询不存在的 key:")
	getWithNullCache("product:9999")

	fmt.Println("第2次查询（应命中空值缓存，不查DB）:")
	getWithNullCache("product:9999")

	fmt.Println("第3次查询（应命中空值缓存，不查DB）:")
	getWithNullCache("product:9999")

	fmt.Printf("DB 实际查询次数: %d（只查了1次）✅\n", atomic.LoadInt64(&dbQueryCount))
	rdb.Del(ctx, "product:9999")
}

// ─────────────────────────────────────────────
// case2: 缓存击穿
// 问题：热点key过期瞬间，大量并发请求同时查DB
// 方案：singleflight —— 相同key并发请求只执行一次DB查询
// ─────────────────────────────────────────────
func case2_cacheBreakdown(ctx context.Context, rdb *redis.Client) {
	var sf singleflight.Group

	getProduct := func(key string) (string, error) {
		// 1. 查缓存
		val, err := rdb.Get(ctx, key).Result()
		if err == nil {
			return val, nil
		}

		// 2. 缓存 miss，用 singleflight 保证并发下只有1个请求打到DB
		// Do 的语义：相同 key 的并发调用，只有第一个会执行 fn
		// 其余的等待第一个完成后，共享同一个结果
		result, err, shared := sf.Do(key, func() (interface{}, error) {
			fmt.Printf("  [DB] %s → 实际查询数据库\n", key)
			val, err := queryDB(key)
			if err != nil {
				return nil, err
			}
			// 写入缓存
			rdb.Set(ctx, key, val, 5*time.Minute)
			return val, nil
		})

		if err != nil {
			return "", err
		}
		if shared {
			fmt.Printf("  [singleflight] %s → 共享了其他请求的结果\n", key)
		}
		return result.(string), nil
	}

	// 模拟热点key过期后，10个并发请求同时来
	rdb.Del(ctx, "product:1001")
	atomic.StoreInt64(&dbQueryCount, 0)

	var wg sync.WaitGroup
	fmt.Println("10个并发请求同时查询 product:1001（缓存已过期）:")
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			val, _ := getProduct("product:1001")
			fmt.Printf("  goroutine-%d 得到结果: %s\n", id, val)
		}(i)
	}
	wg.Wait()

	fmt.Printf("DB 实际查询次数: %d（10个请求只查了1次DB）✅\n", atomic.LoadInt64(&dbQueryCount))
	rdb.Del(ctx, "product:1001")
}

// ─────────────────────────────────────────────
// case3: 缓存雪崩
// 问题：大量 key 在同一时间过期，DB 被瞬间打垮
// 方案：TTL 基础值 + 随机抖动，让过期时间分散
// ─────────────────────────────────────────────
func case3_cacheAvalanche(ctx context.Context, rdb *redis.Client) {
	// 带随机抖动的 TTL
	jitterTTL := func(base time.Duration, maxJitter time.Duration) time.Duration {
		jitter := time.Duration(rand.Int63n(int64(maxJitter)))
		return base + jitter
	}

	keys := make([]string, 10)
	fmt.Println("设置10个缓存 key 的 TTL（基础5分钟 + 随机抖动1分钟）:")
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("product:%d", 2000+i)
		ttl := jitterTTL(5*time.Minute, 1*time.Minute)
		rdb.Set(ctx, key, fmt.Sprintf("product_data_%d", i), ttl)
		keys[i] = key
		fmt.Printf("  %s → TTL: %v\n", key, ttl.Round(time.Second))
	}

	fmt.Println("TTL 分散，不会同时过期，有效防止雪崩 ✅")

	// 清理
	for _, k := range keys {
		rdb.Del(ctx, k)
	}
}

// ─────────────────────────────────────────────
// case4: 综合防护方案
// 把三种防护融合在一个 Get 函数里
// ─────────────────────────────────────────────

type Cache struct {
	rdb *redis.Client
	sf  singleflight.Group
}

func NewCache(rdb *redis.Client) *Cache {
	return &Cache{rdb: rdb}
}

func (c *Cache) Get(ctx context.Context, key string) (string, error) {
	const nullValue = "__NULL__"

	// ① 查缓存
	val, err := c.rdb.Get(ctx, key).Result()
	if err == nil {
		if val == nullValue {
			return "", errors.New("not found")
		}
		return val, nil
	}

	// ② singleflight 防击穿
	result, err, _ := c.sf.Do(key, func() (interface{}, error) {
		// ③ 查 DB
		dbVal, dbErr := queryDB(key)
		if dbErr != nil {
			// 缓存空值防穿透，短 TTL
			c.rdb.Set(ctx, key, nullValue, 30*time.Second)
			return "", errors.New("not found")
		}
		// TTL 加随机抖动防雪崩
		ttl := 5*time.Minute + time.Duration(rand.Int63n(int64(time.Minute)))
		c.rdb.Set(ctx, key, dbVal, ttl)
		return dbVal, nil
	})

	if err != nil {
		return "", err
	}
	return result.(string), nil
}

func case4_combined(ctx context.Context, rdb *redis.Client) {
	cache := NewCache(rdb)
	rdb.Del(ctx, "product:1001", "product:9999")
	atomic.StoreInt64(&dbQueryCount, 0)

	// 正常查询
	val, _ := cache.Get(ctx, "product:1001")
	fmt.Println("查询存在的商品:", val)

	// 再次查询，命中缓存
	val, _ = cache.Get(ctx, "product:1001")
	fmt.Println("再次查询（命中缓存）:", val)

	// 查询不存在的商品（防穿透）
	_, err := cache.Get(ctx, "product:9999")
	fmt.Println("查询不存在的商品:", err)

	// 再次查询不存在的，命中空值缓存
	_, err = cache.Get(ctx, "product:9999")
	fmt.Println("再次查询不存在的（命中空值缓存）:", err)

	fmt.Printf("DB 实际查询次数: %d ✅\n", atomic.LoadInt64(&dbQueryCount))

	rdb.Del(ctx, "product:1001", "product:9999")
}

func newClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", DB: 0})
}
