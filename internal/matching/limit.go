package matching

import (
	"github.com/hzx/matchengine/internal/models"
	"github.com/hzx/matchengine/internal/orderbook"
	"github.com/shopspring/decimal"
)

// Matcher 撮合器接口
type Matcher interface {
	Match(order *models.Order, ob *orderbook.OrderBook, tradeIDGen func() uint64) *models.TradeResult
}

// LimitMatcher 限价单撮合器
type LimitMatcher struct{}

// NewLimitMatcher 创建限价单撮合器
func NewLimitMatcher() *LimitMatcher {
	return &LimitMatcher{}
}

// Match 撮合限价单
func (m *LimitMatcher) Match(takerOrder *models.Order, ob *orderbook.OrderBook, tradeIDGen func() uint64) *models.TradeResult {
	result := models.NewTradeResult()
	result.TakerOrder = takerOrder

	// 撮合循环
	for takerOrder.RemainAmount.GreaterThan(decimal.Zero) {
		// 获取对手盘最优价
		var bestLevel *orderbook.PriceLevel
		if takerOrder.IsBuy() {
			bestLevel = ob.Asks.Min()
		} else {
			bestLevel = ob.Bids.Max()
		}

		if bestLevel == nil {
			break
		}

		// 检查价格是否满足
		if takerOrder.IsBuy() {
			// 买单价格必须 >= 卖一价
			if takerOrder.Price.LessThan(bestLevel.Price) {
				break
			}
		} else {
			// 卖单价格必须 <= 买一价
			if takerOrder.Price.GreaterThan(bestLevel.Price) {
				break
			}
		}

		// 在这个价格档位撮合
		empty := m.matchAtPriceLevel(takerOrder, bestLevel, result, tradeIDGen)

		// 如果价格档位为空，从订单簿删除
		if empty {
			if takerOrder.IsBuy() {
				ob.Asks.Delete(bestLevel.Price)
			} else {
				ob.Bids.Delete(bestLevel.Price)
			}
		}

		// IOC订单：立即取消未成交部分
		if takerOrder.TimeInForce == models.TimeInForceIOC {
			break
		}

		// GTX订单：只做Maker，不立即成交
		if takerOrder.TimeInForce == models.TimeInForceGTX {
			break
		}
	}

	return result
}

// matchAtPriceLevel 在价格档位撮合，返回价格档位是否为空
func (m *LimitMatcher) matchAtPriceLevel(takerOrder *models.Order, priceLevel *orderbook.PriceLevel,
	result *models.TradeResult, tradeIDGen func() uint64) bool {

	for takerOrder.RemainAmount.GreaterThan(decimal.Zero) {
		// 获取第一个订单
		makerOrder := priceLevel.GetFirst()
		if makerOrder == nil {
			return true // 价格档位为空
		}

		// 检查是否可以成交
		matchAmount := decimal.Min(takerOrder.RemainAmount, makerOrder.RemainAmount)
		if matchAmount.LessThanOrEqual(decimal.Zero) {
			return priceLevel.IsEmpty()
		}

		// 创建成交记录
		trade := models.NewTrade(tradeIDGen(), takerOrder.Symbol, takerOrder, makerOrder, priceLevel.Price, matchAmount)
		result.AddTrade(trade)
		result.MakerOrder = makerOrder

		// 更新订单
		tradeTotal := priceLevel.Price.Mul(matchAmount)
		takerOrder.UpdateFilled(matchAmount, tradeTotal)
		makerOrder.UpdateFilled(matchAmount, tradeTotal)

		// 如果maker订单完全成交，移除
		if makerOrder.IsFinished() {
			priceLevel.RemoveOrder(makerOrder.ID)
		}
	}

	return priceLevel.IsEmpty()
}

// RBTreeWrapper 红黑树包装器
type RBTreeWrapper struct {
	tree *orderbook.RBTree
	desc bool
}

// NewRBTreeWrapper 创建包装器
func NewRBTreeWrapper(tree *orderbook.RBTree, desc bool) *RBTreeWrapper {
	return &RBTreeWrapper{tree: tree, desc: desc}
}

// Best 获取最优价格档位
func (w *RBTreeWrapper) Best() *orderbook.PriceLevel {
	if w.desc {
		return w.tree.Max()
	}
	return w.tree.Min()
}
