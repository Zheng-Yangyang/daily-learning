package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	brokerAddr = "localhost:9092"
	topic      = "hello-kafka"
)

func main() {
	switch os.Args[1] {
	case "produce":
		produce()
	case "consume":
		consume(os.Args[2]) // 传入 group name
	case "offsets":
		showOffsets()
	default:
		fmt.Println("用法: go run main.go [produce|consume <group>|offsets]")
	}
}

// ===== 生产者 =====
func produce() {
	cl, _ := kgo.NewClient(kgo.SeedBrokers(brokerAddr))
	defer cl.Close()

	ctx := context.Background()
	for i := 0; i < 10; i++ {
		results := cl.ProduceSync(ctx, &kgo.Record{
			Topic: topic,
			Key:   []byte(fmt.Sprintf("user-%d", i%3)), // 3种key，演示同key路由到同partition
			Value: []byte(fmt.Sprintf("消息-%d [key=user-%d]", i, i%3)),
		})
		rec := results[0].Record
		fmt.Printf("✅ Partition:%d Offset:%d Key:%s Value:%s\n",
			rec.Partition, rec.Offset, rec.Key, rec.Value)
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Println("生产完毕")
}

// ===== 消费者 =====
func consume(group string) {
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokerAddr),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()), // 新 group 从最早开始
	)
	if err != nil {
		log.Fatal(err)
	}
	defer cl.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	fmt.Printf("🎧 Consumer [group=%s] 启动，Ctrl+C 退出\n", group)

	for {
		fetches := cl.PollFetches(ctx)
		if ctx.Err() != nil {
			fmt.Println("\n🛑 正常退出，offset 已自动提交")
			return
		}

		fetches.EachRecord(func(r *kgo.Record) {
			fmt.Printf("📨 [group=%s] Partition:%d Offset:%d Key:%s Value:%s\n",
				group, r.Partition, r.Offset, r.Key, r.Value)
		})
	}
}

// ===== 查看各消费组的 offset 进度 =====
func showOffsets() {
	cl, _ := kgo.NewClient(kgo.SeedBrokers(brokerAddr))
	defer cl.Close()

	adm := kadm.NewClient(cl)
	ctx := context.Background()

	// 查看 topic 各 partition 最新 offset
	topicOffsets, err := adm.ListEndOffsets(ctx, topic)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("=== Topic Partition 最新 Offset ===")
	topicOffsets.Each(func(o kadm.ListedOffset) {
		fmt.Printf("  Topic:%s Partition:%d EndOffset:%d\n", o.Topic, o.Partition, o.Offset)
	})

	// 查看消费组列表
	groups, err := adm.DescribeGroups(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("\n=== 消费组列表 ===")
	for _, g := range groups {
		fmt.Printf("  Group:%-12s State:%s Members:%d\n", g.Group, g.State, len(g.Members))
	}

	// 查看各消费组提交的 offset（逐个查询）
	fmt.Println("\n=== 消费进度 ===")
	for _, groupName := range []string{"my-first-group", "my-group-v2", "group-A", "group-B"} {
		committed, err := adm.FetchOffsets(ctx, groupName)
		if err != nil {
			fmt.Printf("  [%s] 查询失败: %v\n", groupName, err)
			continue
		}
		committed.Each(func(o kadm.OffsetResponse) {
			if o.Err == nil {
				fmt.Printf("  Group:%-14s Topic:%s Partition:%d CommittedOffset:%d\n",
					groupName, o.Topic, o.Partition, o.At)
			}
		})
	}
}
