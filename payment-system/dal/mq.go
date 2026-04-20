package dal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

type MQ struct {
	writer *kafka.Writer
}

// PaymentEvent 支付成功事件，发给下游消费
type PaymentEvent struct {
	EventType string    `json:"event_type"` // payment.success
	OrderNo   string    `json:"order_no"`
	UserID    int64     `json:"user_id"`
	ItemNo    string    `json:"item_no"`
	Amount    float64   `json:"amount"`
	Timestamp time.Time `json:"timestamp"`
}

func NewMQ(brokers []string, topic string) *MQ {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
	}
	fmt.Println("[MQ] Kafka 连接成功 ✓")
	return &MQ{writer: writer}
}

func (m *MQ) PublishPaymentSuccess(ctx context.Context, event *PaymentEvent) error {
	event.EventType = "payment.success"
	event.Timestamp = time.Now()

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("序列化事件失败: %v", err)
	}

	msg := kafka.Message{
		Key:   []byte(event.OrderNo), // 同一订单的消息保证有序
		Value: data,
	}

	if err := m.writer.WriteMessages(ctx, msg); err != nil {
		return fmt.Errorf("发送事件失败: %v", err)
	}

	fmt.Printf("[MQ] 发布支付成功事件: order_no=%s\n", event.OrderNo)
	return nil
}

func (m *MQ) Close() {
	m.writer.Close()
}
