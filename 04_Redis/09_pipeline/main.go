// 案例09：Redis Pipeline —— 批量操作性能优化
// 知识点：Pipeline / TxPipeline（事务Pipeline）/ 性能对比
//
// 正常模式：每个命令都要经历 发送→等待→接收 一次网络RTT
// Pipeline：把多个命令打包一次性发送，只需要一次RTT
//
// 类比：
//
//	普通模式 = 每买一件东西跑一趟超市
//	Pipeline = 列好购物清单，跑一趟买完所有东西
//
// 注意：Pipeline 不是原子的，中间某个命令失败不影响其他命令
//
//	TxPipeline 是原子的（MULTI/EXEC包裹），要么全成功要么全失败
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

	fmt.Println("=== case1: 普通模式 vs Pipeline 性能对比 ===")
	case1_benchmark(ctx, rdb)

	fmt.Println("\n=== case2: Pipeline 基础用法 ===")
	case2_basic(ctx, rdb)

	fmt.Println("\n=== case3: TxPipeline 事务用法 ===")
	case3_txPipeline(ctx, rdb)

	fmt.Println("\n=== case4: Pipeline 批量写入用户数据 ===")
	case4_batchWrite(ctx, rdb)

	fmt.Println("\n=== case5: Pipeline 批量读取 ===")
	case5_batchRead(ctx, rdb)
}

// ─────────────────────────────────────────────
// case1: 性能对比
// ─────────────────────────────────────────────
func case1_benchmark(ctx context.Context, rdb *redis.Client) {
	const count = 100

	// 清理
	for i := 0; i < count; i++ {
		rdb.Del(ctx, fmt.Sprintf("bench:normal:%d", i))
		rdb.Del(ctx, fmt.Sprintf("bench:pipeline:%d", i))
	}

	// 普通模式：100 次独立命令，每次都要等待网络 RTT
	start := time.Now()
	for i := 0; i < count; i++ {
		rdb.Set(ctx, fmt.Sprintf("bench:normal:%d", i), i, time.Minute)
	}
	normalDuration := time.Since(start)
	fmt.Printf("普通模式  100次SET: %v\n", normalDuration)

	// Pipeline 模式：100 次命令打包一次发送
	start = time.Now()
	pipe := rdb.Pipeline()
	for i := 0; i < count; i++ {
		pipe.Set(ctx, fmt.Sprintf("bench:pipeline:%d", i), i, time.Minute)
	}
	pipe.Exec(ctx) // 一次性发送所有命令
	pipelineDuration := time.Since(start)
	fmt.Printf("Pipeline  100次SET: %v\n", pipelineDuration)
	fmt.Printf("性能提升: %.1fx ✅\n", float64(normalDuration)/float64(pipelineDuration))

	// 清理
	for i := 0; i < count; i++ {
		rdb.Del(ctx, fmt.Sprintf("bench:normal:%d", i))
		rdb.Del(ctx, fmt.Sprintf("bench:pipeline:%d", i))
	}
}

// ─────────────────────────────────────────────
// case2: Pipeline 基础用法
// ─────────────────────────────────────────────
func case2_basic(ctx context.Context, rdb *redis.Client) {
	rdb.Del(ctx, "pl:name", "pl:age", "pl:city")

	// 方式一：手动管理 Pipeline
	pipe := rdb.Pipeline()

	// 把命令加入 pipeline，此时还没有发送到 Redis
	setName := pipe.Set(ctx, "pl:name", "Alice", time.Minute)
	setAge := pipe.Set(ctx, "pl:age", "28", time.Minute)
	setCity := pipe.Set(ctx, "pl:city", "Beijing", time.Minute)

	// Exec 才真正发送，返回每个命令的结果
	_, err := pipe.Exec(ctx)
	if err != nil {
		fmt.Println("Pipeline 执行失败:", err)
		return
	}

	fmt.Println("SET name:", setName.Err())
	fmt.Println("SET age: ", setAge.Err())
	fmt.Println("SET city:", setCity.Err())

	// 方式二：用 Pipelined 闭包，更简洁
	var getName, getAge, getCity *redis.StringCmd
	rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		getName = pipe.Get(ctx, "pl:name")
		getAge = pipe.Get(ctx, "pl:age")
		getCity = pipe.Get(ctx, "pl:city")
		return nil
	})

	fmt.Printf("GET name=%s age=%s city=%s\n",
		getName.Val(), getAge.Val(), getCity.Val())

	rdb.Del(ctx, "pl:name", "pl:age", "pl:city")
}

