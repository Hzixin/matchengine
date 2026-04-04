package matching

import (
	"testing"

	"github.com/hzx/matchengine/internal/models"
	"github.com/hzx/matchengine/internal/orderbook"
	"github.com/hzx/matchengine/pkg/utils"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// 测试FOK订单
func TestLimitMatcher_FOK(t *testing.T) {
	ob := orderbook.NewOrderBook("BTC_USDT")
	matcher := NewLimitMatcher()
	idGen := utils.NewSnowflake(1)

	// 添加卖单
	sellOrder := models.NewOrder(
		idGen.Generate(), 1, "BTC_USDT",
		models.OrderSideSell, models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("0.5"),
		models.TimeInForceGTC,
	)
	ob.AddOrder(sellOrder)

	// FOK买单 - 数量大于可用
	fokOrder := models.NewOrder(
		idGen.Generate(), 2, "BTC_USDT",
		models.OrderSideBuy, models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("1"),
		models.TimeInForceFOK,
	)

	result := matcher.Match(fokOrder, ob, func() uint64 { return idGen.Generate() })

	// FOK应该不成交
	assert.False(t, result.HasTrade())
	assert.True(t, fokOrder.FilledAmount.IsZero())

	// FOK买单 - 数量刚好
	fokOrder2 := models.NewOrder(
		idGen.Generate(), 3, "BTC_USDT",
		models.OrderSideBuy, models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("0.5"),
		models.TimeInForceFOK,
	)

	result2 := matcher.Match(fokOrder2, ob, func() uint64 { return idGen.Generate() })
	assert.True(t, result2.HasTrade())
	assert.True(t, fokOrder2.FilledAmount.Equal(decimal.RequireFromString("0.5")))
}

// 测试IOC订单
func TestLimitMatcher_IOC(t *testing.T) {
	ob := orderbook.NewOrderBook("BTC_USDT")
	matcher := NewLimitMatcher()
	idGen := utils.NewSnowflake(1)

	// 添加卖单
	sellOrder := models.NewOrder(
		idGen.Generate(), 1, "BTC_USDT",
		models.OrderSideSell, models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("0.5"),
		models.TimeInForceGTC,
	)
	ob.AddOrder(sellOrder)

	// IOC买单 - 部分成交后取消
	iocOrder := models.NewOrder(
		idGen.Generate(), 2, "BTC_USDT",
		models.OrderSideBuy, models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("1"),
		models.TimeInForceIOC,
	)

	result := matcher.Match(iocOrder, ob, func() uint64 { return idGen.Generate() })

	assert.True(t, result.HasTrade())
	assert.True(t, iocOrder.FilledAmount.Equal(decimal.RequireFromString("0.5")))
	assert.True(t, iocOrder.RemainAmount.Equal(decimal.RequireFromString("0.5")))
}

// 测试空订单簿
func TestLimitMatcher_EmptyOrderBook(t *testing.T) {
	ob := orderbook.NewOrderBook("BTC_USDT")
	matcher := NewLimitMatcher()
	idGen := utils.NewSnowflake(1)

	// 买单匹配空订单簿
	buyOrder := models.NewOrder(
		idGen.Generate(), 1, "BTC_USDT",
		models.OrderSideBuy, models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("1"),
		models.TimeInForceGTC,
	)

	result := matcher.Match(buyOrder, ob, func() uint64 { return idGen.Generate() })
	assert.False(t, result.HasTrade())

	// 卖单匹配空订单簿
	sellOrder := models.NewOrder(
		idGen.Generate(), 2, "BTC_USDT",
		models.OrderSideSell, models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("1"),
		models.TimeInForceGTC,
	)

	result2 := matcher.Match(sellOrder, ob, func() uint64 { return idGen.Generate() })
	assert.False(t, result2.HasTrade())
}

// 测试价格不匹配
func TestLimitMatcher_PriceMismatch(t *testing.T) {
	ob := orderbook.NewOrderBook("BTC_USDT")
	matcher := NewLimitMatcher()
	idGen := utils.NewSnowflake(1)

	// 添加卖单 50000
	sellOrder := models.NewOrder(
		idGen.Generate(), 1, "BTC_USDT",
		models.OrderSideSell, models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("1"),
		models.TimeInForceGTC,
	)
	ob.AddOrder(sellOrder)

	// 买单价格太低 49000
	buyOrder := models.NewOrder(
		idGen.Generate(), 2, "BTC_USDT",
		models.OrderSideBuy, models.OrderTypeLimit,
		decimal.RequireFromString("49000"),
		decimal.RequireFromString("1"),
		models.TimeInForceGTC,
	)

	result := matcher.Match(buyOrder, ob, func() uint64 { return idGen.Generate() })
	assert.False(t, result.HasTrade())
}

// 测试多个价格档位撮合
func TestLimitMatcher_MultiplePriceLevels(t *testing.T) {
	ob := orderbook.NewOrderBook("BTC_USDT")
	matcher := NewLimitMatcher()
	idGen := utils.NewSnowflake(1)

	// 添加多个卖单，不同价格
	for i := 0; i < 3; i++ {
		price := decimal.RequireFromString("50000").Add(decimal.NewFromInt(int64(i * 1000)))
		order := models.NewOrder(
			idGen.Generate(), 1, "BTC_USDT",
			models.OrderSideSell, models.OrderTypeLimit,
			price,
			decimal.RequireFromString("1"),
			models.TimeInForceGTC,
		)
		ob.AddOrder(order)
	}

	// 大额买单，吃掉多个档位
	buyOrder := models.NewOrder(
		idGen.Generate(), 2, "BTC_USDT",
		models.OrderSideBuy, models.OrderTypeLimit,
		decimal.RequireFromString("52000"),
		decimal.RequireFromString("2.5"),
		models.TimeInForceGTC,
	)

	result := matcher.Match(buyOrder, ob, func() uint64 { return idGen.Generate() })

	assert.True(t, result.HasTrade())
	assert.Len(t, result.Trades, 3)
	assert.True(t, buyOrder.FilledAmount.Equal(decimal.RequireFromString("2.5")))

	// 验证成交价格顺序
	assert.True(t, result.Trades[0].Price.Equal(decimal.RequireFromString("50000")))
	assert.True(t, result.Trades[1].Price.Equal(decimal.RequireFromString("51000")))
	assert.True(t, result.Trades[2].Price.Equal(decimal.RequireFromString("52000")))
}

// 测试极小数量
func TestLimitMatcher_TinyAmount(t *testing.T) {
	ob := orderbook.NewOrderBook("BTC_USDT")
	matcher := NewLimitMatcher()
	idGen := utils.NewSnowflake(1)

	// 极小数量卖单
	sellOrder := models.NewOrder(
		idGen.Generate(), 1, "BTC_USDT",
		models.OrderSideSell, models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("0.00000001"),
		models.TimeInForceGTC,
	)
	ob.AddOrder(sellOrder)

	// 买单
	buyOrder := models.NewOrder(
		idGen.Generate(), 2, "BTC_USDT",
		models.OrderSideBuy, models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("0.00000001"),
		models.TimeInForceGTC,
	)

	result := matcher.Match(buyOrder, ob, func() uint64 { return idGen.Generate() })
	assert.True(t, result.HasTrade())
	assert.True(t, result.Trades[0].Amount.Equal(decimal.RequireFromString("0.00000001")))
}

// 测试买单卖单交叉撮合
func TestLimitMatcher_CrossMatching(t *testing.T) {
	ob := orderbook.NewOrderBook("BTC_USDT")
	matcher := NewLimitMatcher()
	idGen := utils.NewSnowflake(1)

	// 先添加买单
	buyOrder := models.NewOrder(
		idGen.Generate(), 1, "BTC_USDT",
		models.OrderSideBuy, models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("1"),
		models.TimeInForceGTC,
	)
	ob.AddOrder(buyOrder)

	// 添加卖单，价格更低（应该立即成交）
	sellOrder := models.NewOrder(
		idGen.Generate(), 2, "BTC_USDT",
		models.OrderSideSell, models.OrderTypeLimit,
		decimal.RequireFromString("49000"),
		decimal.RequireFromString("0.5"),
		models.TimeInForceGTC,
	)

	result := matcher.Match(sellOrder, ob, func() uint64 { return idGen.Generate() })

	// 应该以买一价成交（价格优先）
	assert.True(t, result.HasTrade())
	assert.True(t, result.Trades[0].Price.Equal(decimal.RequireFromString("50000")))
}

// 测试大量订单
func TestOrderBook_ManyOrders(t *testing.T) {
	ob := orderbook.NewOrderBook("BTC_USDT")

	// 添加1000个买单
	for i := 0; i < 1000; i++ {
		price := decimal.RequireFromString("50000").Sub(decimal.NewFromInt(int64(i)))
		order := models.NewOrder(
			uint64(i+1), 1, "BTC_USDT",
			models.OrderSideBuy, models.OrderTypeLimit,
			price,
			decimal.RequireFromString("1"),
			models.TimeInForceGTC,
		)
		ob.AddOrder(order)
	}

	assert.Equal(t, 1000, ob.GetOrderCount())
	assert.Equal(t, 1000, ob.GetBidCount())

	// 最优买价应该是50000
	bestBid := ob.GetBestBid()
	assert.NotNil(t, bestBid)
	assert.True(t, bestBid.Price.Equal(decimal.RequireFromString("50000")))
}

// 测试并发读写
func TestOrderBook_ConcurrentAccess(t *testing.T) {
	ob := orderbook.NewOrderBook("BTC_USDT")
	idGen := utils.NewSnowflake(1)

	// 并发添加订单
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func(idx int) {
			price := decimal.RequireFromString("50000").Add(decimal.NewFromInt(int64(idx)))
			order := models.NewOrder(
				idGen.Generate(), 1, "BTC_USDT",
				models.OrderSideBuy, models.OrderTypeLimit,
				price,
				decimal.RequireFromString("1"),
				models.TimeInForceGTC,
			)
			ob.AddOrder(order)
			done <- true
		}(i)
	}

	// 等待完成
	for i := 0; i < 100; i++ {
		<-done
	}

	assert.Equal(t, 100, ob.GetOrderCount())
}
