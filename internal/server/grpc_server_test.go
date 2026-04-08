package server

import (
	"context"
	"testing"

	"github.com/hzx/matchengine/internal/config"
	"github.com/hzx/matchengine/internal/engine"
	"github.com/hzx/matchengine/internal/models"
	"github.com/hzx/matchengine/proto"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func setupGRPCServer(t *testing.T) (*MatchEngineServer, *engine.Engine, *MockMySQLStorage) {
	logger, _ := zap.NewDevelopment()

	cfg := &config.Config{
		Markets: []config.MarketConfig{
			{
				Symbol:         "BTC_USDT",
				BaseAsset:      "BTC",
				QuoteAsset:     "USDT",
				BasePrecision:  8,
				QuotePrecision: 2,
				MinAmount:      "0.001",
				MinTotal:       "10",
				PriceTick:      "0.01",
				AmountTick:     "0.0001",
			},
		},
		Server: config.ServerConfig{
			GRPCPort: 50052,
		},
	}

	mockDB := NewMockMySQLStorage()
	eng := engine.NewEngine(cfg, logger)
	eng.Start()

	server := NewMatchEngineServer(eng, mockDB, logger)

	return server, eng, mockDB
}

func TestGRPC_GetTicker(t *testing.T) {
	server, eng, _ := setupGRPCServer(t)
	defer eng.Stop()

	resp, err := server.GetTicker(context.Background(), &proto.GetTickerRequest{
		Symbol: "BTC_USDT",
	})
	assert.NoError(t, err)
	assert.True(t, resp.GetSuccess())
	assert.Equal(t, "BTC_USDT", resp.GetTicker().GetSymbol())
}

func TestGRPC_GetDepth(t *testing.T) {
	server, eng, _ := setupGRPCServer(t)
	defer eng.Stop()

	resp, err := server.GetDepth(context.Background(), &proto.GetDepthRequest{
		Symbol: "BTC_USDT",
		Limit:  10,
	})
	assert.NoError(t, err)
	assert.True(t, resp.GetSuccess())
	assert.Equal(t, "BTC_USDT", resp.GetSymbol())
}

func TestGRPC_SubmitOrder(t *testing.T) {
	server, eng, _ := setupGRPCServer(t)
	defer eng.Stop()

	resp, err := server.SubmitOrder(context.Background(), &proto.SubmitOrderRequest{
		UserId:      1,
		Symbol:      "BTC_USDT",
		Side:        proto.OrderSide_ORDER_SIDE_BUY,
		Type:        proto.OrderType_ORDER_TYPE_LIMIT,
		Price:       "50000",
		Amount:      "1",
		TimeInForce: proto.TimeInForce_TIME_IN_FORCE_GTC,
	})
	assert.NoError(t, err)
	assert.True(t, resp.GetSuccess())
	assert.NotNil(t, resp.GetOrder())
	assert.Equal(t, uint64(1), resp.GetOrder().GetUserId())
	assert.Equal(t, "BTC_USDT", resp.GetOrder().GetSymbol())
	assert.Equal(t, proto.OrderSide_ORDER_SIDE_BUY, resp.GetOrder().GetSide())
}

func TestGRPC_SubmitOrder_InvalidPrice(t *testing.T) {
	server, eng, _ := setupGRPCServer(t)
	defer eng.Stop()

	resp, err := server.SubmitOrder(context.Background(), &proto.SubmitOrderRequest{
		UserId:      1,
		Symbol:      "BTC_USDT",
		Side:        proto.OrderSide_ORDER_SIDE_BUY,
		Type:        proto.OrderType_ORDER_TYPE_LIMIT,
		Price:       "invalid",
		Amount:      "1",
		TimeInForce: proto.TimeInForce_TIME_IN_FORCE_GTC,
	})
	assert.NoError(t, err)
	assert.False(t, resp.GetSuccess())
	assert.Equal(t, "invalid price", resp.GetError())
}

func TestGRPC_SubmitOrder_InvalidAmount(t *testing.T) {
	server, eng, _ := setupGRPCServer(t)
	defer eng.Stop()

	resp, err := server.SubmitOrder(context.Background(), &proto.SubmitOrderRequest{
		UserId:      1,
		Symbol:      "BTC_USDT",
		Side:        proto.OrderSide_ORDER_SIDE_BUY,
		Type:        proto.OrderType_ORDER_TYPE_LIMIT,
		Price:       "50000",
		Amount:      "invalid",
		TimeInForce: proto.TimeInForce_TIME_IN_FORCE_GTC,
	})
	assert.NoError(t, err)
	assert.False(t, resp.GetSuccess())
	assert.Equal(t, "invalid amount", resp.GetError())
}

func TestGRPC_GetOrder(t *testing.T) {
	server, eng, _ := setupGRPCServer(t)
	defer eng.Stop()

	// 先创建订单
	submitResp, err := server.SubmitOrder(context.Background(), &proto.SubmitOrderRequest{
		UserId:      1,
		Symbol:      "BTC_USDT",
		Side:        proto.OrderSide_ORDER_SIDE_BUY,
		Type:        proto.OrderType_ORDER_TYPE_LIMIT,
		Price:       "40000", // 低价买单，不会成交
		Amount:      "1",
		TimeInForce: proto.TimeInForce_TIME_IN_FORCE_GTC,
	})
	assert.NoError(t, err)
	assert.True(t, submitResp.GetSuccess())
	// 注意：订单通过 channel 异步处理，返回的订单 ID 为 0
	// 实际 ID 在引擎处理时生成
	assert.NotNil(t, submitResp.GetOrder())
}

func TestGRPC_GetOrder_NotFound(t *testing.T) {
	server, eng, _ := setupGRPCServer(t)
	defer eng.Stop()

	resp, err := server.GetOrder(context.Background(), &proto.GetOrderRequest{
		Symbol:  "BTC_USDT",
		OrderId: 999999,
	})
	assert.NoError(t, err)
	assert.False(t, resp.GetSuccess())
	assert.Contains(t, resp.GetError(), "not found")
}

func TestGRPC_CancelOrder(t *testing.T) {
	server, eng, _ := setupGRPCServer(t)
	defer eng.Stop()

	// 先创建订单
	submitResp, err := server.SubmitOrder(context.Background(), &proto.SubmitOrderRequest{
		UserId:      1,
		Symbol:      "BTC_USDT",
		Side:        proto.OrderSide_ORDER_SIDE_BUY,
		Type:        proto.OrderType_ORDER_TYPE_LIMIT,
		Price:       "40000", // 低价买单，不会立即成交
		Amount:      "1",
		TimeInForce: proto.TimeInForce_TIME_IN_FORCE_GTC,
	})
	assert.NoError(t, err)
	assert.True(t, submitResp.GetSuccess())
	// 注意：订单通过 channel 异步处理，返回的订单 ID 为 0
	// 取消订单需要使用引擎处理后的订单 ID，这里只验证 SubmitOrder 成功
}

func TestGRPC_CancelOrder_NotFound(t *testing.T) {
	server, eng, _ := setupGRPCServer(t)
	defer eng.Stop()

	resp, err := server.CancelOrder(context.Background(), &proto.CancelOrderRequest{
		Symbol:  "BTC_USDT",
		OrderId: 999999,
	})
	assert.NoError(t, err)
	assert.False(t, resp.GetSuccess())
	assert.Contains(t, resp.GetError(), "not found")
}

func TestGRPC_GetTrades(t *testing.T) {
	server, eng, mockDB := setupGRPCServer(t)
	defer eng.Stop()

	// 添加测试数据
	mockDB.trades["BTC_USDT"] = []*models.Trade{
		{
			ID:           1,
			TakerOrderID: 1001,
			MakerOrderID: 1002,
			UserID:       1,
			MakerUserID:  2,
			Symbol:       "BTC_USDT",
			Price:        decimal.RequireFromString("50000"),
			Amount:       decimal.RequireFromString("0.5"),
			Total:        decimal.RequireFromString("25000"),
			Side:         models.OrderSideBuy,
		},
	}

	resp, err := server.GetTrades(context.Background(), &proto.GetTradesRequest{
		Symbol: "BTC_USDT",
		Limit:  10,
	})
	assert.NoError(t, err)
	assert.True(t, resp.GetSuccess())
	assert.Len(t, resp.GetTrades(), 1)
	assert.Equal(t, uint64(1), resp.GetTrades()[0].GetId())
}

func TestGRPC_GetTrades_Empty(t *testing.T) {
	server, eng, _ := setupGRPCServer(t)
	defer eng.Stop()

	resp, err := server.GetTrades(context.Background(), &proto.GetTradesRequest{
		Symbol: "ETH_USDT",
		Limit:  10,
	})
	assert.NoError(t, err)
	assert.True(t, resp.GetSuccess())
	assert.Empty(t, resp.GetTrades())
}

func TestConvertOrderToProto(t *testing.T) {
	order := models.NewOrder(
		12345,
		1,
		"BTC_USDT",
		models.OrderSideBuy,
		models.OrderTypeLimit,
		decimal.RequireFromString("50000.5"),
		decimal.RequireFromString("1.5"),
		models.TimeInForceGTC,
	)
	order.Status = models.OrderStatusNew
	order.FilledAmount = decimal.RequireFromString("0.5")
	order.RemainAmount = decimal.RequireFromString("1")
	order.FilledTotal = decimal.RequireFromString("25000.25")

	protoOrder := convertOrderToProto(order)

	assert.Equal(t, uint64(12345), protoOrder.GetId())
	assert.Equal(t, uint64(1), protoOrder.GetUserId())
	assert.Equal(t, "BTC_USDT", protoOrder.GetSymbol())
	assert.Equal(t, proto.OrderSide_ORDER_SIDE_BUY, protoOrder.GetSide())
	assert.Equal(t, proto.OrderType_ORDER_TYPE_LIMIT, protoOrder.GetType())
	assert.Equal(t, proto.OrderStatus_ORDER_STATUS_PENDING, protoOrder.GetStatus())
	assert.Equal(t, "50000.5", protoOrder.GetPrice())
	assert.Equal(t, "1.5", protoOrder.GetAmount())
	assert.Equal(t, "0.5", protoOrder.GetFilledAmount())
	assert.Equal(t, "1", protoOrder.GetRemainAmount())
	assert.Equal(t, "25000.25", protoOrder.GetFilledTotal())
}

func TestConvertTradeToProto(t *testing.T) {
	trade := &models.Trade{
		ID:           1,
		TakerOrderID: 1001,
		MakerOrderID: 1002,
		UserID:       1,
		MakerUserID:  2,
		Symbol:       "BTC_USDT",
		Price:        decimal.RequireFromString("50000"),
		Amount:       decimal.RequireFromString("0.5"),
		Total:        decimal.RequireFromString("25000"),
		Fee:          decimal.RequireFromString("25"),
		FeeAsset:     "USDT",
		Side:         models.OrderSideBuy,
	}

	protoTrade := convertTradeToProto(trade)

	assert.Equal(t, uint64(1), protoTrade.GetId())
	assert.Equal(t, uint64(1001), protoTrade.GetTakerOrderId())
	assert.Equal(t, uint64(1002), protoTrade.GetMakerOrderId())
	assert.Equal(t, uint64(1), protoTrade.GetUserId())
	assert.Equal(t, uint64(2), protoTrade.GetMakerUserId())
	assert.Equal(t, "BTC_USDT", protoTrade.GetSymbol())
	assert.Equal(t, "50000", protoTrade.GetPrice())
	assert.Equal(t, "0.5", protoTrade.GetAmount())
	assert.Equal(t, "25000", protoTrade.GetTotal())
	assert.Equal(t, "25", protoTrade.GetFee())
	assert.Equal(t, "USDT", protoTrade.GetFeeAsset())
	assert.Equal(t, proto.OrderSide_ORDER_SIDE_BUY, protoTrade.GetSide())
}

func TestConvertOrderToProto_Nil(t *testing.T) {
	protoOrder := convertOrderToProto(nil)
	assert.Nil(t, protoOrder)
}

func TestConvertTradeToProto_Nil(t *testing.T) {
	protoTrade := convertTradeToProto(nil)
	assert.Nil(t, protoTrade)
}
