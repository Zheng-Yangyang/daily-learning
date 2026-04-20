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

const (
	brokerAddr = "localhost:9092"
	topic      = "hello-kafka"
	group      = "rebalance-group"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: go run main.go [produce|consume]")
		return
	}
	switch os.Args[1] {
	case "produce":
		produce()
	case "consume":
		consume()
	}
}

func produce() {
	cl, _ := kgo.NewClient(kgo.SeedBrokers(brokerAddr))
	defer cl.Close()
	ctx := context.Background()

	fmt.Println("持续生产消息，Ctrl+C 停止...")
	i := 0
	for {
		cl.ProduceSync(ctx, &kgo.Record{
			Topic: topic,
			Value: []byte(fmt.Sprintf("msg-%d  time:%s", i, time.Now().Format("15:04:05"))),
		})
		fmt.Printf("✅ 发送 msg-%d\n", i)
		i++
		time.Sleep(500 * time.Millisecond)
	}
}

func consume() {
	// 获取消费者实例 ID（用进程ID区分多个消费者）
	instanceID := fmt.Sprintf("consumer-%d", os.Getpid())

	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokerAddr),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),

		// ⚡ 关键：手动提交 offset
		kgo.DisableAutoCommit(),

		// Rebalance 回调：感知分区分配变化
		kgo.OnPartitionsAssigned(func(ctx context.Context, cl *kgo.Client, assigned map[string][]int32) {
			fmt.Printf("\n🔀 [%s] Rebalance！分配到分区: %v\n", instanceID, assigned[topic])
		}),
		kgo.OnPartitionsRevoked(func(ctx context.Context, cl *kgo.Client, revoked map[string][]int32) {
			fmt.Printf("\n🔀 [%s] Rebalance！撤销分区: %v，提交 offset...\n", instanceID, revoked[topic])
			// ⚡ Rebalance 前必须提交 offset，否则会重复消费
			if err := cl.CommitUncommittedOffsets(ctx); err != nil {
				log.Printf("提交 offset 失败: %v", err)
			}
		}),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer cl.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	fmt.Printf("🎧 [%s] 启动，Ctrl+C 退出\n", instanceID)

	msgCount := 0
	for {
		fetches := cl.PollFetches(ctx)
		if ctx.Err() != nil {
			// 退出前提交 offset
			fmt.Printf("\n[%s] 退出前提交 offset...\n", instanceID)
			cl.CommitUncommittedOffsets(context.Background())
			return
		}

		fetches.EachRecord(func(r *kgo.Record) {
			msgCount++
			fmt.Printf("📨 [%s] P:%d Offset:%d Value:%s\n",
				instanceID, r.Partition, r.Offset, r.Value)

			// 模拟处理耗时
			time.Sleep(200 * time.Millisecond)
		})

		// 每处理一批手动提交一次
		if err := cl.CommitUncommittedOffsets(ctx); err != nil && ctx.Err() == nil {
			log.Printf("提交 offset 失败: %v", err)
		}
	}
}
