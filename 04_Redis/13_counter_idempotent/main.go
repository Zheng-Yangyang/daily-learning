// 案例13：Redis 计数器与幂等性
// 知识点：INCR / Lua 原子操作 / 幂等性控制
//
// 幂等性：同一个操作执行多次，结果和执行一次一样
// 典型场景：
//
//	① 接口防重复提交
//	② 消息幂等消费（MQ 消息重试不重复处理）
//	③ 点赞/收藏（只能操作一次）
//	④ 库存扣减（防超卖）
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

	fmt.Println("=== case1: 基础计数器 ===")
	case1_basicCounter(ctx, rdb)

	fmt.Println("\n=== case2: 接口防重复提交 ===")
	case2_deduplicate(ctx, rdb)

	fmt.Println("\n=== case3: 消息幂等消费 ===")
	case3_msgIdempotent(ctx, rdb)

	fmt.Println("\n=== case4: 点赞系统（防重复点赞）===")
	case4_like(ctx, rdb)

	fmt.Println("\n=== case5: 库存扣减（防超卖）===")
	case5_stock(ctx, rdb)

	fmt.Println("\n=== case6: 并发计数器准确性验证 ===")
	case6_concurrentCounter(ctx, rdb)
}

// ─────────────────────────────────────────────
// case1: 基础计数器
// ─────────────────────────────────────────────
func case1_basicCounter(ctx context.Context, rdb *redis.Client) {
	rdb.Del(ctx, "counter:pv", "counter:uv")

	// 文章 PV（每次访问+1，允许重复）
	for i := 0; i < 5; i++ {
		rdb.Incr(ctx, "counter:pv")
	}
	pv, _ := rdb.Get(ctx, "counter:pv").Result()
	fmt.Println("PV:", pv)

	// 带过期时间的计数器（统计今日访问量）
	key := fmt.Sprintf("counter:daily:%s", time.Now().Format("2006-01-02"))
	rdb.Del(ctx, key)
	for i := 0; i < 3; i++ {
		count, _ := rdb.Incr(ctx, key).Result()
		rdb.Expire(ctx, key, 24*time.Hour)
		fmt.Printf("  今日访问量: %d\n", count)
	}

	rdb.Del(ctx, "counter:pv", key)
}

// ─────────────────────────────────────────────
// case2: 接口防重复提交
// 原理：提交时生成唯一 requestID，用 SET NX 写入 Redis
//
//	写入成功 → 第一次提交，处理
//	写入失败 → 重复提交，拒绝
//
// ─────────────────────────────────────────────
func isFirstSubmit(ctx context.Context, rdb *redis.Client, requestID string, ttl time.Duration) bool {
	key := "idempotent:request:" + requestID
	ok, _ := rdb.SetNX(ctx, key, "1", ttl).Result()
	return ok
}

func case2_deduplicate(ctx context.Context, rdb *redis.Client) {
	// 模拟同一个请求被发送3次（网络重试场景）
	requestID := "req-order-20240101-001"

	for i := 1; i <= 3; i++ {
		if isFirstSubmit(ctx, rdb, requestID, 5*time.Minute) {
			fmt.Printf("  第%d次提交 → 处理成功 ✅\n", i)
		} else {
			fmt.Printf("  第%d次提交 → 重复请求，已拒绝 ❌\n", i)
		}
	}

	rdb.Del(ctx, "idempotent:request:"+requestID)
}

// ─────────────────────────────────────────────
// case3: 消息幂等消费
// 场景：MQ 消息因网络问题重试，同一消息可能投递多次
// 方案：消费前检查 msgID 是否已处理过
// ─────────────────────────────────────────────
type Message struct {
	MsgID   string
	Content string
}

func consumeMessage(ctx context.Context, rdb *redis.Client, msg Message) {
	key := "consumed:msg:" + msg.MsgID

	// SET NX 写入消费记录，24小时内不重复消费
	ok, _ := rdb.SetNX(ctx, key, "1", 24*time.Hour).Result()
	if !ok {
		fmt.Printf("  消息 %s 已消费过，跳过 ⏭\n", msg.MsgID)
		return
	}

	// 执行业务逻辑
	fmt.Printf("  消费消息 %s: %s ✅\n", msg.MsgID, msg.Content)
}

func case3_msgIdempotent(ctx context.Context, rdb *redis.Client) {
	// 模拟同一条消息被投递3次
	msg := Message{MsgID: "msg-20240101-888", Content: "用户注册成功，发送欢迎邮件"}

	fmt.Println("模拟消息重复投递:")
	for i := 1; i <= 3; i++ {
		fmt.Printf("  第%d次投递: ", i)
		consumeMessage(ctx, rdb, msg)
	}

	rdb.Del(ctx, "consumed:msg:"+msg.MsgID)
}

// ─────────────────────────────────────────────
// case4: 点赞系统（防重复点赞）
// 用 Set 存储点赞用户，天然去重
// ─────────────────────────────────────────────
type LikeSystem struct {
	rdb *redis.Client
}

