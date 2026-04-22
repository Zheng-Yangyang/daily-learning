// 案例06：Redis 分布式锁
// 知识点：SET NX EX / Lua 原子解锁 / 锁续期 / 可重入锁
//
// 分布式锁要解决的核心问题：
//
//	① 互斥性：同一时刻只有一个客户端持有锁
//	② 防死锁：持有锁的进程崩溃后锁能自动释放（靠 EX）
//	③ 防误删：只能删除自己加的锁（靠唯一 value）
//	④ 原子性：加锁和设过期时间必须是原子操作（靠 SET NX EX）
package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// ─────────────────────────────────────────────
// 分布式锁实现
// ─────────────────────────────────────────────
type DistLock struct {
	rdb   *redis.Client
	key   string
	value string // 唯一标识，防止误删别人的锁
	ttl   time.Duration
}

func NewDistLock(rdb *redis.Client, key string, ttl time.Duration) *DistLock {
	return &DistLock{
		rdb:   rdb,
		key:   key,
		value: uuid.New().String(), // 每个锁实例有唯一 value
		ttl:   ttl,
	}
}

// Lock 加锁：SET key value NX EX ttl
// 返回 true 表示加锁成功
func (l *DistLock) Lock(ctx context.Context) (bool, error) {
	return l.rdb.SetNX(ctx, l.key, l.value, l.ttl).Result()
}

