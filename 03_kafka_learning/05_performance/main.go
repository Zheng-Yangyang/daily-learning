package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

const brokerAddr = "localhost:9092"

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: go run main.go [bench-produce|bench-consume|tuned-produce]")
		return
	}
	switch os.Args[1] {
	case "bench-produce":
		benchProduce()
	case "bench-consume":
		benchConsume()
	case "tuned-produce":
		tunedProduce()
	}
}

// ============================================================
// 基准生产者：默认配置，测吞吐量
// ============================================================
func benchProduce() {
	cl, _ := kgo.NewClient(kgo.SeedBrokers(brokerAddr))
	defer cl.Close()

	ctx := context.Background()
	var sent atomic.Int64
	start := time.Now()
	done := make(chan struct{})

	// 异步发送，回调计数
	go func() {
		for i := 0; i < 100000; i++ {
			cl.Produce(ctx, &kgo.Record{
				Topic: "hello-kafka",
				Value: []byte(fmt.Sprintf(`{"id":%d,"data":"payload-xxxxxxxxxx"}`, i)),
			}, func(r *kgo.Record, err error) {
				if err == nil {
					sent.Add(1)
				}
			})
		}
		cl.Flush(ctx)
		close(done)
	}()

	// 每秒打印进度
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var last int64
	for {
		select {
		case <-ticker.C:
			cur := sent.Load()
			fmt.Printf("吞吐量: %d msg/s | 累计: %d\n", cur-last, cur)
			last = cur
		case <-done:
			total := sent.Load()
			elapsed := time.Since(start).Seconds()
			fmt.Printf("\n默认配置完成: %d 条 | 耗时: %.2fs | 平均: %.0f msg/s\n",
				total, elapsed, float64(total)/elapsed)
			return
		}
	}
}

// ============================================================
// 调优生产者：批量 + 压缩 + Linger
// ============================================================
func tunedProduce() {
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(brokerAddr),

		// 批次大小：默认 1MB，调大可以提升吞吐
		kgo.ProducerBatchMaxBytes(2*1024*1024), // 2MB

		// Linger：等待更多消息凑批，默认已是 10ms
		// 高吞吐场景可以适当增大
		kgo.ProducerLinger(10*time.Millisecond),

		// 压缩：减少网络和存储开销
		// snappy: 速度快，压缩率中等（推荐）
		// lz4:    速度最快，压缩率略低
		// zstd:   压缩率最高，CPU 消耗稍大
		kgo.ProducerBatchCompression(kgo.SnappyCompression()),

		// 并发请求数：默认1，增大可提升吞吐
	)
	if err != nil {
		log.Fatal(err)
	}
	defer cl.Close()

	ctx := context.Background()
	var sent atomic.Int64
	start := time.Now()
	done := make(chan struct{})

	go func() {
		for i := 0; i < 100000; i++ {
			cl.Produce(ctx, &kgo.Record{
				Topic: "hello-kafka",
				Value: []byte(fmt.Sprintf(`{"id":%d,"data":"payload-xxxxxxxxxx"}`, i)),
			}, func(r *kgo.Record, err error) {
				if err == nil {
					sent.Add(1)
				}
			})
		}
		cl.Flush(ctx)
		close(done)
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var last int64
	for {
		select {
		case <-ticker.C:
			cur := sent.Load()
			fmt.Printf("吞吐量: %d msg/s | 累计: %d\n", cur-last, cur)
			last = cur
		case <-done:
			total := sent.Load()
			elapsed := time.Since(start).Seconds()
			fmt.Printf("\n调优配置完成: %d 条 | 耗时: %.2fs | 平均: %.0f msg/s\n",
				total, elapsed, float64(total)/elapsed)
			return
		}
	}
}

// ============================================================
// 基准消费者：测消费吞吐量
// ============================================================
func benchConsume() {
	cl, _ := kgo.NewClient(
		kgo.SeedBrokers(brokerAddr),
		kgo.ConsumerGroup("bench-group"),
		kgo.ConsumeTopics("hello-kafka"),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),

		// 消费调优参数
		kgo.FetchMaxBytes(50*1024*1024),          // 每次最多拉 50MB
		kgo.FetchMaxPartitionBytes(10*1024*1024), // 每个分区最多 10MB
		kgo.FetchMinBytes(1),                     // 有消息就立即返回
	)
	defer cl.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var consumed atomic.Int64
	start := time.Now()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	fmt.Println("消费中（10秒后统计）...")
	go func() {
		for {
			fetches := cl.PollFetches(ctx)
			if ctx.Err() != nil {
				return
			}
			fetches.EachRecord(func(r *kgo.Record) {
				consumed.Add(1)
			})
		}
	}()

	var last int64
	for {
		select {
		case <-ticker.C:
			cur := consumed.Load()
			fmt.Printf("消费速率: %d msg/s | 累计: %d\n", cur-last, cur)
			last = cur
		case <-ctx.Done():
			total := consumed.Load()
			elapsed := time.Since(start).Seconds()
			fmt.Printf("\n消费完成: %d 条 | 耗时: %.2fs | 平均: %.0f msg/s\n",
				total, elapsed, float64(total)/elapsed)
			return
		}
	}
}
