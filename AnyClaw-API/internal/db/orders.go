package db

import (
	"database/sql"
	"fmt"
)

// OrderWithUser 订单（管理员视图含用户邮箱）
type OrderWithUser struct {
	Order
	UserEmail string `json:"user_email,omitempty"`
}

type Order struct {
	ID          int64   `json:"id"`
	UserID      int64   `json:"user_id"`
	PlanID      string  `json:"plan_id"`
	Energy      int     `json:"energy"`
	PriceCny    int     `json:"price_cny"`
	Channel     string  `json:"channel"`
	Status      string  `json:"status"`
	OutTradeNo  string  `json:"out_trade_no"`
	ExternalID  string  `json:"external_id,omitempty"`
	PaidAt      *string `json:"paid_at,omitempty"`
	CreatedAt   string  `json:"created_at"`
}

func (d *DB) CreateOrder(userID int64, planID string, energy, priceCny int, channel, outTradeNo string) (*Order, error) {
	res, err := d.Exec(
		"INSERT INTO orders (user_id, plan_id, energy, price_cny, channel, status, out_trade_no) VALUES (?, ?, ?, ?, ?, 'pending', ?)",
		userID, planID, energy, priceCny, channel, outTradeNo,
	)
	if err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}
	id, _ := res.LastInsertId()
	return d.GetOrderByID(id)
}

func (d *DB) GetOrderByID(id int64) (*Order, error) {
	var o Order
	err := d.QueryRow(
		`SELECT id, user_id, plan_id, energy, price_cny, channel, status, out_trade_no, COALESCE(external_id,''), paid_at, created_at FROM orders WHERE id = ?`,
		id,
	).Scan(&o.ID, &o.UserID, &o.PlanID, &o.Energy, &o.PriceCny, &o.Channel, &o.Status, &o.OutTradeNo, &o.ExternalID, &o.PaidAt, &o.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (d *DB) GetOrderByOutTradeNo(outTradeNo string) (*Order, error) {
	var o Order
	err := d.QueryRow(
		`SELECT id, user_id, plan_id, energy, price_cny, channel, status, out_trade_no, COALESCE(external_id,''), paid_at, created_at FROM orders WHERE out_trade_no = ?`,
		outTradeNo,
	).Scan(&o.ID, &o.UserID, &o.PlanID, &o.Energy, &o.PriceCny, &o.Channel, &o.Status, &o.OutTradeNo, &o.ExternalID, &o.PaidAt, &o.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// ListOrdersForUser 列出用户的订单，按创建时间倒序
func (d *DB) ListOrdersForUser(userID int64, limit int) ([]*Order, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.Query(
		`SELECT id, user_id, plan_id, energy, price_cny, channel, status, out_trade_no, COALESCE(external_id,''), paid_at, created_at
		 FROM orders WHERE user_id = ? ORDER BY created_at DESC LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.UserID, &o.PlanID, &o.Energy, &o.PriceCny, &o.Channel, &o.Status, &o.OutTradeNo, &o.ExternalID, &o.PaidAt, &o.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, &o)
	}
	return list, rows.Err()
}

// ListOrdersAll 列出所有订单（管理员），含用户邮箱，按创建时间倒序
func (d *DB) ListOrdersAll(limit int) ([]*OrderWithUser, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.Query(
		`SELECT o.id, o.user_id, o.plan_id, o.energy, o.price_cny, o.channel, o.status, o.out_trade_no,
		 COALESCE(o.external_id,''), o.paid_at, o.created_at, COALESCE(u.email,'')
		 FROM orders o LEFT JOIN users u ON o.user_id = u.id ORDER BY o.created_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*OrderWithUser
	for rows.Next() {
		var ow OrderWithUser
		if err := rows.Scan(&ow.ID, &ow.UserID, &ow.PlanID, &ow.Energy, &ow.PriceCny, &ow.Channel, &ow.Status, &ow.OutTradeNo, &ow.ExternalID, &ow.PaidAt, &ow.CreatedAt, &ow.UserEmail); err != nil {
			return nil, err
		}
		list = append(list, &ow)
	}
	return list, rows.Err()
}

// MarkOrderPaid 标记订单已支付，返回是否实际更新（用于幂等：仅首次支付时加金币）
func (d *DB) MarkOrderPaid(outTradeNo, externalID string) (bool, error) {
	res, err := d.Exec(
		"UPDATE orders SET status = 'paid', external_id = ?, paid_at = NOW() WHERE out_trade_no = ? AND status = 'pending'",
		externalID, outTradeNo,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
