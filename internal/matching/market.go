package matching

import (
	"github.com/hzx/matchengine/internal/models"
	"github.com/hzx/matchengine/internal/orderbook"
	"github.com/shopspring/decimal"
)

// MarketMatcher 市价单撮合器
type MarketMatcher struct{}

// NewMarketMatcher 创建市价单撮合器
func NewMarketMatcher() *MarketMatcher {
	return &MarketMatcher{}
}

// Match 撮合市价单
func (m *MarketMatcher) Match(takerOrder *models.Order, ob *orderbook.OrderBook, tradeIDGen func() uint64) *models.TradeResult {
	result := models.NewTradeResult()
	result.TakerOrder = takerOrder

	// 市价单立即撮合，直到数量为0或对手盘为空
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
	}

	return result
}

// matchAtPriceLevel 在价格档位撮合，返回价格档位是否为空
func (m *MarketMatcher) matchAtPriceLevel(takerOrder *models.Order, priceLevel *orderbook.PriceLevel,
	result *models.TradeResult, tradeIDGen func() uint64) bool {

	for takerOrder.RemainAmount.GreaterThan(decimal.Zero) {
		makerOrder := priceLevel.GetFirst()
		if makerOrder == nil {
			return true
		}

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

		if makerOrder.IsFinished() {
			priceLevel.RemoveOrder(makerOrder.ID)
		}
	}

	return priceLevel.IsEmpty()
}
