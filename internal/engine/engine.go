package engine

import (
	"errors"
	"sync"
	"time"

	"github.com/hzx/matchengine/internal/config"
	"github.com/hzx/matchengine/internal/matching"
	"github.com/hzx/matchengine/internal/models"
	"github.com/hzx/matchengine/internal/orderbook"
	"github.com/hzx/matchengine/internal/raft"
	"github.com/hzx/matchengine/pkg/utils"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

var (
	ErrEngineStopped  = errors.New("engine stopped")
	ErrMarketNotFound = errors.New("market not found")
	ErrInvalidOrder   = errors.New("invalid order")
	ErrOrderNotFound  = errors.New("order not found")
	ErrNotLeader      = errors.New("not leader")
)

// Engine 撮合引擎
type Engine struct {
	cfg     *config.Config
	logger  *zap.Logger

	// 市场
	markets map[string]*MarketEngine

	// ID生成器
	idGenerator *utils.Snowflake

	// Raft节点 (高可用)
	raftNode *raft.RaftNode

	// 是否启用Raft
	enableRaft bool

	// 事件处理器链
	eventHandler *EventHandlerChain

	// 状态
	running bool
	mu      sync.RWMutex
}

// MarketEngine 市场引擎
type MarketEngine struct {
	Market      *models.Market
	OrderBook   *orderbook.OrderBook
	StopManager *matching.StopOrderManager
	IcebergMgr  *matching.IcebergOrderManager

	// 撮合器
	limitMatcher   *matching.LimitMatcher
	marketMatcher  *matching.MarketMatcher
	stopMatcher    *matching.StopMatcher
	icebergMatcher *matching.IcebergMatcher

	// 订单通道 (单机模式使用)
	orderChan chan *models.Order

	// 最新价格
	lastPrice decimal.Decimal

	// ID生成器
	idGenerator *utils.Snowflake

	// 是否启用Raft
	enableRaft bool

	// 引擎引用 (用于Raft模式提交订单)
	engine *Engine

	// 事件处理器链
	eventHandler *EventHandlerChain
}

// NewEngine 创建撮合引擎
func NewEngine(cfg *config.Config, logger *zap.Logger, eventHandler *EventHandlerChain) *Engine {
	return &Engine{
		cfg:          cfg,
		logger:       logger,
		markets:      make(map[string]*MarketEngine),
		idGenerator:  utils.NewSnowflake(int64(cfg.Raft.NodeID)),
		enableRaft:   len(cfg.Raft.Peers) > 0,
		eventHandler: eventHandler,
	}
}

// Start 启动引擎
func (e *Engine) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 初始化市场
	for _, marketCfg := range e.cfg.Markets {
		minAmount, _ := decimal.NewFromString(marketCfg.MinAmount)
		minTotal, _ := decimal.NewFromString(marketCfg.MinTotal)
		priceTick, _ := decimal.NewFromString(marketCfg.PriceTick)
		amountTick, _ := decimal.NewFromString(marketCfg.AmountTick)

		market := models.NewMarket(
			marketCfg.Symbol,
			marketCfg.BaseAsset,
			marketCfg.QuoteAsset,
			marketCfg.BasePrecision,
			marketCfg.QuotePrecision,
			minAmount,
			minTotal,
			priceTick,
			amountTick,
		)

		me := &MarketEngine{
			Market:         market,
			OrderBook:      orderbook.NewOrderBook(marketCfg.Symbol),
			StopManager:    matching.NewStopOrderManager(),
			IcebergMgr:     matching.NewIcebergOrderManager(),
			limitMatcher:   matching.NewLimitMatcher(),
			marketMatcher:  matching.NewMarketMatcher(),
			stopMatcher:    matching.NewStopMatcher(),
			icebergMatcher: matching.NewIcebergMatcher(),
			lastPrice:      decimal.Zero,
			idGenerator:    e.idGenerator,
			enableRaft:     e.enableRaft,
			engine:         e,
			eventHandler:   e.eventHandler,
		}

		// 单机模式使用channel
		if !e.enableRaft {
			me.orderChan = make(chan *models.Order, 10000)
			go me.processOrders()
		}

		e.markets[marketCfg.Symbol] = me
	}

	// 如果启用Raft，初始化Raft节点
	if e.enableRaft {
		raftNode, err := raft.NewRaftNode(&e.cfg.Raft, e.logger, e)
		if err != nil {
			return err
		}
		e.raftNode = raftNode
		e.logger.Info("raft node started", zap.Uint64("node_id", e.cfg.Raft.NodeID))
	}

	e.running = true
	e.logger.Info("matching engine started",
		zap.Int("markets", len(e.markets)),
		zap.Bool("raft_enabled", e.enableRaft))
	return nil
}

