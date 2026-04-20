// 案例11：Redis 延迟队列
// 知识点：ZSet 实现延迟队列
//
// 原理：
//
//	用 ZSet 存储任务，score = 执行时间戳
//	消费者轮询：ZRANGEBYSCORE 取出 score <= 当前时间的任务
//	取出后立即删除，防止重复消费
//
// 典型场景：
//
//	① 订单30分钟未支付自动取消
//	② 短信/邮件定时发送
//	③ 任务定时重试
//	④ 优惠券到期提醒
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ─────────────────────────────────────────────
// 任务结构体
// ─────────────────────────────────────────────
type Task struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Payload   string `json:"payload"`
	ExecuteAt int64  `json:"execute_at"` // Unix 时间戳（秒）
}

func (t Task) String() string {
	return fmt.Sprintf("Task{id=%s type=%s payload=%s}", t.ID, t.Type, t.Payload)
}

// ─────────────────────────────────────────────
// 延迟队列
// ─────────────────────────────────────────────
type DelayQueue struct {
	rdb *redis.Client
	key string
}

func NewDelayQueue(rdb *redis.Client, key string) *DelayQueue {
	return &DelayQueue{rdb: rdb, key: key}
}

// Push 推入任务，score = 执行时间戳
func (q *DelayQueue) Push(ctx context.Context, task Task) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	return q.rdb.ZAdd(ctx, q.key, redis.Z{
		Score:  float64(task.ExecuteAt),
		Member: string(data),
	}).Err()
}

// Pop 取出到期任务（score <= 当前时间）
// 用 Lua 脚本保证"查询+删除"原子性，防止多消费者重复消费
func (q *DelayQueue) Pop(ctx context.Context) (*Task, error) {
	script := `
		local key = KEYS[1]
		local now = ARGV[1]

		-- 取出 score <= now 的第一个任务
		local items = redis.call("ZRANGEBYSCORE", key, "-inf", now, "LIMIT", 0, 1)
		if #items == 0 then
			return nil
		end

		-- 原子删除，防止重复消费
		redis.call("ZREM", key, items[1])
		return items[1]
	`

	now := time.Now().Unix()
	result, err := q.rdb.Eval(ctx, script, []string{q.key}, now).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // 没有到期任务
		}
		return nil, err
	}

	var task Task
	if err := json.Unmarshal([]byte(result.(string)), &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// Len 队列中任务总数
func (q *DelayQueue) Len(ctx context.Context) int64 {
	n, _ := q.rdb.ZCard(ctx, q.key).Result()
	return n
}

// PendingLen 未到期任务数
func (q *DelayQueue) PendingLen(ctx context.Context) int64 {
	now := time.Now().Unix()
	n, _ := q.rdb.ZCount(ctx, q.key, fmt.Sprintf("%d", now), "+inf").Result()
	return n
}

func main() {
	ctx := context.Background()
	rdb := newClient()
	defer rdb.Close()

	fmt.Println("=== case1: 基础延迟任务 ===")
	case1_basic(ctx, rdb)

	fmt.Println("\n=== case2: 消费者轮询（模拟订单超时取消）===")
	case2_orderTimeout(ctx, rdb)

	fmt.Println("\n=== case3: 多消费者并发安全 ===")
	case3_concurrent(ctx, rdb)

	fmt.Println("\n=== case4: 查看队列状态 ===")
	case4_queueStatus(ctx, rdb)
}

// ─────────────────────────────────────────────
// case1: 基础用法
// ─────────────────────────────────────────────
func case1_basic(ctx context.Context, rdb *redis.Client) {
	q := NewDelayQueue(rdb, "dq:basic")
	rdb.Del(ctx, "dq:basic")

	now := time.Now()

	// 推入不同延迟的任务
	tasks := []Task{
		{ID: "t1", Type: "email", Payload: "发送欢迎邮件", ExecuteAt: now.Add(3 * time.Second).Unix()},
		{ID: "t2", Type: "sms", Payload: "发送验证码", ExecuteAt: now.Add(1 * time.Second).Unix()},
		{ID: "t3", Type: "push", Payload: "发送推送通知", ExecuteAt: now.Add(2 * time.Second).Unix()},
	}

	for _, t := range tasks {
		q.Push(ctx, t)
		fmt.Printf("  推入任务: %s 延迟%.0f秒后执行\n", t.ID, time.Until(time.Unix(t.ExecuteAt, 0)).Seconds())
	}

	fmt.Printf("  队列任务数: %d\n", q.Len(ctx))

	// 轮询消费
	fmt.Println("  开始消费（按到期时间顺序）:")
	deadline := time.Now().Add(5 * time.Second)
	consumed := 0
	for time.Now().Before(deadline) && consumed < 3 {
		task, _ := q.Pop(ctx)
		if task != nil {
			fmt.Printf("  [%.1fs] 消费: %s\n", time.Since(now).Seconds(), task)
			consumed++
		} else {
			time.Sleep(200 * time.Millisecond)
		}
	}

	rdb.Del(ctx, "dq:basic")
}

// ─────────────────────────────────────────────
// case2: 模拟订单超时取消
// ─────────────────────────────────────────────
func case2_orderTimeout(ctx context.Context, rdb *redis.Client) {
	q := NewDelayQueue(rdb, "dq:order")
	rdb.Del(ctx, "dq:order")

	// 模拟创建3个订单，超时时间不同
	orders := []struct {
		orderID string
		timeout time.Duration
	}{
		{"order-001", 2 * time.Second},
		{"order-002", 1 * time.Second},
		{"order-003", 3 * time.Second},
	}

	now := time.Now()
	for _, o := range orders {
		task := Task{
			ID:        o.orderID,
			Type:      "order_timeout",
			Payload:   fmt.Sprintf("订单%s超时未支付，执行取消", o.orderID),
			ExecuteAt: now.Add(o.timeout).Unix(),
		}
		q.Push(ctx, task)
		fmt.Printf("  订单 %s 创建，%v 后超时取消\n", o.orderID, o.timeout)
	}

	// 消费者：模拟订单超时处理
	fmt.Println("  等待订单超时...")
	deadline := time.Now().Add(5 * time.Second)
	consumed := 0
	for time.Now().Before(deadline) && consumed < 3 {
		task, _ := q.Pop(ctx)
		if task != nil {
			fmt.Printf("  [%.1fs] 🔔 %s\n", time.Since(now).Seconds(), task.Payload)
			consumed++
		} else {
			time.Sleep(200 * time.Millisecond)
		}
	}

	rdb.Del(ctx, "dq:order")
}

// ─────────────────────────────────────────────
// case3: 多消费者并发安全
// Lua 脚本保证同一个任务只会被一个消费者消费
// ─────────────────────────────────────────────
func case3_concurrent(ctx context.Context, rdb *redis.Client) {
	q := NewDelayQueue(rdb, "dq:concurrent")
	rdb.Del(ctx, "dq:concurrent")

	// 推入5个立即到期的任务
	for i := 1; i <= 5; i++ {
		q.Push(ctx, Task{
			ID:        fmt.Sprintf("task-%d", i),
			Type:      "job",
			Payload:   fmt.Sprintf("job-%d", i),
			ExecuteAt: time.Now().Add(-1 * time.Second).Unix(), // 已到期
		})
	}

	fmt.Println("  推入5个已到期任务，3个消费者并发抢占:")

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []string
	)

	// 3个消费者同时抢任务
	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				task, _ := q.Pop(ctx)
				if task == nil {
					break
				}
				mu.Lock()
				results = append(results, fmt.Sprintf("worker-%d 消费了 %s", workerID, task.ID))
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// 验证每个任务只被消费一次
	fmt.Printf("  消费结果（共%d条，应等于5）:\n", len(results))
	for _, r := range results {
		fmt.Println("  ", r)
	}
	fmt.Printf("  无重复消费: %v ✅\n", len(results) == 5)

	rdb.Del(ctx, "dq:concurrent")
}

