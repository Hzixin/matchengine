package orderbook

import (
	"testing"

	"github.com/hzx/matchengine/internal/models"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestOrderBook_AddOrder(t *testing.T) {
	ob := NewOrderBook("BTC_USDT")

	// 创建买单
	buyOrder := models.NewOrder(
		1, 1, "BTC_USDT",
		models.OrderSideBuy,
		models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("1"),
		models.TimeInForceGTC,
	)

	ob.AddOrder(buyOrder)

	assert.Equal(t, 1, ob.GetOrderCount())
	assert.Equal(t, 1, ob.GetBidCount())

	// 获取最优买价
	bestBid := ob.GetBestBid()
	assert.NotNil(t, bestBid)
	assert.True(t, bestBid.Price.Equal(decimal.RequireFromString("50000")))
}

func TestOrderBook_RemoveOrder(t *testing.T) {
	ob := NewOrderBook("BTC_USDT")

	order := models.NewOrder(
		1, 1, "BTC_USDT",
		models.OrderSideSell,
		models.OrderTypeLimit,
		decimal.RequireFromString("51000"),
		decimal.RequireFromString("1"),
		models.TimeInForceGTC,
	)

	ob.AddOrder(order)
	assert.Equal(t, 1, ob.GetOrderCount())

	// 删除订单
	removed := ob.RemoveOrder(1)
	assert.NotNil(t, removed)
	assert.Equal(t, 0, ob.GetOrderCount())

	// 删除不存在的订单
	removed2 := ob.RemoveOrder(999)
	assert.Nil(t, removed2)
}

func TestOrderBook_GetDepth(t *testing.T) {
	ob := NewOrderBook("BTC_USDT")

	// 添加多个买单
	for i := 0; i < 3; i++ {
		price := decimal.RequireFromString("50000").Add(decimal.NewFromInt(int64(i * 100)))
		order := models.NewOrder(
			uint64(i+1), 1, "BTC_USDT",
			models.OrderSideBuy,
			models.OrderTypeLimit,
			price,
			decimal.RequireFromString("1"),
			models.TimeInForceGTC,
		)
		ob.AddOrder(order)
	}

	// 添加多个卖单
	for i := 0; i < 3; i++ {
		price := decimal.RequireFromString("51000").Add(decimal.NewFromInt(int64(i * 100)))
		order := models.NewOrder(
			uint64(i+10), 1, "BTC_USDT",
			models.OrderSideSell,
			models.OrderTypeLimit,
			price,
			decimal.RequireFromString("1"),
			models.TimeInForceGTC,
		)
		ob.AddOrder(order)
	}

	depth := ob.GetDepth(5)
	assert.Len(t, depth.Bids, 3)
	assert.Len(t, depth.Asks, 3)

	// 买一价应该是最高价
	assert.True(t, depth.Bids[0].Price.Equal(decimal.RequireFromString("50200")))
	// 卖一价应该是最低价
	assert.True(t, depth.Asks[0].Price.Equal(decimal.RequireFromString("51000")))
}

func TestRBTree(t *testing.T) {
	tree := NewRBTree(false) // 升序

	// 插入节点
	tree.Insert(decimal.RequireFromString("50000"), NewPriceLevel(decimal.RequireFromString("50000")))
	tree.Insert(decimal.RequireFromString("49000"), NewPriceLevel(decimal.RequireFromString("49000")))
	tree.Insert(decimal.RequireFromString("51000"), NewPriceLevel(decimal.RequireFromString("51000")))

	assert.Equal(t, 3, tree.Size())

	// 获取最小值
	min := tree.Min()
	assert.NotNil(t, min)
	assert.True(t, min.Price.Equal(decimal.RequireFromString("49000")))

	// 获取最大值
	max := tree.Max()
	assert.NotNil(t, max)
	assert.True(t, max.Price.Equal(decimal.RequireFromString("51000")))

	// 获取节点
	level := tree.Get(decimal.RequireFromString("50000"))
	assert.NotNil(t, level)

	// 删除节点
	tree.Delete(decimal.RequireFromString("50000"))
	assert.Equal(t, 2, tree.Size())

	level = tree.Get(decimal.RequireFromString("50000"))
	assert.Nil(t, level)
}

func TestPriceLevel(t *testing.T) {
	pl := NewPriceLevel(decimal.RequireFromString("50000"))

	order1 := models.NewOrder(
		1, 1, "BTC_USDT",
		models.OrderSideBuy,
		models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("1.5"),
		models.TimeInForceGTC,
	)

	order2 := models.NewOrder(
		2, 1, "BTC_USDT",
		models.OrderSideBuy,
		models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("0.5"),
		models.TimeInForceGTC,
	)

	pl.AddOrder(order1)
	pl.AddOrder(order2)

	assert.Equal(t, 2, pl.Orders.Len())
	assert.True(t, pl.Total.Equal(decimal.RequireFromString("2")))

	// 获取第一个订单
	first := pl.GetFirst()
	assert.NotNil(t, first)
	assert.Equal(t, uint64(1), first.ID)

	// 移除订单
	removed := pl.RemoveOrder(1)
	assert.NotNil(t, removed)
	assert.Equal(t, 1, pl.Orders.Len())
	assert.True(t, pl.Total.Equal(decimal.RequireFromString("0.5")))
}
