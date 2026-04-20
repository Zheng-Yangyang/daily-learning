package service

import (
	"context"
	"fmt"
	"payment-system/dal"
	"payment-system/model"
	"time"
)

type PaymentService struct {
	db    *dal.DB
	redis *dal.Redis
	mq    *dal.MQ
}

func NewPaymentService(db *dal.DB, redis *dal.Redis, mq *dal.MQ) *PaymentService {
	return &PaymentService{db: db, redis: redis, mq: mq}
}

// CreateOrderRequest 下单请求
type CreateOrderRequest struct {
	UserID int64  `json:"user_id" binding:"required"`
	ItemNo string `json:"item_no" binding:"required"`
}

// PayRequest 支付请求
type PayRequest struct {
	OrderNo string `json:"order_no" binding:"required"`
	UserID  int64  `json:"user_id" binding:"required"`
}

// ============================================================
// CreateOrder 创建订单
// ============================================================

func (s *PaymentService) CreateOrder(ctx context.Context, req *CreateOrderRequest) (*model.Order, error) {
	// 1. 查商品信息
	inv, err := s.db.GetInventory(ctx, req.ItemNo)
	if err != nil {
		return nil, err
	}
	if inv.Stock <= 0 {
		return nil, fmt.Errorf("商品库存不足")
	}

	// 2. 查用户信息
	user, err := s.db.GetUser(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	if user.Balance < inv.Price {
		return nil, fmt.Errorf("余额不足: 当前%.2f, 需要%.2f", user.Balance, inv.Price)
	}

	// 3. 生成订单号
	orderNo := fmt.Sprintf("ORD-%d-%d", req.UserID, time.Now().UnixMilli())

	// 4. 开启事务创建订单
	tx, err := s.db.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	order := &model.Order{
		OrderNo: orderNo,
		UserID:  req.UserID,
		ItemNo:  req.ItemNo,
		Amount:  inv.Price,
		Status:  model.OrderStatusPending,
	}

	if err := s.db.CreateOrder(ctx, tx, order); err != nil {
		return nil, fmt.Errorf("创建订单失败: %v", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	fmt.Printf("[OrderService] 订单创建成功: %s amount=%.2f\n", orderNo, inv.Price)
	return order, nil
}

// ============================================================
// Pay 支付
// 这是整个系统最核心的方法，涉及：
// 1. 分布式锁防重复支付
// 2. 幂等检查
// 3. 事务保证原子性
// 4. Kafka 发布事件
// ============================================================

func (s *PaymentService) Pay(ctx context.Context, req *PayRequest) (*model.Payment, error) {
	// ── 第一步：分布式锁，防止同一笔订单并发支付 ──
	lockKey := fmt.Sprintf("pay:lock:%s", req.OrderNo)
	lock := s.redis.NewLock(lockKey, 30*time.Second)

	ok, err := lock.TryLock(ctx)
	if err != nil {
		return nil, fmt.Errorf("加锁失败: %v", err)
	}
	if !ok {
		return nil, fmt.Errorf("该订单正在处理中，请勿重复提交")
	}
	defer lock.Unlock(ctx)

	fmt.Printf("[PayService] 获取锁成功: %s\n", req.OrderNo)

	// ── 第二步：幂等检查，查支付流水是否已存在 ──
	existing, err := s.db.GetPaymentByOrderNo(ctx, req.OrderNo)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		fmt.Printf("[PayService] 幂等返回，订单已支付: %s\n", req.OrderNo)
		return existing, nil // 已支付，直接返回，不重复处理
	}

	// ── 第三步：查订单 ──
	order, err := s.db.GetOrderByNo(ctx, req.OrderNo)
	if err != nil {
		return nil, err
	}
	if order.UserID != req.UserID {
		return nil, fmt.Errorf("订单不属于该用户")
	}
	if order.Status != model.OrderStatusPending {
		return nil, fmt.Errorf("订单状态异常: %d", order.Status)
	}

	// ── 第四步：开启事务，扣款 + 更新订单 + 写支付流水 ──
	tx, err := s.db.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 扣款（内部用 SELECT FOR UPDATE 防并发超扣）
	if err := s.db.DeductBalance(ctx, tx, req.UserID, order.Amount); err != nil {
		return nil, fmt.Errorf("扣款失败: %v", err)
	}

	// 更新订单状态
	if err := s.db.UpdateOrderStatus(ctx, tx, req.OrderNo, model.OrderStatusPaid); err != nil {
		return nil, fmt.Errorf("更新订单状态失败: %v", err)
	}

	// 写支付流水
	paymentNo := fmt.Sprintf("PAY-%d-%d", req.UserID, time.Now().UnixMilli())
	payment := &model.Payment{
		PaymentNo: paymentNo,
		OrderNo:   req.OrderNo,
		UserID:    req.UserID,
		Amount:    order.Amount,
		Status:    model.PaymentStatusSuccess,
	}
	if err := s.db.CreatePayment(ctx, tx, payment); err != nil {
		return nil, fmt.Errorf("写支付流水失败: %v", err)
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("事务提交失败: %v", err)
	}

	fmt.Printf("[PayService] 支付成功: order_no=%s payment_no=%s amount=%.2f\n",
		req.OrderNo, paymentNo, order.Amount)

	// ── 第五步：发 Kafka 事件通知下游（库存服务、通知服务）──
	// 注意：事务已提交，这里发失败了也没关系，可以重试
	// 这就是"最终一致性"：支付先成功，下游异步处理
	go func() {
		event := &dal.PaymentEvent{
			OrderNo: req.OrderNo,
			UserID:  req.UserID,
			ItemNo:  order.ItemNo,
			Amount:  order.Amount,
		}
		if err := s.mq.PublishPaymentSuccess(context.Background(), event); err != nil {
			fmt.Printf("[PayService] 发布事件失败（需人工补偿）: %v\n", err)
		}
	}()

	return payment, nil
}
