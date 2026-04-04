package server

import (
	"context"

	"github.com/hzx/matchengine/internal/engine"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// RegisterGRPCServices 注册gRPC服务
func RegisterGRPCServices(server *grpc.Server, matchingEngine *engine.Engine, logger *zap.Logger) {
	// TODO: 实现gRPC服务
	// 这里需要定义proto文件并生成代码
	// pb.RegisterMatchEngineServer(server, NewMatchEngineServer(matchingEngine, logger))
}

// MatchEngineServer 撮合引擎gRPC服务器
type MatchEngineServer struct {
	// pb.UnimplementedMatchEngineServer
	engine *engine.Engine
	logger *zap.Logger
}

// NewMatchEngineServer 创建gRPC服务器
func NewMatchEngineServer(matchingEngine *engine.Engine, logger *zap.Logger) *MatchEngineServer {
	return &MatchEngineServer{
		engine: matchingEngine,
		logger: logger,
	}
}

// SubmitOrder 提交订单
func (s *MatchEngineServer) SubmitOrder(ctx context.Context, req *SubmitOrderRequest) (*SubmitOrderResponse, error) {
	// TODO: 实现订单提交
	return &SubmitOrderResponse{
		Success: true,
	}, nil
}

// CancelOrder 取消订单
func (s *MatchEngineServer) CancelOrder(ctx context.Context, req *CancelOrderRequest) (*CancelOrderResponse, error) {
	// TODO: 实现订单取消
	return &CancelOrderResponse{
		Success: true,
	}, nil
}

// GetOrder 获取订单
func (s *MatchEngineServer) GetOrder(ctx context.Context, req *GetOrderRequest) (*GetOrderResponse, error) {
	// TODO: 实现订单查询
	return &GetOrderResponse{
		Success: true,
	}, nil
}

// GetDepth 获取盘口深度
func (s *MatchEngineServer) GetDepth(ctx context.Context, req *GetDepthRequest) (*GetDepthResponse, error) {
	// TODO: 实现盘口查询
	return &GetDepthResponse{
		Success: true,
	}, nil
}

// 以下是占位类型，实际需要从proto生成

type SubmitOrderRequest struct{}
type SubmitOrderResponse struct {
	Success bool
}
type CancelOrderRequest struct{}
type CancelOrderResponse struct {
	Success bool
}
type GetOrderRequest struct{}
type GetOrderResponse struct {
	Success bool
}
type GetDepthRequest struct{}
type GetDepthResponse struct {
	Success bool
}
