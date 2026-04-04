package models

import (
	"time"

	"github.com/shopspring/decimal"
)

// Trade 成交记录
type Trade struct {
	ID           uint64          `json:"id"`            // 成交ID
	OrderID      uint64          `json:"order_id"`      // 挂单ID
	TakerOrderID uint64          `json:"taker_order_id"`// 吃单ID
	MakerOrderID uint64          `json:"maker_order_id"`// 挂单ID
	UserID       uint64          `json:"user_id"`       // 用户ID（吃单方）
	MakerUserID  uint64          `json:"maker_user_id"` // 用户ID（挂单方）
	Symbol       string          `json:"symbol"`        // 交易对
	Price        decimal.Decimal `json:"price"`         // 成交价格
	Amount       decimal.Decimal `json:"amount"`        // 成交数量
	Total        decimal.Decimal `json:"total"`         // 成交总额
	Fee          decimal.Decimal `json:"fee"`           // 手续费
	FeeAsset     string          `json:"fee_asset"`     // 手续费资产
	Side         OrderSide       `json:"side"`          // 交易方向
	TakerSide    OrderSide       `json:"taker_side"`    // 吃单方向
	CreatedAt    time.Time       `json:"created_at"`    // 成交时间
}

// NewTrade 创建成交记录
func NewTrade(id uint64, symbol string, takerOrder, makerOrder *Order, price, amount decimal.Decimal) *Trade {
	total := price.Mul(amount)
	
	var takerSide OrderSide
	var takerUserID, makerUserID uint64
	var fee decimal.Decimal
	
	if takerOrder.IsBuy() {
		takerSide = OrderSideBuy
		takerUserID = takerOrder.UserID
		makerUserID = makerOrder.UserID
		// 买单手续费 = 成交总额 * 费率
		fee = total.Mul(decimal.NewFromFloat(0.001)) // 默认0.1%费率
	} else {
		takerSide = OrderSideSell
		takerUserID = takerOrder.UserID
		makerUserID = makerOrder.UserID
		// 卖单手续费 = 成交数量 * 费率
		fee = amount.Mul(decimal.NewFromFloat(0.001))
	}

	return &Trade{
		ID:           id,
		TakerOrderID: takerOrder.ID,
		MakerOrderID: makerOrder.ID,
		UserID:       takerUserID,
		MakerUserID:  makerUserID,
		Symbol:       symbol,
		Price:        price,
		Amount:       amount,
		Total:        total,
		Fee:          fee,
		Side:         takerSide,
		TakerSide:    takerSide,
		CreatedAt:    time.Now(),
	}
}

// TradeResult 撮合结果
type TradeResult struct {
	TakerOrder *Order   `json:"taker_order"` // 吃单
	MakerOrder *Order   `json:"maker_order"` // 挂单
	Trade      *Trade   `json:"trade"`       // 成交记录
	Trades     []*Trade `json:"trades"`      // 批量成交记录
}

// NewTradeResult 创建撮合结果
func NewTradeResult() *TradeResult {
	return &TradeResult{
		Trades: make([]*Trade, 0),
	}
}

// AddTrade 添加成交记录
func (r *TradeResult) AddTrade(trade *Trade) {
	r.Trades = append(r.Trades, trade)
	r.Trade = trade
}

// HasTrade 是否有成交
func (r *TradeResult) HasTrade() bool {
	return len(r.Trades) > 0
}
