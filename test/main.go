package main

import (
	"fmt"

	"github.com/hzx/matchengine/internal/matching"
	"github.com/hzx/matchengine/internal/models"
	"github.com/hzx/matchengine/internal/orderbook"
	"github.com/hzx/matchengine/pkg/utils"
	"github.com/shopspring/decimal"
)

func main() {
	fmt.Println("=== 撮合引擎测试 ===")
	fmt.Println()

	// 创建订单簿
	ob := orderbook.NewOrderBook("BTC_USDT")
	fmt.Println("✓ 订单簿创建成功")

	// 创建限价单撮合器
	limitMatcher := matching.NewLimitMatcher()
	fmt.Println("✓ 限价单撮合器创建成功")

	// ID生成器
	idGen := utils.NewSnowflake(1)

	// 创建卖单1: 50000 USDT, 1 BTC
	sellOrder1 := models.NewOrder(
		idGen.Generate(),
		1001, // 用户ID
		"BTC_USDT",
		models.OrderSideSell,
		models.OrderTypeLimit,
		decimal.RequireFromString("50000"),
		decimal.RequireFromString("1"),
		models.TimeInForceGTC,
	)
	fmt.Printf("✓ 创建卖单1: ID=%d, 价格=50000, 数量=1\n", sellOrder1.ID)

	// 添加卖单到订单簿
	ob.AddOrder(sellOrder1)
	fmt.Printf("  订单簿: 卖单数量=%d\n", ob.GetAskCount())

	// 创建卖单2: 51000 USDT, 0.5 BTC
	sellOrder2 := models.NewOrder(
		idGen.Generate(),
		1002,
		"BTC_USDT",
		models.OrderSideSell,
		models.OrderTypeLimit,
		decimal.RequireFromString("51000"),
		decimal.RequireFromString("0.5"),
		models.TimeInForceGTC,
	)
	fmt.Printf("✓ 创建卖单2: ID=%d, 价格=51000, 数量=0.5\n", sellOrder2.ID)
	ob.AddOrder(sellOrder2)

	// 创建买单: 50500 USDT, 1.2 BTC (会吃掉卖单1)
	buyOrder := models.NewOrder(
		idGen.Generate(),
		2001,
		"BTC_USDT",
		models.OrderSideBuy,
		models.OrderTypeLimit,
		decimal.RequireFromString("50500"),
		decimal.RequireFromString("1.2"),
		models.TimeInForceGTC,
	)
	fmt.Printf("✓ 创建买单: ID=%d, 价格=50500, 数量=1.2\n", buyOrder.ID)

	fmt.Println()
	fmt.Println("=== 开始撮合 ===")

	// 撮合
	result := limitMatcher.Match(buyOrder, ob, func() uint64 { return idGen.Generate() })

	fmt.Println()
	fmt.Println("=== 撮合结果 ===")
	if result.HasTrade() {
		fmt.Printf("✓ 成交笔数: %d\n", len(result.Trades))
		for i, trade := range result.Trades {
			fmt.Printf("  成交%d: 价格=%s, 数量=%s, 总额=%s\n",
				i+1,
				trade.Price.String(),
				trade.Amount.String(),
				trade.Total.String())
		}

		fmt.Printf("\n买单状态:\n")
		fmt.Printf("  原始数量: %s\n", buyOrder.Amount.String())
		fmt.Printf("  已成交: %s\n", buyOrder.FilledAmount.String())
		fmt.Printf("  剩余: %s\n", buyOrder.RemainAmount.String())
		fmt.Printf("  状态: %s\n", buyOrder.Status.String())

		// 未成交部分加入订单簿
		if buyOrder.RemainAmount.GreaterThan(decimal.Zero) {
			ob.AddOrder(buyOrder)
			fmt.Printf("\n✓ 买单未成交部分(%s)加入订单簿\n", buyOrder.RemainAmount.String())
		}
	} else {
		fmt.Println("✗ 没有成交")
	}

	// 显示盘口
	fmt.Println()
	fmt.Println("=== 盘口深度 ===")
	depth := ob.GetDepth(5)
	fmt.Println("买单:")
	for _, bid := range depth.Bids {
		fmt.Printf("  价格=%s, 数量=%s\n", bid.Price.String(), bid.Amount.String())
	}
	fmt.Println("卖单:")
	for _, ask := range depth.Asks {
		fmt.Printf("  价格=%s, 数量=%s\n", ask.Price.String(), ask.Amount.String())
	}

	// 测试取消订单
	fmt.Println()
	fmt.Println("=== 测试取消订单 ===")
	cancelled := ob.RemoveOrder(sellOrder2.ID)
	if cancelled != nil {
		fmt.Printf("✓ 成功取消订单 ID=%d\n", cancelled.ID)
		fmt.Printf("  订单簿: 卖单数量=%d\n", ob.GetAskCount())
	}

	fmt.Println()
	fmt.Println("=== 测试完成 ===")
}