func NewLikeSystem(rdb *redis.Client) *LikeSystem {
	return &LikeSystem{rdb: rdb}
}

func likeKey(postID string) string {
	return "likes:post:" + postID
}

func (l *LikeSystem) Like(ctx context.Context, postID, userID string) (bool, int64) {
	key := likeKey(postID)
	// SADD 返回 1 表示新增成功（第一次点赞），0 表示已存在（重复点赞）
	added, _ := l.rdb.SAdd(ctx, key, userID).Result()
	count, _ := l.rdb.SCard(ctx, key).Result()
	return added == 1, count
}

func (l *LikeSystem) Unlike(ctx context.Context, postID, userID string) (bool, int64) {
	key := likeKey(postID)
	removed, _ := l.rdb.SRem(ctx, key, userID).Result()
	count, _ := l.rdb.SCard(ctx, key).Result()
	return removed == 1, count
}

func (l *LikeSystem) IsLiked(ctx context.Context, postID, userID string) bool {
	ok, _ := l.rdb.SIsMember(ctx, likeKey(postID), userID).Result()
	return ok
}

func case4_like(ctx context.Context, rdb *redis.Client) {
	ls := NewLikeSystem(rdb)
	postID := "post-001"
	rdb.Del(ctx, likeKey(postID))

	// 不同用户点赞
	users := []string{"user1", "user2", "user3"}
	for _, uid := range users {
		ok, count := ls.Like(ctx, postID, uid)
		fmt.Printf("  %s 点赞 → 成功:%v 总数:%d\n", uid, ok, count)
	}

	// 重复点赞
	ok, count := ls.Like(ctx, postID, "user1")
	fmt.Printf("  user1 重复点赞 → 成功:%v 总数:%d\n", ok, count)

	// 取消点赞
	ok, count = ls.Unlike(ctx, postID, "user2")
	fmt.Printf("  user2 取消点赞 → 成功:%v 总数:%d\n", ok, count)

	// 检查点赞状态
	fmt.Printf("  user1 是否点赞: %v\n", ls.IsLiked(ctx, postID, "user1"))
	fmt.Printf("  user2 是否点赞: %v\n", ls.IsLiked(ctx, postID, "user2"))

	rdb.Del(ctx, likeKey(postID))
}

// ─────────────────────────────────────────────
// case5: 库存扣减（防超卖）
// 用 Lua 脚本保证"检查+扣减"的原子性
// ─────────────────────────────────────────────
func deductStock(ctx context.Context, rdb *redis.Client, itemID string, count int64) (bool, int64) {
	script := `
		local key = KEYS[1]
		local deduct = tonumber(ARGV[1])
		local stock = tonumber(redis.call("GET", key))

		if stock == nil then
			return -1
		end
		if stock < deduct then
			return -2
		end

		local remaining = stock - deduct
		redis.call("SET", key, remaining)
		return remaining
	`

	result, _ := rdb.Eval(ctx, script, []string{"stock:" + itemID}, count).Int64()
	switch result {
	case -1:
		return false, 0 // 商品不存在
	case -2:
		return false, 0 // 库存不足
	default:
		return true, result
	}
}

func case5_stock(ctx context.Context, rdb *redis.Client) {
	itemID := "iphone-15"
	rdb.Set(ctx, "stock:"+itemID, 5, 0)
	fmt.Println("初始库存: 5")

	orders := []struct {
		orderID string
		count   int64
	}{
		{"order-1", 2},
		{"order-2", 2},
		{"order-3", 2}, // 库存不足
	}

	for _, o := range orders {
		ok, remaining := deductStock(ctx, rdb, itemID, o.count)
		if ok {
			fmt.Printf("  %s 扣减%d个 → 成功，剩余库存:%d ✅\n", o.orderID, o.count, remaining)
		} else {
			stock, _ := rdb.Get(ctx, "stock:"+itemID).Result()
			fmt.Printf("  %s 扣减%d个 → 库存不足，当前库存:%s ❌\n", o.orderID, o.count, stock)
		}
	}

	rdb.Del(ctx, "stock:"+itemID)
}

// ─────────────────────────────────────────────
// case6: 并发计数器准确性验证
// 100 个 goroutine 同时 INCR，最终结果必须是 100
// ─────────────────────────────────────────────
func case6_concurrentCounter(ctx context.Context, rdb *redis.Client) {
	key := "counter:concurrent"
	rdb.Del(ctx, key)

	var wg sync.WaitGroup
	var successCount int64

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := rdb.Incr(ctx, key).Result()
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	result, _ := rdb.Get(ctx, key).Result()
	fmt.Printf("100个goroutine并发INCR → 最终值:%s 成功次数:%d\n", result, successCount)
	fmt.Printf("计数器原子性验证: %v ✅\n", result == "100")

	rdb.Del(ctx, key)
}

func newClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", DB: 0})
}
