package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hzx/matchengine/internal/config"
	"github.com/hzx/matchengine/internal/engine"
	"github.com/hzx/matchengine/internal/models"
	"github.com/labstack/echo/v4"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// MockMySQLStorage 模拟MySQL存储
type MockMySQLStorage struct {
	userOrders map[uint64][]*models.Order
	trades     map[string][]*models.Trade
	err        error
}

func NewMockMySQLStorage() *MockMySQLStorage {
	return &MockMySQLStorage{
		userOrders: make(map[uint64][]*models.Order),
		trades:     make(map[string][]*models.Trade),
	}
}

func (m *MockMySQLStorage) GetUserOrders(userID uint64, symbol string, limit int, offset int) ([]*models.Order, error) {
	if m.err != nil {
		return nil, m.err
	}
	orders := m.userOrders[userID]
	if symbol != "" {
		filtered := make([]*models.Order, 0)
		for _, o := range orders {
			if o.Symbol == symbol {
				filtered = append(filtered, o)
			}
		}
		orders = filtered
	}
	if offset >= len(orders) {
		return []*models.Order{}, nil
	}
	end := offset + limit
	if end > len(orders) {
		end = len(orders)
	}
	return orders[offset:end], nil
}

func (m *MockMySQLStorage) GetTrades(symbol string, limit int, offset int) ([]*models.Trade, error) {
	if m.err != nil {
		return nil, m.err
	}
	trades := m.trades[symbol]
	if offset >= len(trades) {
		return []*models.Trade{}, nil
	}
	end := offset + limit
	if end > len(trades) {
		end = len(trades)
	}
	return trades[offset:end], nil
}

// setupTestServer 创建测试服务器
func setupTestServer(mockDB OrderQuerier) (*echo.Echo, *engine.Engine, *zap.Logger) {
	logger, _ := zap.NewDevelopment()

	cfg := &config.Config{
		Markets: []config.MarketConfig{
			{
				Symbol:        "BTC_USDT",
				BaseAsset:     "BTC",
				QuoteAsset:    "USDT",
				BasePrecision: 8,
				QuotePrecision: 2,
				MinAmount:     "0.001",
				MinTotal:      "10",
				PriceTick:     "0.01",
				AmountTick:    "0.0001",
			},
		},
	}

	eng := engine.NewEngine(cfg, logger)
	eng.Start()

	e := NewEchoServer(eng, mockDB, logger)

	return e, eng, logger
}

func TestGetUserOrders(t *testing.T) {
	mockMySQL := NewMockMySQLStorage()

	// 添加测试数据
	mockMySQL.userOrders[1] = []*models.Order{
		models.NewOrder(1001, 1, "BTC_USDT", models.OrderSideBuy, models.OrderTypeLimit,
			decimal.RequireFromString("50000"), decimal.RequireFromString("1"), models.TimeInForceGTC),
		models.NewOrder(1002, 1, "BTC_USDT", models.OrderSideSell, models.OrderTypeLimit,
			decimal.RequireFromString("51000"), decimal.RequireFromString("0.5"), models.TimeInForceGTC),
		models.NewOrder(1003, 1, "ETH_USDT", models.OrderSideBuy, models.OrderTypeLimit,
			decimal.RequireFromString("3000"), decimal.RequireFromString("2"), models.TimeInForceGTC),
	}

	e, eng, _ := setupTestServer(mockMySQL)
	defer eng.Stop()

	tests := []struct {
		name       string
		url        string
		wantStatus int
		wantCount  int
	}{
		{
			name:       "获取用户所有订单",
			url:        "/api/v1/orders/1",
			wantStatus: http.StatusOK,
			wantCount:  3,
		},
		{
			name:       "按交易对筛选订单",
			url:        "/api/v1/orders/1?symbol=BTC_USDT",
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name:       "分页查询",
			url:        "/api/v1/orders/1?limit=2&offset=0",
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name:       "分页偏移",
			url:        "/api/v1/orders/1?limit=2&offset=2",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name:       "用户无订单",
			url:        "/api/v1/orders/999",
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name:       "无效用户ID",
			url:        "/api/v1/orders/invalid",
			wantStatus: http.StatusBadRequest,
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()

			e.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]interface{}
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				assert.NoError(t, err)

				orders, ok := resp["orders"].([]interface{})
				assert.True(t, ok)
				assert.Equal(t, tt.wantCount, len(orders))
			}
		})
	}
}

func TestGetTrades(t *testing.T) {
	mockMySQL := NewMockMySQLStorage()

	// 添加测试数据
	mockMySQL.trades["BTC_USDT"] = []*models.Trade{
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
		{
			ID:           2,
			TakerOrderID: 1003,
			MakerOrderID: 1004,
			UserID:       3,
			MakerUserID:  4,
			Symbol:       "BTC_USDT",
			Price:        decimal.RequireFromString("50500"),
			Amount:       decimal.RequireFromString("1"),
			Total:        decimal.RequireFromString("50500"),
			Side:         models.OrderSideSell,
		},
	}

	e, eng, _ := setupTestServer(mockMySQL)
	defer eng.Stop()

	tests := []struct {
		name       string
		url        string
		wantStatus int
		wantCount  int
	}{
		{
			name:       "获取成交记录",
			url:        "/api/v1/trades/BTC_USDT",
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name:       "限制数量",
			url:        "/api/v1/trades/BTC_USDT?limit=1",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name:       "分页偏移",
			url:        "/api/v1/trades/BTC_USDT?limit=1&offset=1",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name:       "无成交记录的交易对",
			url:        "/api/v1/trades/ETH_USDT",
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()

			e.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)

			var resp map[string]interface{}
			err := json.Unmarshal(rec.Body.Bytes(), &resp)
			assert.NoError(t, err)

			trades, ok := resp["trades"].([]interface{})
			assert.True(t, ok)
			assert.Equal(t, tt.wantCount, len(trades))
		})
	}
}

func TestGetUserOrders_Error(t *testing.T) {
	mockMySQL := NewMockMySQLStorage()
	mockMySQL.err = assert.AnError

	e, eng, _ := setupTestServer(mockMySQL)
	defer eng.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders/1", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.True(t, strings.Contains(rec.Body.String(), "error"))
}

func TestGetTrades_Error(t *testing.T) {
	mockMySQL := NewMockMySQLStorage()
	mockMySQL.err = assert.AnError

	e, eng, _ := setupTestServer(mockMySQL)
	defer eng.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trades/BTC_USDT", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.True(t, strings.Contains(rec.Body.String(), "error"))
}