// Stop 停止引擎
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.running = false

	// 单机模式关闭channel
	if !e.enableRaft {
		for _, me := range e.markets {
			close(me.orderChan)
		}
	}

	if e.raftNode != nil {
		e.raftNode.Shutdown()
	}

	e.logger.Info("matching engine stopped")
}

// SubmitOrder 提交订单
func (e *Engine) SubmitOrder(order *models.Order) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if !e.running {
		return ErrEngineStopped
	}

	me, exists := e.markets[order.Symbol]
	if !exists {
		return ErrMarketNotFound
	}

	// 验证订单
	if !me.Market.ValidateOrder(order) {
		return ErrInvalidOrder
	}

	// Raft模式：通过共识
	if e.enableRaft && e.raftNode != nil {
		// 只有Leader可以提交订单
		if !e.raftNode.IsLeader() {
			return ErrNotLeader
		}

		// 通过Raft共识，共识成功后会调用ApplyOrder
		if err := e.raftNode.ProposeOrder(order); err != nil {
			return err
		}
		return nil
	}

	// 单机模式：直接发送到channel
	me.orderChan <- order
	return nil
}

// ApplyOrder 实现raft.OrderApplier接口 - Raft共识成功后调用
func (e *Engine) ApplyOrder(order *models.Order) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	me, exists := e.markets[order.Symbol]
	if !exists {
		return ErrMarketNotFound
	}

	// 直接处理订单（不经过channel）
	me.processOrder(order)
	return nil
}

// GetAllOrders 实现raft.OrderApplier接口 - 获取所有订单（用于快照）
func (e *Engine) GetAllOrders() (map[string][]*models.Order, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make(map[string][]*models.Order)
	for symbol, me := range e.markets {
		orders := make([]*models.Order, 0)
		
		// 遍历订单簿获取所有订单
		me.OrderBook.AscendBids(func(price decimal.Decimal, level *orderbook.PriceLevel) bool {
			// 从价格档位获取订单
			for _, order := range level.GetOrders() {
				orders = append(orders, order)
			}
			return true
		})
		
		me.OrderBook.AscendAsks(func(price decimal.Decimal, level *orderbook.PriceLevel) bool {
			for _, order := range level.GetOrders() {
				orders = append(orders, order)
			}
			return true
		})
		
		result[symbol] = orders
	}
	return result, nil
}

// RestoreOrders 实现raft.OrderApplier接口 - 恢复订单（从快照）
func (e *Engine) RestoreOrders(orders map[string][]*models.Order) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 清空现有订单簿
	for _, me := range e.markets {
		me.OrderBook = orderbook.NewOrderBook(me.Market.Symbol)
	}

	// 恢复订单
	for symbol, orderList := range orders {
		me, exists := e.markets[symbol]
		if !exists {
			continue
		}

		for _, order := range orderList {
			me.OrderBook.AddOrder(order)
		}
	}

	return nil
}

// IsLeader 是否是Leader节点
func (e *Engine) IsLeader() bool {
	if !e.enableRaft || e.raftNode == nil {
		return true
	}
	return e.raftNode.IsLeader()
}

// CancelOrder 取消订单
func (e *Engine) CancelOrder(symbol string, orderID uint64) (*models.Order, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	me, exists := e.markets[symbol]
	if !exists {
		return nil, ErrMarketNotFound
	}

	order := me.OrderBook.RemoveOrder(orderID)
	if order == nil {
		return nil, ErrOrderNotFound
	}

	order.Status = models.OrderStatusCancelled
	order.UpdatedAt = time.Now()

	// 触发订单取消事件
	if e.eventHandler != nil {
		e.eventHandler.OnOrderUpdate(order)
	}

	return order, nil
}

// GetOrder 获取订单
func (e *Engine) GetOrder(symbol string, orderID uint64) (*models.Order, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	me, exists := e.markets[symbol]
	if !exists {
		return nil, ErrMarketNotFound
	}

	order := me.OrderBook.GetOrder(orderID)
	if order == nil {
		return nil, ErrOrderNotFound
	}

	return order, nil
}

