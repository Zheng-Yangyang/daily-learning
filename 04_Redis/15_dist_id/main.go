// 案例15：Redis 分布式 ID 生成器
// 知识点：INCR + 位运算 + 雪花算法思想
//
// 分布式 ID 的要求：
//
//	① 全局唯一
//	② 趋势递增（有序，对数据库索引友好）
//	③ 高性能（每秒能生成大量 ID）
//	④ 高可用（Redis 挂了有降级方案）
//
// 本案例实现三种方案：
//
//	① 纯 INCR：最简单，适合单业务
//	② 号段模式：批量获取，减少 Redis 请求
//	③ Redis + 雪花：结合时间戳，ID 可解析
package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()
	rdb := newClient()
	defer rdb.Close()

	fmt.Println("=== case1: 纯 INCR 方案 ===")
	case1_incrID(ctx, rdb)

	fmt.Println("\n=== case2: 号段模式（批量获取）===")
	case2_segmentID(ctx, rdb)

	fmt.Println("\n=== case3: Redis + 雪花算法 ===")
	case3_snowflake(ctx, rdb)

	fmt.Println("\n=== case4: 并发压测（10000个ID无重复）===")
	case4_benchmark(ctx, rdb)
}

// ─────────────────────────────────────────────
// case1: 纯 INCR 方案
// 格式：业务前缀 + 日期 + 序号
// 例：ORDER-20240101-000001
// ─────────────────────────────────────────────
type IncrIDGenerator struct {
	rdb    *redis.Client
	prefix string
}

func NewIncrIDGenerator(rdb *redis.Client, prefix string) *IncrIDGenerator {
	return &IncrIDGenerator{rdb: rdb, prefix: prefix}
}

func (g *IncrIDGenerator) Next(ctx context.Context) (string, error) {
	// key 按天隔离，每天从1开始
	date := time.Now().Format("20060102")
	key := fmt.Sprintf("id:%s:%s", g.prefix, date)

	seq, err := g.rdb.Incr(ctx, key).Result()
	if err != nil {
		return "", err
	}
	// 第一次设置过期时间（次日凌晨过期）
	if seq == 1 {
		g.rdb.Expire(ctx, key, 25*time.Hour)
	}
	// 格式：前缀-日期-6位序号
	return fmt.Sprintf("%s-%s-%06d", g.prefix, date, seq), nil
}

func case1_incrID(ctx context.Context, rdb *redis.Client) {
	// 清理旧数据
	date := time.Now().Format("20060102")
	rdb.Del(ctx, "id:ORDER:"+date, "id:USER:"+date)

	orderGen := NewIncrIDGenerator(rdb, "ORDER")
	userGen := NewIncrIDGenerator(rdb, "USER")

	fmt.Println("生成订单ID:")
	for i := 0; i < 5; i++ {
		id, _ := orderGen.Next(ctx)
		fmt.Printf("  %s\n", id)
	}

	fmt.Println("生成用户ID:")
	for i := 0; i < 3; i++ {
		id, _ := userGen.Next(ctx)
		fmt.Printf("  %s\n", id)
	}

	rdb.Del(ctx, "id:ORDER:"+date, "id:USER:"+date)
}

// ─────────────────────────────────────────────
// case2: 号段模式
// 每次从 Redis 批量取一段 ID（如100个），
// 在内存里依次使用，用完再取下一段
// 优点：大幅减少 Redis 请求次数
// ─────────────────────────────────────────────
type SegmentIDGenerator struct {
	rdb     *redis.Client
	key     string
	step    int64 // 每次批量取的数量
	current int64 // 当前使用到的 ID
	maxID   int64 // 当前段的最大 ID
	mu      sync.Mutex
}

func NewSegmentIDGenerator(rdb *redis.Client, key string, step int64) *SegmentIDGenerator {
	return &SegmentIDGenerator{
		rdb:  rdb,
		key:  key,
		step: step,
	}
}

func (g *SegmentIDGenerator) Next(ctx context.Context) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 当前段用完了，重新从 Redis 取一段
	if g.current >= g.maxID {
		maxID, err := g.rdb.IncrBy(ctx, g.key, g.step).Result()
		if err != nil {
			return 0, err
		}
		g.maxID = maxID
		g.current = maxID - g.step
	}

	g.current++
	return g.current, nil
}

func case2_segmentID(ctx context.Context, rdb *redis.Client) {
	rdb.Del(ctx, "segment:order")

	// 每次批量取10个 ID
	gen := NewSegmentIDGenerator(rdb, "segment:order", 10)

	fmt.Println("号段模式生成ID（step=10）:")
	for i := 0; i < 25; i++ {
		id, _ := gen.Next(ctx)
		if i < 5 || i >= 20 {
			fmt.Printf("  第%02d个: %d\n", i+1, id)
		}
		if i == 5 {
			fmt.Println("  ... (中间省略)")
		}
	}

	// 查看 Redis 中的值（应该是 30，取了3次号段）
	val, _ := rdb.Get(ctx, "segment:order").Result()
	fmt.Printf("Redis 中的值: %s（取了3次号段，每次10个）\n", val)

	rdb.Del(ctx, "segment:order")
}

