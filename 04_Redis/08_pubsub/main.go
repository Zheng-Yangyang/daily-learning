// 案例08：Redis 发布订阅（Pub/Sub）
// 知识点：PUBLISH / SUBSCRIBE / PSUBSCRIBE（模式订阅）
//
// Pub/Sub 是消息广播模型：
//
//	发布者把消息发到 channel，所有订阅该 channel 的消费者都能收到
//	和 List 队列的区别：
//	① List 是点对点，一条消息只能被一个消费者消费
//	② Pub/Sub 是广播，一条消息所有订阅者都能收到
//	③ Pub/Sub 不持久化，订阅者离线期间的消息会丢失
//
// 典型场景：
//
//	① 实时通知（订单状态变更通知各个服务）
//	② 聊天室
//	③ 配置变更广播（通知所有节点刷新配置）
package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()
	rdb := newClient()
	defer rdb.Close()

	fmt.Println("=== case1: 基础订阅 / 发布 ===")
	case1_basic(ctx, rdb)

	fmt.Println("\n=== case2: 多个订阅者（广播）===")
	case2_multiSubscriber(ctx, rdb)

	fmt.Println("\n=== case3: PSUBSCRIBE 模式订阅 ===")
	case3_patternSubscribe(ctx, rdb)

	fmt.Println("\n=== case4: 综合案例 —— 订单状态广播 ===")
	case4_orderNotify(ctx, rdb)
}

// ─────────────────────────────────────────────
// case1: 基础发布订阅
// ─────────────────────────────────────────────
func case1_basic(ctx context.Context, rdb *redis.Client) {
	channel := "ch:basic"

	// 订阅 channel，返回 *PubSub 对象
	sub := rdb.Subscribe(ctx, channel)
	defer sub.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	// 消费者 goroutine
	go func() {
		defer wg.Done()
		// ReceiveMessage 阻塞等待消息
		msg, err := sub.ReceiveMessage(ctx)
		if err != nil {
			fmt.Println("接收错误:", err)
			return
		}
		fmt.Printf("  [订阅者] 收到 channel=%s msg=%s\n", msg.Channel, msg.Payload)
	}()

	// 等订阅者就绪
	time.Sleep(100 * time.Millisecond)

	// 发布者发送消息，返回收到消息的订阅者数量
	receivers, _ := rdb.Publish(ctx, channel, "hello redis pubsub").Result()
	fmt.Printf("  [发布者] 发送消息，接收者数量: %d\n", receivers)

	wg.Wait()
}

// ─────────────────────────────────────────────
// case2: 多个订阅者 —— 广播效果
// ─────────────────────────────────────────────
func case2_multiSubscriber(ctx context.Context, rdb *redis.Client) {
	channel := "ch:broadcast"
	subCount := 3

	var wg sync.WaitGroup
	ready := make(chan struct{}, subCount)

	// 启动 3 个订阅者
	for i := 1; i <= subCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// 每个订阅者用独立的连接
			subRdb := newClient()
			defer subRdb.Close()

			sub := subRdb.Subscribe(ctx, channel)
			defer sub.Close()

			ready <- struct{}{}

			msg, err := sub.ReceiveMessage(ctx)
			if err != nil {
				return
			}
			fmt.Printf("  [订阅者-%d] 收到: %s\n", id, msg.Payload)
		}(i)
	}

	// 等所有订阅者就绪
	for i := 0; i < subCount; i++ {
		<-ready
	}
	time.Sleep(100 * time.Millisecond)

	// 发布一条消息，3个订阅者都能收到
	receivers, _ := rdb.Publish(ctx, channel, "广播消息：系统升级通知").Result()
	fmt.Printf("  [发布者] 发送广播，接收者数量: %d\n", receivers)

	wg.Wait()
}

// ─────────────────────────────────────────────
// case3: PSUBSCRIBE 模式订阅
// 用通配符订阅多个 channel：
//   - 匹配任意字符
//     ? 匹配单个字符
//     [abc] 匹配字符集合
//
// ─────────────────────────────────────────────
func case3_patternSubscribe(ctx context.Context, rdb *redis.Client) {
	// 订阅所有 order: 开头的 channel
	pattern := "order:*"

	subRdb := newClient()
	defer subRdb.Close()

	psub := subRdb.PSubscribe(ctx, pattern)
	defer psub.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		// 接收3条消息
		for i := 0; i < 3; i++ {
			msg, err := psub.ReceiveMessage(ctx)
			if err != nil {
				return
			}
			fmt.Printf("  [模式订阅者] pattern=%s channel=%s msg=%s\n",
				msg.Pattern, msg.Channel, msg.Payload)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// 向不同 channel 发消息，模式订阅者全都能收到
	rdb.Publish(ctx, "order:created", "订单已创建 order_id=1001")
	rdb.Publish(ctx, "order:paid", "订单已支付 order_id=1001")
	rdb.Publish(ctx, "order:shipped", "订单已发货 order_id=1001")

	wg.Wait()
}

// ─────────────────────────────────────────────
// case4: 综合案例 —— 订单状态变更广播
// 模拟：订单服务发布状态变更，库存服务和通知服务分别订阅处理
// ─────────────────────────────────────────────
func case4_orderNotify(ctx context.Context, rdb *redis.Client) {
	channel := "order:status"

	var wg sync.WaitGroup
	ready := make(chan struct{}, 2)

	// 库存服务：订阅订单创建事件，扣减库存
	wg.Add(1)
	go func() {
		defer wg.Done()
		subRdb := newClient()
		defer subRdb.Close()
		sub := subRdb.Subscribe(ctx, channel)
		defer sub.Close()

		ready <- struct{}{}
		for i := 0; i < 3; i++ {
			msg, err := sub.ReceiveMessage(ctx)
			if err != nil {
				return
			}
			fmt.Printf("  [库存服务] 收到事件: %s → 执行库存扣减\n", msg.Payload)
		}
	}()

	// 通知服务：订阅同一 channel，发送用户通知
	wg.Add(1)
	go func() {
		defer wg.Done()
		subRdb := newClient()
		defer subRdb.Close()
		sub := subRdb.Subscribe(ctx, channel)
		defer sub.Close()

		ready <- struct{}{}
		for i := 0; i < 3; i++ {
			msg, err := sub.ReceiveMessage(ctx)
			if err != nil {
				return
			}
			fmt.Printf("  [通知服务] 收到事件: %s → 发送用户通知\n", msg.Payload)
		}
	}()

	// 等两个服务都就绪
	<-ready
	<-ready
	time.Sleep(100 * time.Millisecond)

	// 订单服务发布状态变更
	events := []string{"ORDER_CREATED", "ORDER_PAID", "ORDER_SHIPPED"}
	for _, event := range events {
		receivers, _ := rdb.Publish(ctx, channel, event).Result()
		fmt.Printf("  [订单服务] 发布: %-20s 接收者: %d\n", event, receivers)
		time.Sleep(100 * time.Millisecond)
	}

	wg.Wait()
}

func newClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", DB: 0})
}