// GetDepth 获取盘口深度
func (e *Engine) GetDepth(symbol string, limit int) (*models.Depth, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	me, exists := e.markets[symbol]
	if !exists {
		return nil, ErrMarketNotFound
	}

	return me.OrderBook.GetDepth(limit), nil
}

// GetTicker 获取行情
func (e *Engine) GetTicker(symbol string) (*models.Ticker, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	me, exists := e.markets[symbol]
	if !exists {
		return nil, ErrMarketNotFound
	}

	ticker := &models.Ticker{
		Symbol:    symbol,
		LastPrice: me.lastPrice,
		Timestamp: time.Now().UnixMilli(),
	}

	bestBid := me.OrderBook.GetBestBid()
	if bestBid != nil {
		ticker.BidPrice = bestBid.Price
		ticker.BidAmount = bestBid.Total
	}

	bestAsk := me.OrderBook.GetBestAsk()
	if bestAsk != nil {
		ticker.AskPrice = bestAsk.Price
		ticker.AskAmount = bestAsk.Total
	}

	return ticker, nil
}

// processOrders 处理订单 (单机模式)
func (me *MarketEngine) processOrders() {
	for order := range me.orderChan {
		me.processOrder(order)
	}
}

// processOrder 处理单个订单
func (me *MarketEngine) processOrder(order *models.Order) {
	// 生成订单ID
	if order.ID == 0 {
		order.ID = me.idGenerator.Generate()
	}

	var result *models.TradeResult

	// 根据订单类型选择撮合器
	switch order.Type {
	case models.OrderTypeLimit:
		result = me.limitMatcher.Match(order, me.OrderBook, me.idGenerator.Generate)
	case models.OrderTypeMarket:
		result = me.marketMatcher.Match(order, me.OrderBook, me.idGenerator.Generate)
	case models.OrderTypeStopLimit, models.OrderTypeStopMarket:
		me.StopManager.AddOrder(order)
		return
	case models.OrderTypeIceberg:
		me.IcebergMgr.Register(order, order.Visible)
		result = me.icebergMatcher.Match(order, me.OrderBook, me.idGenerator.Generate)
	default:
		order.Status = models.OrderStatusRejected
		return
	}

	// 处理成交
	if result.HasTrade() {
		me.processTrades(result)
	}

	// 未成交部分加入订单簿
	if order.RemainAmount.GreaterThan(decimal.Zero) && !order.IsFinished() {
		if order.Type == models.OrderTypeMarket {
			order.Status = models.OrderStatusCancelled
			// 发送订单取消事件
			if me.eventHandler != nil {
				me.eventHandler.OnOrderUpdate(order)
			}
		} else {
			me.OrderBook.AddOrder(order)
			// 发送新订单事件
			if me.eventHandler != nil {
				me.eventHandler.OnOrderUpdate(order)
			}
		}
	}
}

// processTrades 处理成交
func (me *MarketEngine) processTrades(result *models.TradeResult) {
	for _, trade := range result.Trades {
		me.lastPrice = trade.Price

		// 触发成交事件
		if me.eventHandler != nil {
			me.eventHandler.OnTrade(trade)
		}

		// 处理冰山订单
		if me.IcebergMgr.IsIcebergOrder(result.TakerOrder.ID) {
			me.IcebergMgr.OnTrade(result.TakerOrder.ID, trade.Amount)
		}
		if me.IcebergMgr.IsIcebergOrder(result.MakerOrder.ID) {
			me.IcebergMgr.OnTrade(result.MakerOrder.ID, trade.Amount)
		}
	}

	// 触发订单更新事件
	if me.eventHandler != nil && result.HasTrade() {
		if result.TakerOrder != nil {
			me.eventHandler.OnOrderUpdate(result.TakerOrder)
		}
		if result.MakerOrder != nil {
			me.eventHandler.OnOrderUpdate(result.MakerOrder)
		}
	}

	me.checkStopOrders()
}

// checkStopOrders 检查止损单触发
func (me *MarketEngine) checkStopOrders() {
	if me.lastPrice.IsZero() {
		return
	}

	triggeredOrders := me.StopManager.GetTriggeredOrders(me.lastPrice)

	for _, order := range triggeredOrders {
		me.StopManager.RemoveOrder(order)
		me.processOrder(order) // 直接处理，不经过Raft（简化）
	}
}

// HandleOrder 实现 consumer.OrderHandler 接口 - 从 Kafka 接收订单
func (e *Engine) HandleOrder(order *models.Order) error {
	return e.SubmitOrder(order)
}
