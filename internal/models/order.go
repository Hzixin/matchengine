package models

import (
	"time"

	"github.com/shopspring/decimal"
)

// OrderSide 订单方向
type OrderSide int

const (
	OrderSideBuy OrderSide = iota + 1
	OrderSideSell
)

func (s OrderSide) String() string {
	switch s {
	case OrderSideBuy:
		return "buy"
	case OrderSideSell:
		return "sell"
	default:
		return "unknown"
	}
}

// OrderType 订单类型
type OrderType int

const (
	OrderTypeLimit OrderType = iota + 1 // 限价单
	OrderTypeMarket                     // 市价单
	OrderTypeStopLimit                  // 止损限价单
	OrderTypeStopMarket                 // 止损市价单
	OrderTypeIceberg                    // 冰山单
)

func (t OrderType) String() string {
	switch t {
	case OrderTypeLimit:
		return "limit"
	case OrderTypeMarket:
		return "market"
	case OrderTypeStopLimit:
		return "stop_limit"
	case OrderTypeStopMarket:
		return "stop_market"
	case OrderTypeIceberg:
		return "iceberg"
	default:
		return "unknown"
	}
}

// OrderStatus 订单状态
type OrderStatus int

const (
	OrderStatusNew OrderStatus = iota + 1
	OrderStatusPartiallyFilled
	OrderStatusFilled
	OrderStatusCancelled
	OrderStatusExpired
	OrderStatusRejected
)

func (s OrderStatus) String() string {
	switch s {
	case OrderStatusNew:
		return "new"
	case OrderStatusPartiallyFilled:
		return "partially_filled"
	case OrderStatusFilled:
		return "filled"
	case OrderStatusCancelled:
		return "cancelled"
	case OrderStatusExpired:
		return "expired"
	case OrderStatusRejected:
		return "rejected"
	default:
		return "unknown"
	}
}

// TimeInForce 订单有效期类型
type TimeInForce int

const (
	TimeInForceGTC TimeInForce = iota + 1 // Good Till Cancel - 撤销前有效
	TimeInForceIOC                        // Immediate Or Cancel - 立即成交或取消
	TimeInForceFOK                        // Fill Or Kill - 全部成交或取消
	TimeInForceGTX                        // Good Till Crossing - 只做Maker
)

func (t TimeInForce) String() string {
	switch t {
	case TimeInForceGTC:
		return "GTC"
	case TimeInForceIOC:
		return "IOC"
	case TimeInForceFOK:
		return "FOK"
	case TimeInForceGTX:
		return "GTX"
	default:
		return "unknown"
	}
}

// Order 订单模型
type Order struct {
	ID           uint64          `json:"id"`             // 订单ID
	UserID       uint64          `json:"user_id"`        // 用户ID
	Symbol       string          `json:"symbol"`         // 交易对
	Side         OrderSide       `json:"side"`           // 方向
	Type         OrderType       `json:"type"`           // 类型
	Status       OrderStatus     `json:"status"`         // 状态
	Price        decimal.Decimal `json:"price"`          // 价格
	Amount       decimal.Decimal `json:"amount"`         // 原始数量
	FilledAmount decimal.Decimal `json:"filled_amount"`  // 已成交数量
	RemainAmount decimal.Decimal `json:"remain_amount"`  // 剩余数量
	FilledTotal  decimal.Decimal `json:"filled_total"`   // 已成交总额
	Fee          decimal.Decimal `json:"fee"`            // 手续费
	FeeAsset     string          `json:"fee_asset"`      // 手续费资产
	TimeInForce  TimeInForce     `json:"time_in_force"`  // 有效期类型
	StopPrice    decimal.Decimal `json:"stop_price"`     // 止损触发价
	Visible      decimal.Decimal `json:"visible"`        // 冰山单可见数量
	Hidden       decimal.Decimal `json:"hidden"`         // 冰山单隐藏数量
	CreatedAt    time.Time       `json:"created_at"`     // 创建时间
	UpdatedAt    time.Time       `json:"updated_at"`     // 更新时间
	Triggered    bool            `json:"triggered"`      // 止损单是否已触发
}

// NewOrder 创建新订单
func NewOrder(id, userID uint64, symbol string, side OrderSide, orderType OrderType,
	price, amount decimal.Decimal, timeInForce TimeInForce) *Order {
	now := time.Now()
	return &Order{
		ID:           id,
		UserID:       userID,
		Symbol:       symbol,
		Side:         side,
		Type:         orderType,
		Status:       OrderStatusNew,
		Price:        price,
		Amount:       amount,
		FilledAmount: decimal.Zero,
		RemainAmount: amount,
		FilledTotal:  decimal.Zero,
		Fee:          decimal.Zero,
		TimeInForce:  timeInForce,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// IsBuy 是否是买单
func (o *Order) IsBuy() bool {
	return o.Side == OrderSideBuy
}

// IsSell 是否是卖单
func (o *Order) IsSell() bool {
	return o.Side == OrderSideSell
}

// IsFinished 订单是否已结束
func (o *Order) IsFinished() bool {
	return o.Status == OrderStatusFilled ||
		o.Status == OrderStatusCancelled ||
		o.Status == OrderStatusExpired ||
		o.Status == OrderStatusRejected
}

// CanCancel 是否可以撤销
func (o *Order) CanCancel() bool {
	return o.Status == OrderStatusNew || o.Status == OrderStatusPartiallyFilled
}

// UpdateFilled 更新成交信息
func (o *Order) UpdateFilled(filledAmount, filledTotal decimal.Decimal) {
	o.FilledAmount = o.FilledAmount.Add(filledAmount)
	o.RemainAmount = o.Amount.Sub(o.FilledAmount)
	o.FilledTotal = o.FilledTotal.Add(filledTotal)

	if o.RemainAmount.IsZero() {
		o.Status = OrderStatusFilled
	} else if o.FilledAmount.GreaterThan(decimal.Zero) {
		o.Status = OrderStatusPartiallyFilled
	}
	o.UpdatedAt = time.Now()
}

// AveragePrice 平均成交价
func (o *Order) AveragePrice() decimal.Decimal {
	if o.FilledAmount.IsZero() {
		return decimal.Zero
	}
	return o.FilledTotal.Div(o.FilledAmount)
}

// Clone 克隆订单
func (o *Order) Clone() *Order {
	return &Order{
		ID:           o.ID,
		UserID:       o.UserID,
		Symbol:       o.Symbol,
		Side:         o.Side,
		Type:         o.Type,
		Status:       o.Status,
		Price:        o.Price,
		Amount:       o.Amount,
		FilledAmount: o.FilledAmount,
		RemainAmount: o.RemainAmount,
		FilledTotal:  o.FilledTotal,
		Fee:          o.Fee,
		FeeAsset:     o.FeeAsset,
		TimeInForce:  o.TimeInForce,
		StopPrice:    o.StopPrice,
		Visible:      o.Visible,
		Hidden:       o.Hidden,
		CreatedAt:    o.CreatedAt,
		UpdatedAt:    o.UpdatedAt,
		Triggered:    o.Triggered,
	}
}