// ─────────────────────────────────────────────
// case3: TxPipeline —— 事务，原子执行
// MULTI/EXEC 保证中间不会插入其他客户端的命令
// 注意：Redis 事务不支持回滚，EXEC 时某条命令报错，其他命令仍会执行
// ─────────────────────────────────────────────
func case3_txPipeline(ctx context.Context, rdb *redis.Client) {
	rdb.Del(ctx, "tx:stock", "tx:sold")
	rdb.Set(ctx, "tx:stock", 100, 0)
	rdb.Set(ctx, "tx:sold", 0, 0)

	// TxPipelined：在 MULTI/EXEC 中原子执行
	_, err := rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.DecrBy(ctx, "tx:stock", 5) // 扣减库存
		pipe.IncrBy(ctx, "tx:sold", 5)  // 增加销量
		return nil
	})

	if err != nil {
		fmt.Println("事务执行失败:", err)
		return
	}

	stock, _ := rdb.Get(ctx, "tx:stock").Result()
	sold, _ := rdb.Get(ctx, "tx:sold").Result()
	fmt.Printf("库存: %s，销量: %s（原子扣减成功）✅\n", stock, sold)

	rdb.Del(ctx, "tx:stock", "tx:sold")
}

// ─────────────────────────────────────────────
// case4: 批量写入用户数据
// ─────────────────────────────────────────────
func case4_batchWrite(ctx context.Context, rdb *redis.Client) {
	type User struct {
		ID    int
		Name  string
		Score int
	}

	users := []User{
		{1, "Alice", 100},
		{2, "Bob", 200},
		{3, "Charlie", 300},
		{4, "Dave", 400},
		{5, "Eve", 500},
	}

	// 用 Pipeline 批量写入：每个用户写 Hash + ZSet 排行榜
	start := time.Now()
	_, err := rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, u := range users {
			key := fmt.Sprintf("user:%d", u.ID)
			pipe.HSet(ctx, key, "name", u.Name, "score", u.Score)
			pipe.Expire(ctx, key, time.Hour)
			pipe.ZAdd(ctx, "rank:score", redis.Z{
				Score:  float64(u.Score),
				Member: u.Name,
			})
		}
		return nil
	})

	if err != nil {
		fmt.Println("批量写入失败:", err)
		return
	}

	fmt.Printf("批量写入 %d 个用户耗时: %v\n", len(users), time.Since(start))

	// 验证排行榜
	top, _ := rdb.ZRevRangeWithScores(ctx, "rank:score", 0, -1).Result()
	fmt.Println("排行榜:")
	for i, m := range top {
		fmt.Printf("  第%d名: %-10s %.0f分\n", i+1, m.Member, m.Score)
	}

	// 清理
	for _, u := range users {
		rdb.Del(ctx, fmt.Sprintf("user:%d", u.ID))
	}
	rdb.Del(ctx, "rank:score")
}

// ─────────────────────────────────────────────
// case5: Pipeline 批量读取
// ─────────────────────────────────────────────
func case5_batchRead(ctx context.Context, rdb *redis.Client) {
	// 先写入数据
	rdb.MSet(ctx,
		"item:1", "apple",
		"item:2", "banana",
		"item:3", "cherry",
		"item:4", "durian",
		"item:5", "elderberry",
	)

	// Pipeline 批量读取
	keys := []string{"item:1", "item:2", "item:3", "item:4", "item:5", "item:99"}
	cmds := make([]*redis.StringCmd, len(keys))

	_, err := rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for i, key := range keys {
			cmds[i] = pipe.Get(ctx, key)
		}
		return nil
	})

	// Pipeline Exec 返回的 err 是最后一个错误
	// 需要逐个检查每个命令的结果
	_ = err
	fmt.Println("批量读取结果:")
	for i, cmd := range cmds {
		val, err := cmd.Result()
		if err != nil {
			fmt.Printf("  %s → 不存在\n", keys[i])
		} else {
			fmt.Printf("  %s → %s\n", keys[i], val)
		}
	}

	rdb.Del(ctx, "item:1", "item:2", "item:3", "item:4", "item:5")
}

func newClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", DB: 0})
}
