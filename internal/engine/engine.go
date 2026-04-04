package engine

import (
	"errors"
	"sync"
	"time"

	"github.com/hzx/matchengine/internal/config"
	"github.com/hzx/matchengine/internal/matching"
	"github.com/hzx/matchengine/internal/models"
	"github.com/hzx/matchengine/internal/orderbook"
	"github.com/hzx/matchengine/pkg/utils"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

var (
	ErrEngineStopped = errors.New("engine stopped")
	ErrMarketNotFound = errors.New("market not found")
	ErrInvalidOrder = errors.New("invalid order")
	ErrOrderNotFound = errors.New("order not found")
)

// Engine 撮合引擎
type Engine struct {
	cfg     *config.Config
	logger  *zap.Logger

	// 市场
	markets map[string]*MarketEngine

	// ID生成器
	idGenerator *utils.Snowflake

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
	limitMatcher  *matching.LimitMatcher
	marketMatcher *matching.MarketMatcher
	stopMatcher   *matching.StopMatcher
	icebergMatcher *matching.IcebergMatcher

	// 订单通道
	orderChan chan *models.Order

	// 最新价格
	lastPrice decimal.Decimal

	// ID生成器
	idGenerator *utils.Snowflake
}

// NewEngine 创建撮合引擎
func NewEngine(cfg *config.Config, logger *zap.Logger) *Engine {
	return &Engine{
		cfg:         cfg,
		logger:      logger,
		markets:     make(map[string]*MarketEngine),
		idGenerator: utils.NewSnowflake(1),
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
			Market:        market,
			OrderBook:     orderbook.NewOrderBook(marketCfg.Symbol),
			StopManager:   matching.NewStopOrderManager(),
			IcebergMgr:    matching.NewIcebergOrderManager(),
			limitMatcher:  matching.NewLimitMatcher(),
			marketMatcher: matching.NewMarketMatcher(),
			stopMatcher:   matching.NewStopMatcher(),
			icebergMatcher: matching.NewIcebergMatcher(),
			orderChan:     make(chan *models.Order, 10000),
			lastPrice:     decimal.Zero,
			idGenerator:   e.idGenerator,
		}

		e.markets[marketCfg.Symbol] = me

		// 启动市场处理协程
		go me.processOrders()
	}

	e.running = true
	e.logger.Info("matching engine started", zap.Int("markets", len(e.markets)))
	return nil
}

// Stop 停止引擎
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.running = false
	for _, me := range e.markets {
		close(me.orderChan)
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

	// 发送到订单通道
	me.orderChan <- order

	return nil
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

	// 获取买卖盘
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

// processOrders 处理订单
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
		// 止损单先加入管理器
		me.StopManager.AddOrder(order)
		return
	case models.OrderTypeIceberg:
		// 冰山单注册到管理器
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
			// 市价单未成交部分取消
			order.Status = models.OrderStatusCancelled
		} else {
			// 限价单加入订单簿
			me.OrderBook.AddOrder(order)
		}
	}
}

// processTrades 处理成交
func (me *MarketEngine) processTrades(result *models.TradeResult) {
	for _, trade := range result.Trades {
		// 更新最新价格
		me.lastPrice = trade.Price

		// 更新冰山单状态
		if me.IcebergMgr.IsIcebergOrder(result.TakerOrder.ID) {
			me.IcebergMgr.OnTrade(result.TakerOrder.ID, trade.Amount)
		}
		if me.IcebergMgr.IsIcebergOrder(result.MakerOrder.ID) {
			me.IcebergMgr.OnTrade(result.MakerOrder.ID, trade.Amount)
		}

		// TODO: 发送成交消息到Kafka
		// TODO: 更新Redis缓存
		// TODO: 持久化到MySQL
	}

	// 检查止损单触发
	me.checkStopOrders()
}

// checkStopOrders 检查止损单触发
func (me *MarketEngine) checkStopOrders() {
	if me.lastPrice.IsZero() {
		return
	}

	// 获取触发的止损单
	triggeredOrders := me.StopManager.GetTriggeredOrders(me.lastPrice)

	// 处理触发的止损单
	for _, order := range triggeredOrders {
		// 从止损管理器移除
		me.StopManager.RemoveOrder(order)

		// 重新提交到订单通道
		me.orderChan <- order
	}
}
