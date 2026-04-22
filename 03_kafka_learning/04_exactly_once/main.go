package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

const brokerAddr = "localhost:9092"

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: go run main.go [idempotent|transaction|eos]")
		return
	}
	switch os.Args[1] {
	case "idempotent":
		demoIdempotent()
	case "transaction":
		demoTransaction()
	case "eos":
		demoExactlyOnce()
	}
}

// ============================================================
// 演示1：幂等生产者
// ============================================================
func demoIdempotent() {
	fmt.Println("=== 幂等生产者演示 ===")

	// 非幂等（手动关闭）
	cl, _ := kgo.NewClient(
		kgo.SeedBrokers(brokerAddr),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.DisableIdempotentWrite(),
	)
	ctx := context.Background()

	fmt.Println("\n[非幂等] 连续发送2条相同内容的消息（模拟重试）:")
	for i := 0; i < 2; i++ {
		results := cl.ProduceSync(ctx, &kgo.Record{
			Topic: "hello-kafka",
			Key:   []byte("order-001"),
			Value: []byte(`{"orderId":"001","amount":100}`),
		})
		r := results[0].Record
		fmt.Printf("  写入 Partition:%d Offset:%d\n", r.Partition, r.Offset)
	}
	cl.Close()

	// 幂等（默认开启）
	cl2, _ := kgo.NewClient(
		kgo.SeedBrokers(brokerAddr),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		// 默认就是幂等，无需额外配置
	)
	defer cl2.Close()

	fmt.Println("\n[幂等开启，franz-go 默认] Producer 有唯一 ProducerID + SequenceNumber:")
	fmt.Println("  网络超时重试时，Broker 检测到相同 sequence 自动去重")
	for i := 0; i < 2; i++ {
		results := cl2.ProduceSync(ctx, &kgo.Record{
			Topic: "hello-kafka",
			Key:   []byte("order-002"),
			Value: []byte(`{"orderId":"002","amount":200}`),
		})
		r := results[0].Record
		fmt.Printf("  写入 Partition:%d Offset:%d\n", r.Partition, r.Offset)
	}
	fmt.Println("\n结论：幂等保护的是同一个 ProducerID 内的重试去重，不是业务去重")
}

// ============================================================
// 演示2：事务 - 原子写入
// ============================================================
func demoTransaction() {
	fmt.Println("=== 事务生产者演示 ===")

	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokerAddr),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.TransactionalID("order-service-tx-1"),
	)
	if err != nil {
		log.Fatal("创建事务 Producer 失败:", err)
	}
	defer cl.Close()

	ctx := context.Background()

	// --- 场景1：提交事务 ---
	fmt.Println("\n[场景1] 事务提交：原子写入2条消息")
	if err := cl.BeginTransaction(); err != nil {
		log.Fatal("开启事务失败:", err)
	}

	cl.Produce(ctx, &kgo.Record{
		Topic: "hello-kafka",
		Key:   []byte("order-tx-001"),
		Value: []byte(`{"event":"ORDER_CREATED","orderId":"tx-001","amount":500}`),
	}, nil)

	cl.Produce(ctx, &kgo.Record{
		Topic: "hello-kafka",
		Key:   []byte("notify-tx-001"),
		Value: []byte(`{"event":"SEND_EMAIL","orderId":"tx-001"}`),
	}, nil)

	if err := cl.Flush(ctx); err != nil {
		// Flush 失败则中止事务
		cl.EndTransaction(ctx, kgo.TryAbort)
		log.Fatal("Flush 失败:", err)
	}

	if err := cl.EndTransaction(ctx, kgo.TryCommit); err != nil {
		log.Fatal("提交事务失败:", err)
	}
	fmt.Println("  事务提交成功！2条消息原子写入，消费者同时可见")

	time.Sleep(500 * time.Millisecond)

	// --- 场景2：回滚事务 ---
	fmt.Println("\n[场景2] 模拟业务校验失败 → 事务回滚")
	if err := cl.BeginTransaction(); err != nil {
		log.Fatal(err)
	}

	cl.Produce(ctx, &kgo.Record{
		Topic: "hello-kafka",
		Key:   []byte("order-tx-002"),
		Value: []byte(`{"event":"ORDER_CREATED","orderId":"tx-002"}`),
	}, nil)

	// 模拟业务失败
	fmt.Println("  业务校验：库存不足！中止事务...")
	cl.AbortBufferedRecords(ctx) // 丢弃还没发出去的消息

	if err := cl.EndTransaction(ctx, kgo.TryAbort); err != nil {
		log.Fatal("回滚失败:", err)
	}
	fmt.Println("  事务已回滚！order-tx-002 对消费者完全不可见")
	fmt.Println("\n注意：需要用 FetchIsolationLevel(kgo.ReadCommitted) 的消费者才能过滤掉未提交消息")
}

// ============================================================
// 演示3：EOS - 使用 GroupTransactSession
// 消费 → 处理 → 生产，全程原子
// ============================================================
func demoExactlyOnce() {
	fmt.Println("=== Exactly Once Semantics 演示 ===")
	fmt.Println("场景：从 hello-kafka 读，处理后写回 hello-kafka")
	fmt.Println("先在另一个终端运行 produce 再跑这个，Ctrl+C 退出\n")

	sess, err := kgo.NewGroupTransactSession(
		kgo.SeedBrokers(brokerAddr),
		kgo.ConsumerGroup("eos-group-v1"),
		kgo.ConsumeTopics("hello-kafka"),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.TransactionalID("eos-processor-tx-1"),
		kgo.FetchIsolationLevel(kgo.ReadCommitted()),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer sess.Close()

	cl := sess.Client() // 拿到底层 *Client 用于 Produce

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	for {
		fetches := sess.PollFetches(ctx)
		if ctx.Err() != nil {
			fmt.Println("\n退出")
			return
		}

		records := fetches.Records()
		if len(records) == 0 {
			continue
		}

		if err := sess.Begin(); err != nil {
			log.Printf("开启事务失败: %v", err)
			continue
		}

		for _, r := range records {
			processed := fmt.Sprintf("[EOS处理] %s", r.Value)
			fmt.Printf("  处理: P:%d Offset:%d → %s\n", r.Partition, r.Offset, processed)

			cl.Produce(ctx, &kgo.Record{
				Topic: "hello-kafka",
				Key:   []byte("eos-" + string(r.Key)),
				Value: []byte(processed),
			}, nil)
		}

		if err := cl.Flush(ctx); err != nil {
			sess.End(ctx, kgo.TryAbort)
			continue
		}

		committed, err := sess.End(ctx, kgo.TryCommit)
		if err != nil {
			log.Printf("事务结束失败: %v", err)
		} else if committed {
			fmt.Printf("  事务提交成功，处理了 %d 条\n\n", len(records))
		} else {
			fmt.Println("  事务中止（Rebalance），消息将重新消费")
		}
	}
}
