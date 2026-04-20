package handler

import (
	"context"
	"fmt"
	"time"
	order "order-service/kitex_gen/order"
)

type OrderServiceImpl struct{}

func (s *OrderServiceImpl) CreateOrder(ctx context.Context, req *order.CreateOrderRequest) (*order.CreateOrderResponse, error) {
	fmt.Printf("[OrderService] 收到请求: user_id=%d amount=%.2f item=%s\n",
		req.UserId, req.Amount, req.ItemName)

	// 模拟业务处理
	time.Sleep(10 * time.Millisecond)

	orderNo := fmt.Sprintf("ORD-%d-%d", req.UserId, time.Now().UnixMilli())

	resp := &order.CreateOrderResponse{
		OrderNo: orderNo,
		Status:  "created",
	}

	fmt.Printf("[OrderService] 订单创建成功: %s\n", orderNo)
	return resp, nil
}