// Unlock 解锁：用 Lua 脚本保证"判断+删除"的原子性
// 先判断 value 是否是自己的，是才删除，防止误删其他客户端的锁
func (l *DistLock) Unlock(ctx context.Context) error {
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`
	result, err := l.rdb.Eval(ctx, script, []string{l.key}, l.value).Result()
	if err != nil {
		return err
	}
	if result.(int64) == 0 {
		return errors.New("解锁失败：锁不存在或已被其他客户端持有")
	}
	return nil
}

// TryLockWithRetry 带重试的加锁
func (l *DistLock) TryLockWithRetry(ctx context.Context, maxRetry int, interval time.Duration) (bool, error) {
	for i := 0; i < maxRetry; i++ {
		ok, err := l.Lock(ctx)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
		fmt.Printf("  [重试] 第%d次尝试加锁失败，等待%v后重试...\n", i+1, interval)
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(interval):
		}
	}
	return false, nil
}

func main() {
	ctx := context.Background()
	rdb := newClient()
	defer rdb.Close()

	fmt.Println("=== case1: 基础加锁 / 解锁 ===")
	case1_basicLock(ctx, rdb)

	fmt.Println("\n=== case2: 防误删（value 唯一性验证）===")
	case2_preventMistakeDelete(ctx, rdb)

	fmt.Println("\n=== case3: 并发抢锁（10个goroutine抢1把锁）===")
	case3_concurrent(ctx, rdb)

	fmt.Println("\n=== case4: 带重试的加锁 ===")
	case4_retryLock(ctx, rdb)

	fmt.Println("\n=== case5: 锁超时自动释放（防死锁）===")
	case5_autoExpire(ctx, rdb)
}

func case1_basicLock(ctx context.Context, rdb *redis.Client) {
	lock := NewDistLock(rdb, "lock:order:1001", 10*time.Second)

	// 加锁
	ok, err := lock.Lock(ctx)
	if err != nil {
		fmt.Println("加锁出错:", err)
		return
	}
	fmt.Println("加锁结果:", ok) // true

	// 查看锁的 value
	val, _ := rdb.Get(ctx, "lock:order:1001").Result()
	fmt.Println("锁的 value:", val)

	// 模拟业务处理
	fmt.Println("执行业务逻辑...")
	time.Sleep(100 * time.Millisecond)

	// 解锁
	err = lock.Unlock(ctx)
	if err != nil {
		fmt.Println("解锁失败:", err)
		return
	}
	fmt.Println("解锁成功 ✅")

	// 确认锁已释放
	_, err = rdb.Get(ctx, "lock:order:1001").Result()
	if errors.Is(err, redis.Nil) {
		fmt.Println("锁已释放，key 不存在 ✅")
	}
}

func case2_preventMistakeDelete(ctx context.Context, rdb *redis.Client) {
	// 模拟场景：
	// worker1 加锁后业务超时，锁自动过期
	// worker2 拿到锁正在执行
	// worker1 业务结束想解锁，但不能删掉 worker2 的锁

	lockKey := "lock:order:2001"

	worker1 := NewDistLock(rdb, lockKey, 1*time.Second)
	worker2 := NewDistLock(rdb, lockKey, 10*time.Second)

	// worker1 加锁
	ok, _ := worker1.Lock(ctx)
	fmt.Println("worker1 加锁:", ok)

	// 模拟 worker1 的锁过期
	fmt.Println("模拟 worker1 锁过期...")
	time.Sleep(1100 * time.Millisecond)

	// worker2 此时拿到锁
	ok, _ = worker2.Lock(ctx)
	fmt.Println("worker2 加锁:", ok)

	// worker1 尝试解锁（此时锁属于 worker2）
	err := worker1.Unlock(ctx)
	if err != nil {
		fmt.Println("worker1 解锁失败（符合预期）:", err, "✅")
	}

	// worker2 正常解锁
	err = worker2.Unlock(ctx)
	if err == nil {
		fmt.Println("worker2 解锁成功 ✅")
	}
}

func case3_concurrent(ctx context.Context, rdb *redis.Client) {
	lockKey := "lock:flash_sale"
	rdb.Del(ctx, lockKey)

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		success []int
		failed  []int
	)

	// 10 个 goroutine 同时抢锁
	for i := 1; i <= 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			lock := NewDistLock(rdb, lockKey, 5*time.Second)
			ok, _ := lock.Lock(ctx)
			mu.Lock()
			if ok {
				success = append(success, id)
				// 模拟持锁业务
				time.Sleep(10 * time.Millisecond)
				lock.Unlock(ctx)
			} else {
				failed = append(failed, id)
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()
	fmt.Printf("抢锁成功: %v\n", success)
	fmt.Printf("抢锁失败: %v\n", failed)
	fmt.Printf("同时只有1个goroutine持锁: %v ✅\n", len(success) == 1)
}

func case4_retryLock(ctx context.Context, rdb *redis.Client) {
	lockKey := "lock:inventory"

	// 先用一把锁占住，模拟有人持有锁
	holder := NewDistLock(rdb, lockKey, 2*time.Second)
	holder.Lock(ctx)
	fmt.Println("holder 已持有锁（2秒后自动过期）")

	// 另一个客户端带重试加锁
	waiter := NewDistLock(rdb, lockKey, 5*time.Second)
	ok, err := waiter.TryLockWithRetry(ctx, 5, 600*time.Millisecond)
	if err != nil {
		fmt.Println("加锁出错:", err)
		return
	}
	if ok {
		fmt.Println("waiter 重试加锁成功 ✅")
		waiter.Unlock(ctx)
	} else {
		fmt.Println("waiter 重试加锁失败")
	}
}

func case5_autoExpire(ctx context.Context, rdb *redis.Client) {
	// 防死锁：持有锁的进程崩溃，锁会在 TTL 后自动释放
	lockKey := "lock:payment"
	lock := NewDistLock(rdb, lockKey, 2*time.Second)

	ok, _ := lock.Lock(ctx)
	fmt.Println("加锁:", ok)

	ttl, _ := rdb.TTL(ctx, lockKey).Result()
	fmt.Println("锁的 TTL:", ttl)

	// 模拟进程崩溃（不调用 Unlock，让锁自动过期）
	fmt.Print("模拟进程崩溃，等待锁自动过期...")
	time.Sleep(2500 * time.Millisecond)

	_, err := rdb.Get(ctx, lockKey).Result()
	if errors.Is(err, redis.Nil) {
		fmt.Println(" 锁已自动释放，防死锁生效 ✅")
	}

	// 新进程可以重新加锁
	newLock := NewDistLock(rdb, lockKey, 10*time.Second)
	ok, _ = newLock.Lock(ctx)
	fmt.Println("新进程重新加锁:", ok)
	newLock.Unlock(ctx)
}

func newClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", DB: 0})
}