// ─────────────────────────────────────────────
// case3: Redis + 雪花算法
//
// ID 结构（64位）：
//   [符号位 1bit][时间戳 32bit][机器ID 10bit][序号 21bit]
//   时间戳：秒级，相对于自定义 epoch
//   机器ID：从 Redis INCR 获取，保证不同实例不重复
//   序号：同一秒内的递增序号
//
// 优点：
//   ① ID 包含时间信息，可反解析
//   ② 机器ID 由 Redis 统一分配，无需手动配置
// ─────────────────────────────────────────────

// 自定义 epoch：2024-01-01 00:00:00
var epoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Unix()

type SnowflakeID struct {
	MachineID int64
	Timestamp int64
	Sequence  int64
}

type RedisSnowflake struct {
	machineID int64
	sequence  int64
	lastSec   int64
	mu        sync.Mutex
}

func NewRedisSnowflake(ctx context.Context, rdb *redis.Client) (*RedisSnowflake, error) {
	// 从 Redis 获取唯一机器ID（0~1023）
	id, err := rdb.Incr(ctx, "snowflake:machine_id").Result()
	if err != nil {
		return nil, err
	}
	machineID := (id - 1) % 1024 // 限制在 10bit 范围内
	fmt.Printf("  获取到机器ID: %d\n", machineID)
	return &RedisSnowflake{machineID: machineID}, nil
}

func (s *RedisSnowflake) Next() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix() - epoch

	if now == s.lastSec {
		s.sequence++
		// 序号用 21bit，最大 2097151
		if s.sequence > 2097151 {
			// 同一秒序号用尽，等下一秒
			for time.Now().Unix()-epoch == s.lastSec {
				time.Sleep(time.Millisecond)
			}
			now = time.Now().Unix() - epoch
			s.sequence = 0
		}
	} else {
		s.sequence = 0
		s.lastSec = now
	}

	// 拼装 ID：时间戳左移31位 | 机器ID左移21位 | 序号
	id := (now << 31) | (s.machineID << 21) | s.sequence
	return id
}

// 解析 ID
func ParseSnowflakeID(id int64) SnowflakeID {
	sequence := id & 0x1FFFFF       // 低 21 位
	machineID := (id >> 21) & 0x3FF // 中 10 位
	timestamp := (id >> 31) + epoch // 高 32 位 + epoch
	return SnowflakeID{
		MachineID: machineID,
		Timestamp: timestamp,
		Sequence:  sequence,
	}
}

func case3_snowflake(ctx context.Context, rdb *redis.Client) {
	rdb.Del(ctx, "snowflake:machine_id")

	sf, err := NewRedisSnowflake(ctx, rdb)
	if err != nil {
		fmt.Println("初始化失败:", err)
		return
	}

	fmt.Println("生成雪花ID:")
	ids := make([]int64, 5)
	for i := 0; i < 5; i++ {
		id := sf.Next()
		ids[i] = id
		parsed := ParseSnowflakeID(id)
		t := time.Unix(parsed.Timestamp, 0).Format("2006-01-02 15:04:05")
		fmt.Printf("  ID:%-20d 时间:%s 机器:%d 序号:%d\n",
			id, t, parsed.MachineID, parsed.Sequence)
	}

	// 验证递增性
	isIncr := true
	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			isIncr = false
			break
		}
	}
	fmt.Printf("ID 趋势递增: %v ✅\n", isIncr)

	rdb.Del(ctx, "snowflake:machine_id")
}

// ─────────────────────────────────────────────
// case4: 并发压测
// 10000 个并发请求生成 ID，验证无重复
// ─────────────────────────────────────────────
func case4_benchmark(ctx context.Context, rdb *redis.Client) {
	rdb.Del(ctx, "snowflake:machine_id")

	sf, _ := NewRedisSnowflake(ctx, rdb)

	const total = 10000
	ids := make([]int64, total)
	var wg sync.WaitGroup
	var idx int64

	start := time.Now()
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := sf.Next()
			pos := atomic.AddInt64(&idx, 1) - 1
			ids[pos] = id
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	// 检查重复
	seen := make(map[int64]bool, total)
	duplicates := 0
	for _, id := range ids {
		if seen[id] {
			duplicates++
		}
		seen[id] = true
	}

	fmt.Printf("生成 %d 个ID 耗时: %v\n", total, elapsed)
	fmt.Printf("重复数量: %d\n", duplicates)
	fmt.Printf("无重复验证: %v ✅\n", duplicates == 0)
	fmt.Printf("QPS: %.0f/s\n", float64(total)/elapsed.Seconds())

	rdb.Del(ctx, "snowflake:machine_id")
}

func newClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", DB: 0})
}
