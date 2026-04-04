package matching

import (
	"github.com/hzx/matchengine/internal/models"
	"github.com/hzx/matchengine/internal/orderbook"
	"github.com/shopspring/decimal"
)

// StopMatcher 止损单撮合器
type StopMatcher struct {
	limitMatcher  *LimitMatcher
	marketMatcher *MarketMatcher
}

// NewStopMatcher 创建止损单撮合器
func NewStopMatcher() *StopMatcher {
	return &StopMatcher{
		limitMatcher:  NewLimitMatcher(),
		marketMatcher: NewMarketMatcher(),
	}
}

// CheckTrigger 检查止损单是否触发
func (m *StopMatcher) CheckTrigger(order *models.Order, lastPrice decimal.Decimal) bool {
	if order.Triggered {
		return true
	}

	// 止损买单：市场价格 <= 触发价
	if order.IsBuy() {
		if lastPrice.LessThanOrEqual(order.StopPrice) {
			order.Triggered = true
			return true
		}
	} else {
		// 止损卖单：市场价格 >= 触发价
		if lastPrice.GreaterThanOrEqual(order.StopPrice) {
			order.Triggered = true
			return true
		}
	}

	return false
}

// Match 撮合止损单
func (m *StopMatcher) Match(takerOrder *models.Order, ob *orderbook.OrderBook, tradeIDGen func() uint64) *models.TradeResult {
	result := models.NewTradeResult()
	result.TakerOrder = takerOrder

	// 检查是否已触发
	if !takerOrder.Triggered {
		return result
	}

	// 根据类型选择撮合器
	switch takerOrder.Type {
	case models.OrderTypeStopLimit:
		// 止损限价单：转为限价单撮合
		return m.limitMatcher.Match(takerOrder, ob, tradeIDGen)
	case models.OrderTypeStopMarket:
		// 止损市价单：转为市价单撮合
		return m.marketMatcher.Match(takerOrder, ob, tradeIDGen)
	}

	return result
}

// StopOrderManager 止损单管理器
type StopOrderManager struct {
	// 按价格排序的止损单
	buyStops  *StopOrderTree  // 止损买单（价格升序）
	sellStops *StopOrderTree  // 止损卖单（价格降序）
}

// NewStopOrderManager 创建止损单管理器
func NewStopOrderManager() *StopOrderManager {
	return &StopOrderManager{
		buyStops:  NewStopOrderTree(true),  // 升序
		sellStops: NewStopOrderTree(false), // 降序
	}
}

// AddOrder 添加止损单
func (m *StopOrderManager) AddOrder(order *models.Order) {
	if order.IsBuy() {
		m.buyStops.Insert(order.StopPrice, order)
	} else {
		m.sellStops.Insert(order.StopPrice, order)
	}
}

// RemoveOrder 移除止损单
func (m *StopOrderManager) RemoveOrder(order *models.Order) {
	if order.IsBuy() {
		m.buyStops.Delete(order.StopPrice, order.ID)
	} else {
		m.sellStops.Delete(order.StopPrice, order.ID)
	}
}

// GetTriggeredOrders 获取触发的订单
func (m *StopOrderManager) GetTriggeredOrders(lastPrice decimal.Decimal) []*models.Order {
	triggered := make([]*models.Order, 0)

	// 检查止损买单（市场价格下跌触发）
	orders := m.buyStops.GetOrdersBelowPrice(lastPrice)
	triggered = append(triggered, orders...)

	// 检查止损卖单（市场价格上涨触发）
	orders = m.sellStops.GetOrdersAbovePrice(lastPrice)
	triggered = append(triggered, orders...)

	return triggered
}

// StopOrderTree 止损单树
type StopOrderTree struct {
	orders map[decimal.Decimal]map[uint64]*models.Order
	prices []decimal.Decimal
	asc    bool
}

// NewStopOrderTree 创建止损单树
func NewStopOrderTree(asc bool) *StopOrderTree {
	return &StopOrderTree{
		orders: make(map[decimal.Decimal]map[uint64]*models.Order),
		prices: make([]decimal.Decimal, 0),
		asc:    asc,
	}
}

// Insert 插入订单
func (t *StopOrderTree) Insert(price decimal.Decimal, order *models.Order) {
	if _, exists := t.orders[price]; !exists {
		t.orders[price] = make(map[uint64]*models.Order)
		t.prices = append(t.prices, price)
		// 排序
		t.sortPrices()
	}
	t.orders[price][order.ID] = order
}

// Delete 删除订单
func (t *StopOrderTree) Delete(price decimal.Decimal, orderID uint64) {
	if orders, exists := t.orders[price]; exists {
		delete(orders, orderID)
		if len(orders) == 0 {
			delete(t.orders, price)
			// 从prices中删除
			for i, p := range t.prices {
				if p.Equal(price) {
					t.prices = append(t.prices[:i], t.prices[i+1:]...)
					break
				}
			}
		}
	}
}

// GetOrdersBelowPrice 获取低于指定价格的订单（用于止损买单）
func (t *StopOrderTree) GetOrdersBelowPrice(price decimal.Decimal) []*models.Order {
	result := make([]*models.Order, 0)
	for _, p := range t.prices {
		if p.LessThanOrEqual(price) {
			for _, order := range t.orders[p] {
				order.Triggered = true
				result = append(result, order)
			}
			// 清空这个价格档位
			delete(t.orders, p)
		} else {
			break
		}
	}
	// 更新prices
	t.prices = t.prices[len(result):]
	return result
}

// GetOrdersAbovePrice 获取高于指定价格的订单（用于止损卖单）
func (t *StopOrderTree) GetOrdersAbovePrice(price decimal.Decimal) []*models.Order {
	result := make([]*models.Order, 0)
	for i := len(t.prices) - 1; i >= 0; i-- {
		p := t.prices[i]
		if p.GreaterThanOrEqual(price) {
			for _, order := range t.orders[p] {
				order.Triggered = true
				result = append(result, order)
			}
			delete(t.orders, p)
		} else {
			break
		}
	}
	// 更新prices
	t.prices = t.prices[:len(t.prices)-len(result)]
	return result
}

// sortPrices 排序价格
func (t *StopOrderTree) sortPrices() {
	// 简单插入排序
	for i := 1; i < len(t.prices); i++ {
		for j := i; j > 0; j-- {
			shouldSwap := false
			if t.asc && t.prices[j].LessThan(t.prices[j-1]) {
				shouldSwap = true
			} else if !t.asc && t.prices[j].GreaterThan(t.prices[j-1]) {
				shouldSwap = true
			}
			if shouldSwap {
				t.prices[j], t.prices[j-1] = t.prices[j-1], t.prices[j]
			} else {
				break
			}
		}
	}
}
