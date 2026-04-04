package matching

import (
	"testing"

	"github.com/hzx/matchengine/internal/models"
	"github.com/hzx/matchengine/internal/orderbook"
	"github.com/hzx/matchengine/pkg/utils"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestLimitMatcher_Match(t *testing.T) {
	ob := orderbook.NewOrderBook("BTC_USDT")
	matcher := NewLimitMatcher()
	idGen := utils.NewSnowflake(1)

	// 添加卖单
	sellOrder1 := models.NewOrder(
		idGen.Generate(), 1, "BTC_USDT",
		models.OrderSideSell, models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("1"),
		models.TimeInForceGTC,
	)
	ob.AddOrder(sellOrder1)

	sellOrder2 := models.NewOrder(
		idGen.Generate(), 2, "BTC_USDT",
		models.OrderSideSell, models.OrderTypeLimit,
		decimal.RequireFromString("51000"),
		decimal.RequireFromString("0.5"),
		models.TimeInForceGTC,
	)
	ob.AddOrder(sellOrder2)

	// 创建买单
	buyOrder := models.NewOrder(
		idGen.Generate(), 3, "BTC_USDT",
		models.OrderSideBuy, models.OrderTypeLimit,
		decimal.RequireFromString("50500"),
		decimal.RequireFromString("1.2"),
		models.TimeInForceGTC,
	)

	// 撮合
	result := matcher.Match(buyOrder, ob, func() uint64 { return idGen.Generate() })

	// 验证结果
	assert.True(t, result.HasTrade())
	assert.Len(t, result.Trades, 1)
	assert.True(t, result.Trades[0].Price.Equal(decimal.RequireFromString("50000")))
	assert.True(t, result.Trades[0].Amount.Equal(decimal.RequireFromString("1")))
	assert.True(t, buyOrder.FilledAmount.Equal(decimal.RequireFromString("1")))
	assert.True(t, buyOrder.RemainAmount.Equal(decimal.RequireFromString("0.2")))
	assert.Equal(t, models.OrderStatusPartiallyFilled, buyOrder.Status)
}

func TestLimitMatcher_FullMatch(t *testing.T) {
	ob := orderbook.NewOrderBook("BTC_USDT")
	matcher := NewLimitMatcher()
	idGen := utils.NewSnowflake(1)

	// 添加卖单
	sellOrder := models.NewOrder(
		idGen.Generate(), 1, "BTC_USDT",
		models.OrderSideSell, models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("1"),
		models.TimeInForceGTC,
	)
	ob.AddOrder(sellOrder)

	// 创建买单（完全成交）
	buyOrder := models.NewOrder(
		idGen.Generate(), 2, "BTC_USDT",
		models.OrderSideBuy, models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("1"),
		models.TimeInForceGTC,
	)

	result := matcher.Match(buyOrder, ob, func() uint64 { return idGen.Generate() })

	assert.True(t, result.HasTrade())
	assert.True(t, buyOrder.RemainAmount.IsZero())
	assert.Equal(t, models.OrderStatusFilled, buyOrder.Status)
}

func TestLimitMatcher_NoMatch(t *testing.T) {
	ob := orderbook.NewOrderBook("BTC_USDT")
	matcher := NewLimitMatcher()
	idGen := utils.NewSnowflake(1)

	// 添加卖单（价格高于买价）
	sellOrder := models.NewOrder(
		idGen.Generate(), 1, "BTC_USDT",
		models.OrderSideSell, models.OrderTypeLimit,
		decimal.RequireFromString("52000"),
		decimal.RequireFromString("1"),
		models.TimeInForceGTC,
	)
	ob.AddOrder(sellOrder)

	// 创建买单
	buyOrder := models.NewOrder(
		idGen.Generate(), 2, "BTC_USDT",
		models.OrderSideBuy, models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("1"),
		models.TimeInForceGTC,
	)

	result := matcher.Match(buyOrder, ob, func() uint64 { return idGen.Generate() })

	assert.False(t, result.HasTrade())
	assert.True(t, buyOrder.FilledAmount.IsZero())
	assert.Equal(t, models.OrderStatusNew, buyOrder.Status)
}

func TestMarketMatcher_Match(t *testing.T) {
	ob := orderbook.NewOrderBook("BTC_USDT")
	matcher := NewMarketMatcher()
	idGen := utils.NewSnowflake(1)

	// 添加多个卖单
	for i := 0; i < 3; i++ {
		price := decimal.RequireFromString("50000").Add(decimal.NewFromInt(int64(i * 1000)))
		order := models.NewOrder(
			idGen.Generate(), 1, "BTC_USDT",
			models.OrderSideSell, models.OrderTypeLimit,
			price,
			decimal.RequireFromString("0.5"),
			models.TimeInForceGTC,
		)
		ob.AddOrder(order)
	}

	// 创建市价买单
	marketOrder := models.NewOrder(
		idGen.Generate(), 2, "BTC_USDT",
		models.OrderSideBuy, models.OrderTypeMarket,
		decimal.Zero, // 市价单价格为0
		decimal.RequireFromString("1"),
		models.TimeInForceGTC,
	)

	result := matcher.Match(marketOrder, ob, func() uint64 { return idGen.Generate() })

	assert.True(t, result.HasTrade())
	// 应该吃掉两个卖单
	assert.Len(t, result.Trades, 2)
	assert.True(t, marketOrder.FilledAmount.Equal(decimal.RequireFromString("1")))
}

func TestPriceTimePriority(t *testing.T) {
	ob := orderbook.NewOrderBook("BTC_USDT")
	matcher := NewLimitMatcher()
	idGen := utils.NewSnowflake(1)

	// 添加两个相同价格的卖单
	sellOrder1 := models.NewOrder(
		idGen.Generate(), 1, "BTC_USDT",
		models.OrderSideSell, models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("0.5"),
		models.TimeInForceGTC,
	)
	ob.AddOrder(sellOrder1)

	sellOrder2 := models.NewOrder(
		idGen.Generate(), 2, "BTC_USDT",
		models.OrderSideSell, models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("0.5"),
		models.TimeInForceGTC,
	)
	ob.AddOrder(sellOrder2)

	// 创建买单
	buyOrder := models.NewOrder(
		idGen.Generate(), 3, "BTC_USDT",
		models.OrderSideBuy, models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("0.7"),
		models.TimeInForceGTC,
	)

	result := matcher.Match(buyOrder, ob, func() uint64 { return idGen.Generate() })

	// 验证先成交第一个卖单（时间优先）
	assert.Len(t, result.Trades, 2)
	assert.Equal(t, sellOrder1.ID, result.Trades[0].MakerOrderID)
	assert.True(t, result.Trades[0].Amount.Equal(decimal.RequireFromString("0.5")))
	assert.Equal(t, sellOrder2.ID, result.Trades[1].MakerOrderID)
	assert.True(t, result.Trades[1].Amount.Equal(decimal.RequireFromString("0.2")))
}
