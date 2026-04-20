package main

import (
	"context"
	"fmt"
	"time"

	"math/rand"

	"github.com/redis/go-redis/v9"
)

type DistributedLock struct {
	client *redis.Client
	key    string
	value  string
	ttl    time.Duration
}

func NewDistributedLock(client *redis.Client, key string, ttl time.Duration) *DistributedLock {
	return &DistributedLock{
		client: client,
		key:    key,
		value:  fmt.Sprintf("%d-%d", time.Now().UnixNano(), rand.Int63()),
		ttl:    ttl,
	}
}

func (l *DistributedLock) TryLock(ctx context.Context) (bool, error) {
	result, err := l.client.SetArgs(ctx, l.key, l.value, redis.SetArgs{
		TTL:  l.ttl,
		Mode: "NX",
	}).Result()

	return result == "OK", err
}

func (l *DistributedLock) Unlock(ctx context.Context) error {
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`
	result, err := l.client.Eval(ctx, script, []string{l.key}, l.value).Result()
	if err != nil {
		return err
	}
	if result.(int64) == 0 {
		return fmt.Errorf("锁已失效或不属于当前持有者（可能发生了锁过期问题）")
	}
	return nil
}

func processPayment(ctx context.Context, client *redis.Client, orderID string, workerID int) {
	lockKey := "pay:lock:" + orderID
	lock := NewDistributedLock(client, lockKey, 5*time.Second)

	ok, err := lock.TryLock(ctx)
	if err != nil {
		fmt.Printf("  Worker %2d: 加锁出错 - %v\n", workerID, err)
		return
	}
	if !ok {
		fmt.Printf("  Worker %2d: ✗ 未抢到锁，跳过（防重复处理）\n", workerID)
		return
	}

	// 拿到锁，开始处理
	fmt.Printf("  Worker %2d: ✓ 抢到锁，开始处理订单 %s\n", workerID, orderID)
	time.Sleep(200 * time.Millisecond) // 模拟业务处理耗时

	// 处理完毕，释放锁
	if err := lock.Unlock(ctx); err != nil {
		fmt.Printf("  Worker %2d: ⚠ 解锁失败 - %v\n", workerID, err)
	} else {
		fmt.Printf("  Worker %2d: 锁已释放\n", workerID)
	}
}

func main() {
	ctx := context.Background()

	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	// 检查连接
	if err := client.Ping(ctx).Err(); err != nil {
		fmt.Printf("Redis 连接失败: %v\n请确认 redis-server 已启动\n", err)
		return
	}
	fmt.Println("Redis 连接成功 ✓")

	// 清理旧数据
	client.Del(ctx, "pay:lock:ORDER-88888")

	fmt.Println("\n========================================")
	fmt.Println(" 场景一：10个并发回调争抢同一笔订单的锁")
	fmt.Println(" 只允许1个处理，其余直接跳过（幂等保障）")
	fmt.Println("========================================")

	// 10 个 goroutine 同时争抢同一笔订单的锁
	done := make(chan struct{}, 10)
	for i := 1; i <= 10; i++ {
		go func(id int) {
			processPayment(ctx, client, "ORDER-88888", id)
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	// ---- 演示锁过期问题 ----
	fmt.Println("\n========================================")
	fmt.Println(" 场景二：演示锁过期后的安全性")
	fmt.Println(" 锁TTL=1s，业务耗时2s，锁会在中途过期")
	fmt.Println("========================================")

	shortLock := NewDistributedLock(client, "pay:lock:expire-demo", 1*time.Second)
	ok, _ := shortLock.TryLock(ctx)
	if ok {
		fmt.Println("  加锁成功，TTL=1s")
		fmt.Println("  模拟业务处理耗时 2s（超过锁TTL）...")
		time.Sleep(2 * time.Second)

		// 此时锁已经自动过期，可能被别人拿走了
		err := shortLock.Unlock(ctx)
		if err != nil {
			fmt.Printf("  ⚠ 解锁结果: %v\n", err)
			fmt.Println("  这就是锁过期的危险：业务还没跑完，锁已经被别人持有")
			fmt.Println("  解决方案：1) 合理评估TTL  2) 看门狗自动续期（Redisson方案）")
		} else {
			fmt.Println("  锁释放成功（运气好，没人抢）")
		}
	}

	fmt.Println("\n========================================")
	fmt.Println(" 场景三：演示 value 校验防误删")
	fmt.Println("========================================")

	client.Del(ctx, "pay:lock:verify-demo")
	lockA := NewDistributedLock(client, "pay:lock:verify-demo", 10*time.Second)
	lockB := NewDistributedLock(client, "pay:lock:verify-demo", 10*time.Second)

	lockA.TryLock(ctx)
	fmt.Printf("  lockA 加锁成功，value=%s\n", lockA.value[:16]+"...")

	// lockB 尝试删除 lockA 的锁（不同value，会被 Lua 脚本拒绝）
	err := lockB.Unlock(ctx)
	if err != nil {
		fmt.Printf("  lockB 尝试删除 lockA 的锁: 失败 ✓（Lua 脚本保护生效）\n")
	}

	// lockA 自己删除，成功
	err = lockA.Unlock(ctx)
	if err == nil {
		fmt.Println("  lockA 自己解锁: 成功 ✓")
	}

	fmt.Println("\n全部演示完毕。")
	fmt.Println("\n思考题：如果 Redis 主节点宕机，从节点还没同步锁数据，会发生什么？")
	fmt.Println("（提示：这就是 RedLock 算法要解决的问题）")
}