// ─────────────────────────────────────────────
// case4: 查看队列状态
// ─────────────────────────────────────────────
func case4_queueStatus(ctx context.Context, rdb *redis.Client) {
	q := NewDelayQueue(rdb, "dq:status")
	rdb.Del(ctx, "dq:status")

	now := time.Now()
	// 推入一些过期和未过期的任务
	q.Push(ctx, Task{ID: "past-1", Type: "job", Payload: "已到期任务1", ExecuteAt: now.Add(-2 * time.Second).Unix()})
	q.Push(ctx, Task{ID: "past-2", Type: "job", Payload: "已到期任务2", ExecuteAt: now.Add(-1 * time.Second).Unix()})
	q.Push(ctx, Task{ID: "future-1", Type: "job", Payload: "未到期任务1", ExecuteAt: now.Add(10 * time.Second).Unix()})
	q.Push(ctx, Task{ID: "future-2", Type: "job", Payload: "未到期任务2", ExecuteAt: now.Add(20 * time.Second).Unix()})
	q.Push(ctx, Task{ID: "future-3", Type: "job", Payload: "未到期任务3", ExecuteAt: now.Add(30 * time.Second).Unix()})

	total := q.Len(ctx)
	pending := q.PendingLen(ctx)
	ready := total - pending

	fmt.Printf("  队列总任务数: %d\n", total)
	fmt.Printf("  已到期待消费: %d\n", ready)
	fmt.Printf("  未到期等待中: %d\n", pending)

	// 查看所有任务的执行时间
	items, _ := rdb.ZRangeWithScores(ctx, "dq:status", 0, -1).Result()
	fmt.Println("  任务执行时间表:")
	for _, item := range items {
		var t Task
		json.Unmarshal([]byte(item.Member.(string)), &t)
		execTime := time.Unix(int64(item.Score), 0)
		diff := time.Until(execTime).Round(time.Second)
		status := "⏰ 待执行"
		if diff < 0 {
			status = "✅ 已到期"
		}
		fmt.Printf("    %s %s (%.0f秒后)\n", status, t.ID, diff.Seconds())
	}

	rdb.Del(ctx, "dq:status")
}

func newClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", DB: 0})
}
