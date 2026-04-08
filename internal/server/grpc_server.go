package server

import (
	"context"

	"github.com/hzx/matchengine/internal/engine"
	"github.com/hzx/matchengine/internal/models"
	"github.com/hzx/matchengine/proto"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// RegisterGRPCServices 注册gRPC服务
func RegisterGRPCServices(server *grpc.Server, matchingEngine *engine.Engine, db OrderQuerier, logger *zap.Logger) {
	proto.RegisterMatchEngineServer(server, NewMatchEngineServer(matchingEngine, db, logger))
}

// MatchEngineServer 撮合引擎gRPC服务器
type MatchEngineServer struct {
	proto.UnimplementedMatchEngineServer
	engine *engine.Engine
	db     OrderQuerier
	logger *zap.Logger
}

// NewMatchEngineServer 创建gRPC服务器
func NewMatchEngineServer(matchingEngine *engine.Engine, db OrderQuerier, logger *zap.Logger) *MatchEngineServer {
	return &MatchEngineServer{
		engine: matchingEngine,
		db:     db,
		logger: logger,
	}
}

// SubmitOrder 提交订单
func (s *MatchEngineServer) SubmitOrder(ctx context.Context, req *proto.SubmitOrderRequest) (*proto.SubmitOrderResponse, error) {
	// 解析价格和数量
	price, err := decimal.NewFromString(req.GetPrice())
	if err != nil {
		return &proto.SubmitOrderResponse{
			Success: false,
			Error:   "invalid price",
		}, nil
	}

	amount, err := decimal.NewFromString(req.GetAmount())
	if err != nil {
		return &proto.SubmitOrderResponse{
			Success: false,
			Error:   "invalid amount",
		}, nil
	}

	// 创建订单
	order := models.NewOrder(
		0,
		req.GetUserId(),
		req.GetSymbol(),
		models.OrderSide(req.GetSide()),
		models.OrderType(req.GetType()),
		price,
		amount,
		models.TimeInForce(req.GetTimeInForce()),
	)

	// 设置止损价格
	if req.GetStopPrice() != "" {
		stopPrice, err := decimal.NewFromString(req.GetStopPrice())
		if err == nil {
			order.StopPrice = stopPrice
		}
	}

	// 设置冰山单可见数量
	if req.GetVisibleAmount() != "" {
		visibleAmount, err := decimal.NewFromString(req.GetVisibleAmount())
		if err == nil {
			order.Visible = visibleAmount
		}
	}

	// 提交订单
	if err := s.engine.SubmitOrder(order); err != nil {
		return &proto.SubmitOrderResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &proto.SubmitOrderResponse{
		Success: true,
		Order:   convertOrderToProto(order),
	}, nil
}

// CancelOrder 取消订单
func (s *MatchEngineServer) CancelOrder(ctx context.Context, req *proto.CancelOrderRequest) (*proto.CancelOrderResponse, error) {
	order, err := s.engine.CancelOrder(req.GetSymbol(), req.GetOrderId())
	if err != nil {
		return &proto.CancelOrderResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &proto.CancelOrderResponse{
		Success: true,
		Order:   convertOrderToProto(order),
	}, nil
}

// GetOrder 获取订单
func (s *MatchEngineServer) GetOrder(ctx context.Context, req *proto.GetOrderRequest) (*proto.GetOrderResponse, error) {
	order, err := s.engine.GetOrder(req.GetSymbol(), req.GetOrderId())
	if err != nil {
		return &proto.GetOrderResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &proto.GetOrderResponse{
		Success: true,
		Order:   convertOrderToProto(order),
	}, nil
}

// GetDepth 获取盘口深度
func (s *MatchEngineServer) GetDepth(ctx context.Context, req *proto.GetDepthRequest) (*proto.GetDepthResponse, error) {
	limit := int(req.GetLimit())
	if limit == 0 {
		limit = 20
	}

	depth, err := s.engine.GetDepth(req.GetSymbol(), limit)
	if err != nil {
		return &proto.GetDepthResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	bids := make([]*proto.DepthLevel, len(depth.Bids))
	for i, level := range depth.Bids {
		bids[i] = &proto.DepthLevel{
			Price:  level.Price.String(),
			Amount: level.Amount.String(),
		}
	}

	asks := make([]*proto.DepthLevel, len(depth.Asks))
	for i, level := range depth.Asks {
		asks[i] = &proto.DepthLevel{
			Price:  level.Price.String(),
			Amount: level.Amount.String(),
		}
	}

	return &proto.GetDepthResponse{
		Success: true,
		Symbol:  depth.Symbol,
		Bids:    bids,
		Asks:    asks,
	}, nil
}

// GetTicker 获取行情
func (s *MatchEngineServer) GetTicker(ctx context.Context, req *proto.GetTickerRequest) (*proto.GetTickerResponse, error) {
	ticker, err := s.engine.GetTicker(req.GetSymbol())
	if err != nil {
		return &proto.GetTickerResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &proto.GetTickerResponse{
		Success: true,
		Ticker: &proto.Ticker{
			Symbol:    ticker.Symbol,
			LastPrice: ticker.LastPrice.String(),
			BidPrice:  ticker.BidPrice.String(),
			BidAmount: ticker.BidAmount.String(),
			AskPrice:  ticker.AskPrice.String(),
			AskAmount: ticker.AskAmount.String(),
			Timestamp: ticker.Timestamp,
		},
	}, nil
}

// GetTrades 获取成交记录
func (s *MatchEngineServer) GetTrades(ctx context.Context, req *proto.GetTradesRequest) (*proto.GetTradesResponse, error) {
	limit := int(req.GetLimit())
	if limit == 0 {
		limit = 100
	}
	offset := int(req.GetOffset())

	trades, err := s.db.GetTrades(req.GetSymbol(), limit, offset)
	if err != nil {
		return &proto.GetTradesResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	protoTrades := make([]*proto.Trade, len(trades))
	for i, trade := range trades {
		protoTrades[i] = convertTradeToProto(trade)
	}

	return &proto.GetTradesResponse{
		Success: true,
		Trades:  protoTrades,
	}, nil
}

// convertOrderToProto 将内部订单转换为proto订单
func convertOrderToProto(order *models.Order) *proto.Order {
	if order == nil {
		return nil
	}

	return &proto.Order{
		Id:            order.ID,
		UserId:        order.UserID,
		Symbol:        order.Symbol,
		Side:          proto.OrderSide(order.Side),
		Type:          proto.OrderType(order.Type),
		Status:        proto.OrderStatus(order.Status),
		Price:         order.Price.String(),
		Amount:        order.Amount.String(),
		FilledAmount:  order.FilledAmount.String(),
		RemainAmount:  order.RemainAmount.String(),
		FilledTotal:   order.FilledTotal.String(),
		Fee:           order.Fee.String(),
		FeeAsset:      order.FeeAsset,
		TimeInForce:   proto.TimeInForce(order.TimeInForce),
		StopPrice:     order.StopPrice.String(),
		VisibleAmount: order.Visible.String(),
		CreatedAt:     order.CreatedAt.Unix(),
		UpdatedAt:     order.UpdatedAt.Unix(),
	}
}

// convertTradeToProto 将内部成交转换为proto成交
func convertTradeToProto(trade *models.Trade) *proto.Trade {
	if trade == nil {
		return nil
	}

	return &proto.Trade{
		Id:           trade.ID,
		TakerOrderId: trade.TakerOrderID,
		MakerOrderId: trade.MakerOrderID,
		UserId:       trade.UserID,
		MakerUserId:  trade.MakerUserID,
		Symbol:       trade.Symbol,
		Price:        trade.Price.String(),
		Amount:       trade.Amount.String(),
		Total:        trade.Total.String(),
		Fee:          trade.Fee.String(),
		FeeAsset:     trade.FeeAsset,
		Side:         proto.OrderSide(trade.Side),
		CreatedAt:    trade.CreatedAt.Unix(),
	}
}
