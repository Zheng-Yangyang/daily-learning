package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	brokerAddr = "localhost:9092"
	topic      = "hello-kafka"
)

func main() {
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	// 启动消费者
	wg.Add(1)
	go func() {
		defer wg.Done()
		startConsumer(ctx)
	}()

	time.Sleep(1 * time.Second) // 等消费者就绪

	// 生产消息
	startProducer()

	// 等一会儿让 Consumer 消费完
	time.Sleep(3 * time.Second)
	cancel() // 通知 Consumer 退出

	wg.Wait()
	fmt.Println("✅ 程序正常退出")
}

func startProducer() {
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokerAddr),
	)
	if err != nil {
		log.Fatal("创建 Producer 失败:", err)
	}
	defer cl.Close()

	ctx := context.Background()

	for i := 0; i < 5; i++ {
		msg := fmt.Sprintf("Hello Kafka! 消息编号: %d", i)

		results := cl.ProduceSync(ctx, &kgo.Record{
			Topic: topic,
			Key:   []byte(fmt.Sprintf("key-%d", i)),
			Value: []byte(msg),
		})

		if err := results.FirstErr(); err != nil {
			log.Printf("发送失败: %v", err)
		} else {
			rec := results[0].Record
			fmt.Printf("✅ 发送成功 | Partition: %d | Offset: %d | Value: %s\n",
				rec.Partition, rec.Offset, rec.Value)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func startConsumer(ctx context.Context) {
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokerAddr),
		kgo.ConsumerGroup("my-first-group"),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		log.Fatal("创建 Consumer 失败:", err)
	}
	defer cl.Close()

	fmt.Println("🎧 Consumer 启动，等待消息...")

	for {
		fetches := cl.PollFetches(ctx)

		// ctx 被取消时正常退出
		if ctx.Err() != nil {
			fmt.Println("🛑 Consumer 收到退出信号")
			return
		}

		if errs := fetches.Errors(); len(errs) > 0 {
			log.Printf("拉取错误: %v", errs)
			continue
		}

		fetches.EachRecord(func(record *kgo.Record) {
			fmt.Printf("📨 收到消息 | Partition: %d | Offset: %d | Key: %s | Value: %s\n",
				record.Partition, record.Offset, record.Key, record.Value)
		})
	}
}
