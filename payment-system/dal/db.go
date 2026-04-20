package dal

import (
	"context"
	"database/sql"
	"fmt"
	"payment-system/model"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type DB struct {
	db *sql.DB
}

func NewDB(dsn string) (*DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(time.Hour)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("MySQL 连接失败: %v", err)
	}
	fmt.Println("[DB] MySQL 连接成功 ✓")
	return &DB{db: db}, nil
}

// ============================================================
// User 相关操作
// ============================================================

func (d *DB) GetUser(ctx context.Context, userID int64) (*model.User, error) {
	u := &model.User{}
	err := d.db.QueryRowContext(ctx,
		"SELECT id, name, balance FROM users WHERE id = ?", userID,
	).Scan(&u.ID, &u.Name, &u.Balance)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("用户不存在: %d", userID)
	}
	return u, err
}

// DeductBalance 扣款，使用 SELECT FOR UPDATE 防止并发超扣
// 必须在事务内调用
func (d *DB) DeductBalance(ctx context.Context, tx *sql.Tx, userID int64, amount float64) error {
	var balance float64
	err := tx.QueryRowContext(ctx,
		"SELECT balance FROM users WHERE id = ? FOR UPDATE", userID,
	).Scan(&balance)
	if err != nil {
		return err
	}
	if balance < amount {
		return fmt.Errorf("余额不足: 当前%.2f, 需要%.2f", balance, amount)
	}
	_, err = tx.ExecContext(ctx,
		"UPDATE users SET balance = balance - ? WHERE id = ?", amount, userID,
	)
	return err
}

// ============================================================
// Inventory 相关操作
// ============================================================

func (d *DB) GetInventory(ctx context.Context, itemNo string) (*model.Inventory, error) {
	inv := &model.Inventory{}
	err := d.db.QueryRowContext(ctx,
		"SELECT id, item_name, item_no, stock, price FROM inventory WHERE item_no = ?", itemNo,
	).Scan(&inv.ID, &inv.ItemName, &inv.ItemNo, &inv.Stock, &inv.Price)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("商品不存在: %s", itemNo)
	}
	return inv, err
}

func (d *DB) DeductStock(ctx context.Context, itemNo string) error {
	result, err := d.db.ExecContext(ctx,
		// 乐观锁：stock > 0 才更新，防止超卖
		"UPDATE inventory SET stock = stock - 1 WHERE item_no = ? AND stock > 0", itemNo,
	)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("库存不足: %s", itemNo)
	}
	return nil
}

// ============================================================
// Order 相关操作
// ============================================================

func (d *DB) CreateOrder(ctx context.Context, tx *sql.Tx, order *model.Order) error {
	_, err := tx.ExecContext(ctx,
		"INSERT INTO orders (order_no, user_id, item_no, amount, status) VALUES (?, ?, ?, ?, ?)",
		order.OrderNo, order.UserID, order.ItemNo, order.Amount, order.Status,
	)
	return err
}

func (d *DB) GetOrderByNo(ctx context.Context, orderNo string) (*model.Order, error) {
	o := &model.Order{}
	err := d.db.QueryRowContext(ctx,
		"SELECT id, order_no, user_id, item_no, amount, status FROM orders WHERE order_no = ?", orderNo,
	).Scan(&o.ID, &o.OrderNo, &o.UserID, &o.ItemNo, &o.Amount, &o.Status)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("订单不存在: %s", orderNo)
	}
	return o, err
}

func (d *DB) UpdateOrderStatus(ctx context.Context, tx *sql.Tx, orderNo string, status int8) error {
	_, err := tx.ExecContext(ctx,
		"UPDATE orders SET status = ? WHERE order_no = ?", status, orderNo,
	)
	return err
}

// ============================================================
// Payment 相关操作
// ============================================================

func (d *DB) CreatePayment(ctx context.Context, tx *sql.Tx, payment *model.Payment) error {
	_, err := tx.ExecContext(ctx,
		"INSERT INTO payments (payment_no, order_no, user_id, amount, status) VALUES (?, ?, ?, ?, ?)",
		payment.PaymentNo, payment.OrderNo, payment.UserID, payment.Amount, payment.Status,
	)
	return err
}

func (d *DB) GetPaymentByOrderNo(ctx context.Context, orderNo string) (*model.Payment, error) {
	p := &model.Payment{}
	err := d.db.QueryRowContext(ctx,
		"SELECT id, payment_no, order_no, user_id, amount, status FROM payments WHERE order_no = ?", orderNo,
	).Scan(&p.ID, &p.PaymentNo, &p.OrderNo, &p.UserID, &p.Amount, &p.Status)
	if err == sql.ErrNoRows {
		return nil, nil // 不存在不报错，用于幂等判断
	}
	return p, err
}

// BeginTx 开启事务
func (d *DB) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return d.db.BeginTx(ctx, nil)
}
