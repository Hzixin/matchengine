package matching

import (
	"github.com/hzx/matchengine/internal/models"
	"github.com/hzx/matchengine/internal/orderbook"
	"github.com/shopspring/decimal"
)

// IcebergMatcher 冰山单撮合器
type IcebergMatcher struct {
	limitMatcher *LimitMatcher
}

// NewIcebergMatcher 创建冰山单撮合器
func NewIcebergMatcher() *IcebergMatcher {
	return &IcebergMatcher{
		limitMatcher: NewLimitMatcher(),
	}
}

// Match 撮合冰山单
func (m *IcebergMatcher) Match(takerOrder *models.Order, ob *orderbook.OrderBook, tradeIDGen func() uint64) *models.TradeResult {
	result := models.NewTradeResult()
	result.TakerOrder = takerOrder

	// 冰山单作为taker时，使用可见数量撮合
	// 但实际可以成交更多（因为有隐藏数量）

	// 获取对手盘
	var oppositeBook *RBTreeWrapper
	if takerOrder.IsBuy() {
		oppositeBook = NewRBTreeWrapper(ob.Asks, false)
	} else {
		oppositeBook = NewRBTreeWrapper(ob.Bids, true)
	}

	// 撮合循环
	for takerOrder.RemainAmount.GreaterThan(decimal.Zero) {
		bestLevel := oppositeBook.Best()
		if bestLevel == nil {
			break
		}

		// 检查价格
		if takerOrder.IsBuy() && takerOrder.Price.LessThan(bestLevel.Price) {
			break
		}
		if takerOrder.IsSell() && takerOrder.Price.GreaterThan(bestLevel.Price) {
			break
		}

		// 撮合
		m.matchAtPriceLevel(takerOrder, bestLevel, result, tradeIDGen)
	}

	return result
}

// matchAtPriceLevel 在价格档位撮合冰山单
func (m *IcebergMatcher) matchAtPriceLevel(takerOrder *models.Order, priceLevel *orderbook.PriceLevel,
	result *models.TradeResult, tradeIDGen func() uint64) {
	for {
		makerOrder := priceLevel.GetFirst()
		if makerOrder == nil {
			return
		}

		// 计算成交量
		matchAmount := decimal.Min(takerOrder.RemainAmount, makerOrder.RemainAmount)
		if matchAmount.LessThanOrEqual(decimal.Zero) {
			return
		}

		// 创建成交记录
		trade := models.NewTrade(tradeIDGen(), takerOrder.Symbol, takerOrder, makerOrder, priceLevel.Price, matchAmount)
		result.AddTrade(trade)
		result.MakerOrder = makerOrder

		// 更新订单
		tradeTotal := priceLevel.Price.Mul(matchAmount)
		takerOrder.UpdateFilled(matchAmount, tradeTotal)
		makerOrder.UpdateFilled(matchAmount, tradeTotal)

		if makerOrder.IsFinished() {
			priceLevel.RemoveOrder(makerOrder.ID)
		}
	}
}

// IcebergOrderManager 冰山单管理器
type IcebergOrderManager struct {
	orders map[uint64]*IcebergOrderState
}

// IcebergOrderState 冰山单状态
type IcebergOrderState struct {
	Order         *models.Order
	VisibleAmount decimal.Decimal // 当前可见数量
	HiddenAmount  decimal.Decimal // 隐藏数量
	DisplaySize   decimal.Decimal // 每次显示的数量
	RandFactor    decimal.Decimal // 随机因子（0-0.5）
}

// NewIcebergOrderManager 创建冰山单管理器
func NewIcebergOrderManager() *IcebergOrderManager {
	return &IcebergOrderManager{
		orders: make(map[uint64]*IcebergOrderState),
	}
}

// Register 注册冰山单
func (m *IcebergOrderManager) Register(order *models.Order, displaySize decimal.Decimal) {
	state := &IcebergOrderState{
		Order:         order,
		VisibleAmount: order.Visible,
		HiddenAmount:  order.Hidden,
		DisplaySize:   displaySize,
		RandFactor:    decimal.NewFromFloat(0.1), // 10%随机波动
	}
	m.orders[order.ID] = state
}

// Unregister 取消注册
func (m *IcebergOrderManager) Unregister(orderID uint64) {
	delete(m.orders, orderID)
}

// GetVisibleAmount 获取可见数量
func (m *IcebergOrderManager) GetVisibleAmount(orderID uint64) decimal.Decimal {
	if state, exists := m.orders[orderID]; exists {
		return state.VisibleAmount
	}
	return decimal.Zero
}

// OnTrade 成交后更新冰山单状态
func (m *IcebergOrderManager) OnTrade(orderID uint64, filledAmount decimal.Decimal) {
	state, exists := m.orders[orderID]
	if !exists {
		return
	}

	// 更新剩余数量
	totalRemain := state.Order.RemainAmount

	// 如果可见数量成交完，补充新的可见数量
	if state.VisibleAmount.LessThanOrEqual(filledAmount) {
		// 重新显示一部分
		if state.HiddenAmount.GreaterThan(decimal.Zero) {
			// 计算新的可见数量（带随机因子）
			newVisible := decimal.Min(state.DisplaySize, state.HiddenAmount)
			state.VisibleAmount = newVisible
			state.HiddenAmount = state.HiddenAmount.Sub(newVisible)
		} else {
			state.VisibleAmount = decimal.Zero
		}
	} else {
		state.VisibleAmount = state.VisibleAmount.Sub(filledAmount)
	}

	// 更新订单的可见数量
	if totalRemain.LessThan(state.VisibleAmount) {
		state.VisibleAmount = totalRemain
	}
}

// IsIcebergOrder 检查是否是冰山单
func (m *IcebergOrderManager) IsIcebergOrder(orderID uint64) bool {
	_, exists := m.orders[orderID]
	return exists
}
