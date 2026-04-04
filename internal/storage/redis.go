package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hzx/matchengine/internal/config"
	"github.com/hzx/matchengine/internal/models"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// RedisStorage Redis存储
type RedisStorage struct {
	client *redis.Client
	logger *zap.Logger
}

// NewRedisStorage 创建Redis存储
func NewRedisStorage(cfg *config.RedisConfig, logger *zap.Logger) (*RedisStorage, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: 100,
	})

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &RedisStorage{
		client: client,
		logger: logger,
	}, nil
}

// SaveOrder 保存订单到Redis
func (s *RedisStorage) SaveOrder(order *models.Order) error {
	ctx := context.Background()
	key := fmt.Sprintf("order:%s:%d", order.Symbol, order.ID)

	data, err := json.Marshal(order)
	if err != nil {
		return err
	}

	return s.client.Set(ctx, key, data, 24*time.Hour).Err()
}

// GetOrder 从Redis获取订单
func (s *RedisStorage) GetOrder(symbol string, orderID uint64) (*models.Order, error) {
	ctx := context.Background()
	key := fmt.Sprintf("order:%s:%d", symbol, orderID)

	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var order models.Order
	if err := json.Unmarshal(data, &order); err != nil {
		return nil, err
	}

	return &order, nil
}

// DeleteOrder 删除订单
func (s *RedisStorage) DeleteOrder(symbol string, orderID uint64) error {
	ctx := context.Background()
	key := fmt.Sprintf("order:%s:%d", symbol, orderID)
	return s.client.Del(ctx, key).Err()
}

// SaveUserOrders 保存用户订单列表
func (s *RedisStorage) SaveUserOrders(userID uint64, symbol string, orderIDs []uint64) error {
	ctx := context.Background()
	key := fmt.Sprintf("user:%d:orders:%s", userID, symbol)

	// 使用sorted set，score为订单ID（时间有序）
	members := make([]redis.Z, len(orderIDs))
	for i, id := range orderIDs {
		members[i] = redis.Z{
			Score:  float64(id),
			Member: id,
		}
	}

	pipe := s.client.Pipeline()
	pipe.ZAdd(ctx, key, members...)
	pipe.ZRemRangeByRank(ctx, key, 0, -101) // 只保留最近100个订单
	_, err := pipe.Exec(ctx)
	return err
}

// GetUserOrders 获取用户订单列表
func (s *RedisStorage) GetUserOrders(userID uint64, symbol string, limit int64) ([]uint64, error) {
	ctx := context.Background()
	key := fmt.Sprintf("user:%d:orders:%s", userID, symbol)

	result, err := s.client.ZRevRange(ctx, key, 0, limit-1).Result()
	if err != nil {
		return nil, err
	}

	orders := make([]uint64, len(result))
	for i, id := range result {
		var orderID uint64
		fmt.Sscanf(id, "%d", &orderID)
		orders[i] = orderID
	}

	return orders, nil
}

// SaveTicker 保存行情
func (s *RedisStorage) SaveTicker(ticker *models.Ticker) error {
	ctx := context.Background()
	key := fmt.Sprintf("ticker:%s", ticker.Symbol)

	data, err := json.Marshal(ticker)
	if err != nil {
		return err
	}

	return s.client.Set(ctx, key, data, 0).Err()
}

// GetTicker 获取行情
func (s *RedisStorage) GetTicker(symbol string) (*models.Ticker, error) {
	ctx := context.Background()
	key := fmt.Sprintf("ticker:%s", symbol)

	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var ticker models.Ticker
	if err := json.Unmarshal(data, &ticker); err != nil {
		return nil, err
	}

	return &ticker, nil
}

// SaveDepth 保存盘口深度
func (s *RedisStorage) SaveDepth(depth *models.Depth) error {
	ctx := context.Background()
	key := fmt.Sprintf("depth:%s", depth.Symbol)

	// 使用hash存储
	pipe := s.client.Pipeline()

	// 保存买单
	for i, level := range depth.Bids {
		bidKey := fmt.Sprintf("bid:%d", i)
		pipe.HSet(ctx, key, bidKey+":price", level.Price.String())
		pipe.HSet(ctx, key, bidKey+":amount", level.Amount.String())
	}

	// 保存卖单
	for i, level := range depth.Asks {
		askKey := fmt.Sprintf("ask:%d", i)
		pipe.HSet(ctx, key, askKey+":price", level.Price.String())
		pipe.HSet(ctx, key, askKey+":amount", level.Amount.String())
	}

	_, err := pipe.Exec(ctx)
	return err
}

// GetDepth 获取盘口深度
func (s *RedisStorage) GetDepth(symbol string, limit int) (*models.Depth, error) {
	ctx := context.Background()
	key := fmt.Sprintf("depth:%s", symbol)

	depth := &models.Depth{
		Symbol: symbol,
		Bids:   make([]*models.DepthLevel, 0, limit),
		Asks:   make([]*models.DepthLevel, 0, limit),
	}

	// 获取买单
	for i := 0; i < limit; i++ {
		priceStr, err := s.client.HGet(ctx, key, fmt.Sprintf("bid:%d:price", i)).Result()
		if err == redis.Nil {
			break
		}
		if err != nil {
			return nil, err
		}

		amountStr, err := s.client.HGet(ctx, key, fmt.Sprintf("bid:%d:amount", i)).Result()
		if err != nil {
			return nil, err
		}

		price, _ := decimal.NewFromString(priceStr)
		amount, _ := decimal.NewFromString(amountStr)

		depth.Bids = append(depth.Bids, &models.DepthLevel{
			Price:  price,
			Amount: amount,
		})
	}

	// 获取卖单
	for i := 0; i < limit; i++ {
		priceStr, err := s.client.HGet(ctx, key, fmt.Sprintf("ask:%d:price", i)).Result()
		if err == redis.Nil {
			break
		}
		if err != nil {
			return nil, err
		}

		amountStr, err := s.client.HGet(ctx, key, fmt.Sprintf("ask:%d:amount", i)).Result()
		if err != nil {
			return nil, err
		}

		price, _ := decimal.NewFromString(priceStr)
		amount, _ := decimal.NewFromString(amountStr)

		depth.Asks = append(depth.Asks, &models.DepthLevel{
			Price:  price,
			Amount: amount,
		})
	}

	return depth, nil
}

// SetLastTradePrice 设置最新成交价
func (s *RedisStorage) SetLastTradePrice(symbol string, price decimal.Decimal) error {
	ctx := context.Background()
	key := fmt.Sprintf("last_price:%s", symbol)
	return s.client.Set(ctx, key, price.String(), 0).Err()
}

// GetLastTradePrice 获取最新成交价
func (s *RedisStorage) GetLastTradePrice(symbol string) (decimal.Decimal, error) {
	ctx := context.Background()
	key := fmt.Sprintf("last_price:%s", symbol)

	priceStr, err := s.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return decimal.Zero, nil
		}
		return decimal.Zero, err
	}

	return decimal.NewFromString(priceStr)
}

// Close 关闭连接
func (s *RedisStorage) Close() error {
	return s.client.Close()
}
