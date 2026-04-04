package models

import (
	"sync"

	"github.com/shopspring/decimal"
)

// MarketStatus 市场状态
type MarketStatus int

const (
	MarketStatusActive MarketStatus = iota + 1
	MarketStatusSuspended
	MarketStatusClosed
)

// Market 交易市场
type Market struct {
	Symbol         string          `json:"symbol"`
	BaseAsset      string          `json:"base_asset"`
	QuoteAsset     string          `json:"quote_asset"`
	Status         MarketStatus    `json:"status"`
	BasePrecision  int             `json:"base_precision"`
	QuotePrecision int             `json:"quote_precision"`
	MinAmount      decimal.Decimal `json:"min_amount"`       // 最小下单量
	MinTotal       decimal.Decimal `json:"min_total"`        // 最小下单金额
	PriceTick      decimal.Decimal `json:"price_tick"`       // 价格变动单位
	AmountTick     decimal.Decimal `json:"amount_tick"`      // 数量变动单位
	mu             sync.RWMutex
}

// NewMarket 创建市场
func NewMarket(symbol, baseAsset, quoteAsset string, basePrecision, quotePrecision int,
	minAmount, minTotal, priceTick, amountTick decimal.Decimal) *Market {
	return &Market{
		Symbol:         symbol,
		BaseAsset:      baseAsset,
		QuoteAsset:     quoteAsset,
		Status:         MarketStatusActive,
		BasePrecision:  basePrecision,
		QuotePrecision: quotePrecision,
		MinAmount:      minAmount,
		MinTotal:       minTotal,
		PriceTick:      priceTick,
		AmountTick:     amountTick,
	}
}

// IsActive 市场是否活跃
func (m *Market) IsActive() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Status == MarketStatusActive
}

// Suspend 暂停市场
func (m *Market) Suspend() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Status = MarketStatusSuspended
}

// Resume 恢复市场
func (m *Market) Resume() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Status = MarketStatusActive
}

// Close 关闭市场
func (m *Market) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Status = MarketStatusClosed
}

// ValidatePrice 验证价格
func (m *Market) ValidatePrice(price decimal.Decimal) bool {
	if price.LessThanOrEqual(decimal.Zero) {
		return false
	}
	// 价格必须是价格变动单位的整数倍
	remainder := price.Mod(m.PriceTick)
	return remainder.IsZero()
}

// ValidateAmount 验证数量
func (m *Market) ValidateAmount(amount decimal.Decimal) bool {
	if amount.LessThan(m.MinAmount) {
		return false
	}
	// 数量必须是数量变动单位的整数倍
	remainder := amount.Mod(m.AmountTick)
	return remainder.IsZero()
}

// ValidateTotal 验证总金额
func (m *Market) ValidateTotal(total decimal.Decimal) bool {
	return total.GreaterThanOrEqual(m.MinTotal)
}

// ValidateOrder 验证订单
func (m *Market) ValidateOrder(order *Order) bool {
	// 检查市场状态
	if !m.IsActive() {
		return false
	}

	// 检查价格
	if order.Type != OrderTypeMarket && order.Type != OrderTypeStopMarket {
		if !m.ValidatePrice(order.Price) {
			return false
		}
	}

	// 检查数量
	if !m.ValidateAmount(order.Amount) {
		return false
	}

	// 检查最小金额
	if order.Type == OrderTypeLimit || order.Type == OrderTypeStopLimit {
		total := order.Price.Mul(order.Amount)
		if !m.ValidateTotal(total) {
			return false
		}
	}

	return true
}

// FormatPrice 格式化价格
func (m *Market) FormatPrice(price decimal.Decimal) string {
	return price.StringFixed(int32(m.QuotePrecision))
}

// FormatAmount 格式化数量
func (m *Market) FormatAmount(amount decimal.Decimal) string {
	return amount.StringFixed(int32(m.BasePrecision))
}

// Ticker 行情数据
type Ticker struct {
	Symbol    string          `json:"symbol"`
	LastPrice decimal.Decimal `json:"last_price"` // 最新价
	BidPrice  decimal.Decimal `json:"bid_price"`  // 买一价
	BidAmount decimal.Decimal `json:"bid_amount"` // 买一量
	AskPrice  decimal.Decimal `json:"ask_price"`  // 卖一价
	AskAmount decimal.Decimal `json:"ask_amount"` // 卖一量
	High24h   decimal.Decimal `json:"high_24h"`   // 24小时最高价
	Low24h    decimal.Decimal `json:"low_24h"`    // 24小时最低价
	Volume24h decimal.Decimal `json:"volume_24h"` // 24小时成交量
	Amount24h decimal.Decimal `json:"amount_24h"` // 24小时成交额
	Timestamp int64           `json:"timestamp"`  // 时间戳
}

// DepthLevel 盘口深度档位
type DepthLevel struct {
	Price  decimal.Decimal `json:"price"`
	Amount decimal.Decimal `json:"amount"`
}

// Depth 盘口深度
type Depth struct {
	Symbol string        `json:"symbol"`
	Bids   []*DepthLevel `json:"bids"` // 买单
	Asks   []*DepthLevel `json:"asks"` // 卖单
}
