package engine

import (
	"sync"
	"testing"

	"github.com/hzx/matchengine/internal/models"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// MockEventHandler 模拟事件处理器
type MockEventHandler struct {
	trades       []*models.Trade
	orders       []*models.Order
	mu           sync.Mutex
	onTradeErr   error
	onOrderErr   error
}

func NewMockEventHandler() *MockEventHandler {
	return &MockEventHandler{
		trades: make([]*models.Trade, 0),
		orders: make([]*models.Order, 0),
	}
}

func (h *MockEventHandler) OnTrade(trade *models.Trade) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.trades = append(h.trades, trade)
	return h.onTradeErr
}

func (h *MockEventHandler) OnOrderUpdate(order *models.Order) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.orders = append(h.orders, order)
	return h.onOrderErr
}

func (h *MockEventHandler) Name() string {
	return "mock_handler"
}

func (h *MockEventHandler) GetTrades() []*models.Trade {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.trades
}

func (h *MockEventHandler) GetOrders() []*models.Order {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.orders
}

func (h *MockEventHandler) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.trades = make([]*models.Trade, 0)
	h.orders = make([]*models.Order, 0)
}

// TestEventHandlerChain 测试事件处理器链
func TestEventHandlerChain(t *testing.T) {
	handler1 := NewMockEventHandler()
	handler2 := NewMockEventHandler()
	chain := NewEventHandlerChain(handler1, handler2)

	trade := &models.Trade{
		ID:     1,
		Symbol: "BTC_USDT",
		Price:  decimal.NewFromFloat(50000.0),
		Amount: decimal.NewFromFloat(1.0),
	}

	order := &models.Order{
		ID:     100,
		Symbol: "BTC_USDT",
		Status: models.OrderStatusNew,
	}

	// 测试 OnTrade
	err := chain.OnTrade(trade)
	assert.NoError(t, err)

	assert.Len(t, handler1.GetTrades(), 1)
	assert.Len(t, handler2.GetTrades(), 1)
	assert.Equal(t, trade.ID, handler1.GetTrades()[0].ID)
	assert.Equal(t, trade.ID, handler2.GetTrades()[0].ID)

	// 测试 OnOrderUpdate
	err = chain.OnOrderUpdate(order)
	assert.NoError(t, err)

	assert.Len(t, handler1.GetOrders(), 1)
	assert.Len(t, handler2.GetOrders(), 1)
	assert.Equal(t, order.ID, handler1.GetOrders()[0].ID)
	assert.Equal(t, order.ID, handler2.GetOrders()[0].ID)
}

// TestEventHandlerChainErrorContinue 测试处理器链错误继续执行
func TestEventHandlerChainErrorContinue(t *testing.T) {
	handler1 := NewMockEventHandler()
	handler1.onTradeErr = assert.AnError // 第一个处理器返回错误

	handler2 := NewMockEventHandler()
	chain := NewEventHandlerChain(handler1, handler2)

	trade := &models.Trade{
		ID:     1,
		Symbol: "BTC_USDT",
	}

	// 即使第一个处理器失败,第二个处理器仍应被调用
	err := chain.OnTrade(trade)
	assert.NoError(t, err) // chain 永不返回错误

	assert.Len(t, handler1.GetTrades(), 1)
	assert.Len(t, handler2.GetTrades(), 1) // 第二个处理器仍然收到事件
}

// TestEventHandlerChainEmpty 测试空处理器链
func TestEventHandlerChainEmpty(t *testing.T) {
	chain := NewEventHandlerChain()

	trade := &models.Trade{ID: 1}
	order := &models.Order{ID: 100}

	// 空链不应 panic
	err := chain.OnTrade(trade)
	assert.NoError(t, err)

	err = chain.OnOrderUpdate(order)
	assert.NoError(t, err)
}

// TestStorageHandler 测试存储处理器接口
func TestStorageHandler_Name(t *testing.T) {
	// 只测试 Name 方法,实际存储需要真实 MySQL
	// 集成测试会测试完整功能
	handler := &StorageHandler{}
	assert.Equal(t, "mysql_storage", handler.Name())
}

// TestKafkaHandler_Name 测试 Kafka 处理器接口
func TestKafkaHandler_Name(t *testing.T) {
	// 只测试 Name 方法,实际发送需要真实 Kafka
	// 集成测试会测试完整功能
	handler := &KafkaHandler{}
	assert.Equal(t, "kafka_producer", handler.Name())
}
