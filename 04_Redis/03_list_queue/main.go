// 案例03：Redis List —— 消息队列 & 最近记录
// 知识点：LPUSH / RPUSH / LPOP / RPOP / BLPOP / LRANGE / LLEN / LINDEX / LREM / LTRIM
//
// List 是双端链表，两头增删都是 O(1)
// 典型场景：
//
//	① 先进先出队列（LPUSH + RPOP）—— 任务队列
//	② 先进后出栈  （LPUSH + LPOP） —— 撤销操作
//	③ 最近 N 条记录（LPUSH + LTRIM）—— 浏览历史
//	④ 阻塞消费（BLPOP）—— 轻量级消息队列
package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()
	rdb := newClient()
	defer rdb.Close()

	fmt.Println("=== case1: LPUSH / RPUSH / LRANGE / LLEN ===")
	case1_basic(ctx, rdb)

	fmt.Println("\n=== case2: 先进先出队列（任务队列）===")
	case2_fifoQueue(ctx, rdb)

	fmt.Println("\n=== case3: 栈 LIFO ===")
	case3_stack(ctx, rdb)

	fmt.Println("\n=== case4: 最近 N 条浏览记录（LTRIM）===")
	case4_recentN(ctx, rdb)

	fmt.Println("\n=== case5: BLPOP 阻塞消费 ===")
	case5_blpop(ctx, rdb)

	fmt.Println("\n=== case6: LINDEX / LREM ===")
	case6_lindexLrem(ctx, rdb)
}

func case1_basic(ctx context.Context, rdb *redis.Client) {
	key := "demo:list"
	rdb.Del(ctx, key)

	// LPUSH 从左侧插入，多个元素是逆序进入的
	// LPUSH key c b a  →  链表从左到右: a b c
	rdb.LPush(ctx, key, "c", "b", "a")

	// RPUSH 从右侧插入
	rdb.RPush(ctx, key, "d", "e")

	// LRANGE key start stop（-1 表示最后一个）
	vals, _ := rdb.LRange(ctx, key, 0, -1).Result()
	fmt.Println("LRANGE 0 -1 →", vals) // [a b c d e]

	length, _ := rdb.LLen(ctx, key).Result()
	fmt.Println("LLEN →", length)

	vals, _ = rdb.LRange(ctx, key, 0, 2).Result()
	fmt.Println("LRANGE 0 2 →", vals) // 前3个

	rdb.Del(ctx, key)
}

func case2_fifoQueue(ctx context.Context, rdb *redis.Client) {
	queue := "queue:task"
	rdb.Del(ctx, queue)

	// 生产者：LPUSH 从左插入
	for i := 1; i <= 5; i++ {
		task := fmt.Sprintf("task%d", i)
		rdb.LPush(ctx, queue, task)
		fmt.Println("  生产:", task)
	}

	fmt.Println("  队列长度:", mustLLen(ctx, rdb, queue))

	// 消费者：RPOP 从右弹出 → 先进先出
	fmt.Println("  开始消费:")
	for {
		val, err := rdb.RPop(ctx, queue).Result()
		if errors.Is(err, redis.Nil) {
			break
		}
		fmt.Println("  消费:", val)
	}
}

func case3_stack(ctx context.Context, rdb *redis.Client) {
	stack := "demo:stack"
	rdb.Del(ctx, stack)

	// LPUSH 压栈
	for _, item := range []string{"page1", "page2", "page3"} {
		rdb.LPush(ctx, stack, item)
		fmt.Println("  PUSH:", item)
	}

	// LPOP 弹栈 → 后进先出
	fmt.Println("  开始弹栈:")
	for {
		val, err := rdb.LPop(ctx, stack).Result()
		if errors.Is(err, redis.Nil) {
			break
		}
		fmt.Println("  POP:", val) // page3, page2, page1
	}
}

func case4_recentN(ctx context.Context, rdb *redis.Client) {
	const maxHistory = 5
	key := "user:1001:history"
	rdb.Del(ctx, key)

	pages := []string{"/home", "/product/1", "/product/2", "/cart", "/checkout", "/order/success", "/profile"}

	for _, page := range pages {
		// ① LPUSH 插入最新记录到头部
		rdb.LPush(ctx, key, page)
		// ② LTRIM 只保留最近 maxHistory 条，多余的自动丢弃
		rdb.LTrim(ctx, key, 0, int64(maxHistory-1))

		history, _ := rdb.LRange(ctx, key, 0, -1).Result()
		fmt.Printf("  访问 %-20s → 历史: %v\n", page, history)
	}

	rdb.Del(ctx, key)
}

func case5_blpop(ctx context.Context, rdb *redis.Client) {
	// BLPOP 是 LPOP 的阻塞版本：
	//   队列非空 → 立即返回
	//   队列为空 → 阻塞等待，直到有数据或超时
	// 比轮询 RPOP 更高效，不浪费 CPU
	queueKey := "queue:blpop_demo"
	rdb.Del(ctx, queueKey)

	var wg sync.WaitGroup

	// 消费者先启动，队列为空，阻塞等待
	wg.Add(1)
	go func() {
		defer wg.Done()
		fmt.Println("  [消费者] 启动，等待消息...")
		for i := 0; i < 3; i++ {
			// result[0] = key 名，result[1] = 值
			result, err := rdb.BLPop(ctx, 5*time.Second, queueKey).Result()
			if err != nil {
				fmt.Println("  [消费者] 超时或错误:", err)
				return
			}
			fmt.Printf("  [消费者] 收到: %s\n", result[1])
		}
	}()

	// 生产者延迟 500ms 后发消息
	time.Sleep(500 * time.Millisecond)
	for _, msg := range []string{"order_created", "payment_done", "shipped"} {
		rdb.LPush(ctx, queueKey, msg)
		fmt.Println("  [生产者] 发送:", msg)
		time.Sleep(300 * time.Millisecond)
	}

	wg.Wait()
	rdb.Del(ctx, queueKey)
}

func case6_lindexLrem(ctx context.Context, rdb *redis.Client) {
	key := "demo:lrem"
	rdb.Del(ctx, key)

	rdb.RPush(ctx, key, "a", "b", "a", "c", "a", "d")
	vals, _ := rdb.LRange(ctx, key, 0, -1).Result()
	fmt.Println("初始列表 →", vals)

	// LINDEX 按索引读（不弹出），O(n)
	v0, _ := rdb.LIndex(ctx, key, 0).Result()
	vLast, _ := rdb.LIndex(ctx, key, -1).Result()
	fmt.Printf("LINDEX: [0]=%s  [-1]=%s\n", v0, vLast)

	// LREM key count value
	//   count > 0：从头到尾删最多 count 个
	//   count < 0：从尾到头删最多 |count| 个
	//   count = 0：删除全部
	removed, _ := rdb.LRem(ctx, key, 2, "a").Result()
	fmt.Println("LREM 删除 2 个 'a'，实际删除:", removed)

	vals, _ = rdb.LRange(ctx, key, 0, -1).Result()
	fmt.Println("删除后 →", vals)

	rdb.Del(ctx, key)
}

func mustLLen(ctx context.Context, rdb *redis.Client, key string) int64 {
	n, _ := rdb.LLen(ctx, key).Result()
	return n
}

func newClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", DB: 0})
}

func checkErr(err error) {
	if err != nil && !errors.Is(err, redis.Nil) {
		panic(err)
	}
}
