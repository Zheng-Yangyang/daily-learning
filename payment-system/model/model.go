package model

import "time"

type User struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Balance   float64   `json:"balance"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Inventory struct {
	ID        int64     `json:"id"`
	ItemName  string    `json:"item_name"`
	ItemNo    string    `json:"item_no"`
	Stock     int       `json:"stock"`
	Price     float64   `json:"price"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Order struct {
	ID        int64     `json:"id"`
	OrderNo   string    `json:"order_no"`
	UserID    int64     `json:"user_id"`
	ItemNo    string    `json:"item_no"`
	Amount    float64   `json:"amount"`
	Status    int8      `json:"status"` // 0待支付 1已支付 2已取消 3已退款
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Payment struct {
	ID        int64     `json:"id"`
	PaymentNo string    `json:"payment_no"`
	OrderNo   string    `json:"order_no"`
	UserID    int64     `json:"user_id"`
	Amount    float64   `json:"amount"`
	Status    int8      `json:"status"` // 0处理中 1成功 2失败
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// 订单状态常量
const (
	OrderStatusPending  int8 = 0
	OrderStatusPaid     int8 = 1
	OrderStatusCanceled int8 = 2
	OrderStatusRefunded int8 = 3
)

// 支付状态常量
const (
	PaymentStatusProcessing int8 = 0
	PaymentStatusSuccess    int8 = 1
	PaymentStatusFailed     int8 = 2
)
