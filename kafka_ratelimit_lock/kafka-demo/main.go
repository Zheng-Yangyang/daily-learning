package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/segmentio/kafka-go"
)

const (
	brokerAddr = "localhost:9092"
	topic      = "payment-events"
	groupID    = "payment-processor"
)

// ============================================================
// 生产者：模拟支付系统发送订单事件
// ============================================================
func runProducer(ctx context.Context) {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokerAddr),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{}, // 发到消息量最少的分区
		RequiredAcks: kafka.RequireOne,    // 至少 1 个 broker 确认
	}
	defer writer.Close()

	fmt.Println("[Producer] 启动，开始发送支付事件...")

	orders := []string{"ORDER-001", "ORDER-002", "ORDER-003", "ORDER-004", "ORDER-005"}
	for i, orderID := range orders {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msg := kafka.Message{
			Key: []byte(orderID), // Key 决定分区，同一 Key 保证有序
			Value: []byte(fmt.Sprintf(`{"order_id":"%s","amount":%.2f,"status":"paid"}`,
				orderID, float64(100+i*50))),
			Headers: []kafka.Header{
				{Key: "event-type", Value: []byte("payment.completed")},
				{Key: "timestamp", Value: []byte(strconv.FormatInt(time.Now().Unix(), 10))},
			},
		}

		if err := writer.WriteMessages(ctx, msg); err != nil {
			fmt.Printf("[Producer] 发送失败: %v\n", err)
			continue
		}
		fmt.Printf("[Producer] ✓ 发送: key=%s value=%s\n", msg.Key, msg.Value)
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println("[Producer] 全部发送完毕")
}

// ============================================================
// 消费者：模拟支付处理服务消费事件
// ============================================================
func runConsumer(ctx context.Context, consumerID int, wg *sync.WaitGroup) {
	defer wg.Done()

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{brokerAddr},
		Topic:    topic,
		GroupID:  groupID, // 同一 GroupID 的消费者共同分担消费，不会重复
		MinBytes: 1,
		MaxBytes: 10e6,
		// 关键配置：手动提交 offset，确保消息处理成功后才"确认"
		// 如果不手动提交，消费者崩溃后会从最新位置开始，丢失消息
		CommitInterval: 0, // 0 = 手动提交
	})
	defer reader.Close()

	fmt.Printf("[Consumer-%d] 启动，等待消息...\n", consumerID)

	for {
		// FetchMessage 只拉取消息，不移动 offset
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				fmt.Printf("[Consumer-%d] 收到退出信号，停止\n", consumerID)
				return
			}
			fmt.Printf("[Consumer-%d] 读取失败: %v\n", consumerID, err)
			continue
		}

		fmt.Printf("[Consumer-%d] 收到消息: partition=%d offset=%d key=%s\n",
			consumerID, msg.Partition, msg.Offset, msg.Key)
		fmt.Printf("[Consumer-%d]   value=%s\n", consumerID, msg.Value)

		// 模拟业务处理
		if err := processMessage(msg); err != nil {
			fmt.Printf("[Consumer-%d] ✗ 处理失败，不提交 offset（会重试）: %v\n", consumerID, err)
			continue
		}

		// 处理成功后才提交 offset —— 这是"至少一次"语义的关键
		// 如果这里崩溃了，下次重启会从这条消息重新消费（需要幂等处理）
		if err := reader.CommitMessages(ctx, msg); err != nil {
			fmt.Printf("[Consumer-%d] ⚠ 提交 offset 失败: %v\n", consumerID, err)
		} else {
			fmt.Printf("[Consumer-%d] ✓ 处理完成，offset 已提交\n\n", consumerID)
		}
	}
}

func processMessage(msg kafka.Message) error {
	// 模拟业务处理（这里你可以加入幂等检查、写数据库等逻辑）
	time.Sleep(100 * time.Millisecond)
	return nil
}

// ============================================================
// 主函数：先确认 topic 存在，再同时启动生产者和消费者
// ============================================================
func main() {
	ctx, cancel := context.WithCancel(context.Background())

	// 捕获 Ctrl+C，优雅退出
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		<-c
		fmt.Println("\n收到退出信号，正在关闭...")
		cancel()
	}()

	// 确保 topic 存在（Kafka 需要预先创建 topic 或开启自动创建）
	ensureTopic(ctx)

	var wg sync.WaitGroup

	// 启动 2 个消费者（演示消费者组分担消费）
	for i := 1; i <= 2; i++ {
		wg.Add(1)
		go runConsumer(ctx, i, &wg)
	}

	// 稍等消费者就绪
	time.Sleep(1 * time.Second)

	// 启动生产者
	go runProducer(ctx)

	// 等生产者发完，再等 3 秒让消费者处理完，然后退出
	time.Sleep(10 * time.Second)
	cancel()
	wg.Wait()

	fmt.Println("\n程序退出。")
	fmt.Println("\n思考题：")
	fmt.Println("1. 如果消费者处理成功但提交 offset 失败，下次重启会发生什么？")
	fmt.Println("   （提示：会重复消费，所以业务层必须保证幂等）")
	fmt.Println("2. 消费者组里有 2 个消费者，5 个消息，消息是如何分配的？")
	fmt.Println("   （提示：取决于分区数，建议用 kafka-console-consumer 观察）")
}

func ensureTopic(ctx context.Context) {
	conn, err := kafka.DialContext(ctx, "tcp", brokerAddr)
	if err != nil {
		fmt.Printf("⚠ 连接 Kafka 失败: %v\n请确认 Kafka 已启动（见下方 docker-compose 命令）\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		fmt.Printf("⚠ 获取 Kafka controller 失败: %v\n", err)
		os.Exit(1)
	}

	controllerConn, err := kafka.Dial("tcp", fmt.Sprintf("%s:%d", controller.Host, controller.Port))
	if err != nil {
		fmt.Printf("⚠ 连接 Kafka controller 失败: %v\n", err)
		os.Exit(1)
	}
	defer controllerConn.Close()

	err = controllerConn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     2, // 2个分区，2个消费者各处理一个
		ReplicationFactor: 1,
	})
	if err != nil && err.Error() != "Topic with this name already exists" {
		fmt.Printf("⚠ 创建 topic 失败: %v\n", err)
	} else {
		fmt.Printf("Topic [%s] 就绪 ✓\n", topic)
	}
}
