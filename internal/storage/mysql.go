package storage

import (
	"fmt"
	"time"

	"github.com/hzx/matchengine/internal/config"
	"github.com/hzx/matchengine/internal/models"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OrderRecord 订单记录
type OrderRecord struct {
	ID           uint64 `gorm:"primaryKey"`
	UserID       uint64 `gorm:"index:idx_user_symbol"`
	Symbol       string `gorm:"size:20;index:idx_symbol;index:idx_user_symbol"`
	Side         int    `gorm:"type:tinyint"`
	Type         int    `gorm:"type:tinyint"`
	Status       int    `gorm:"type:tinyint;index:idx_status"`
	Price        string `gorm:"type:decimal(30,18)"`
	Amount       string `gorm:"type:decimal(30,18)"`
	FilledAmount string `gorm:"type:decimal(30,18)"`
	RemainAmount string `gorm:"type:decimal(30,18)"`
	FilledTotal  string `gorm:"type:decimal(30,18)"`
	Fee          string `gorm:"type:decimal(30,18)"`
	FeeAsset     string `gorm:"size:20"`
	TimeInForce  int    `gorm:"type:tinyint"`
	StopPrice    string `gorm:"type:decimal(30,18)"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TableName 表名
func (OrderRecord) TableName() string {
	return "orders"
}

// TradeRecord 成交记录
type TradeRecord struct {
	ID           uint64 `gorm:"primaryKey"`
	TakerOrderID uint64 `gorm:"index:idx_taker_order"`
	MakerOrderID uint64 `gorm:"index:idx_maker_order"`
	UserID       uint64 `gorm:"index:idx_user"`
	MakerUserID  uint64 `gorm:"index:idx_maker_user"`
	Symbol       string `gorm:"size:20;index:idx_symbol;index:idx_user_symbol"`
	Price        string `gorm:"type:decimal(30,18)"`
	Amount       string `gorm:"type:decimal(30,18)"`
	Total        string `gorm:"type:decimal(30,18)"`
	Fee          string `gorm:"type:decimal(30,18)"`
	FeeAsset     string `gorm:"size:20"`
	Side         int    `gorm:"type:tinyint"`
	CreatedAt    time.Time `gorm:"index:idx_time"`
}

// TableName 表名
func (TradeRecord) TableName() string {
	return "trades"
}

// KlineRecord K线记录
type KlineRecord struct {
	ID        uint64 `gorm:"primaryKey"`
	Symbol    string `gorm:"size:20;uniqueIndex:idx_symbol_time"`
	OpenTime  int64  `gorm:"uniqueIndex:idx_symbol_time"`
	CloseTime int64
	Open      string `gorm:"type:decimal(30,18)"`
	High      string `gorm:"type:decimal(30,18)"`
	Low       string `gorm:"type:decimal(30,18)"`
	Close     string `gorm:"type:decimal(30,18)"`
	Volume    string `gorm:"type:decimal(30,18)"`
	Amount    string `gorm:"type:decimal(30,18)"`
	CreatedAt time.Time
}

// TableName 表名
func (KlineRecord) TableName() string {
	return "klines"
}

// MySQLStorage MySQL存储
type MySQLStorage struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewMySQLStorage 创建MySQL存储
func NewMySQLStorage(cfg *config.MySQLConfig, logger *zap.Logger) (*MySQLStorage, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// 自动迁移
	if err := db.AutoMigrate(&OrderRecord{}, &TradeRecord{}, &KlineRecord{}); err != nil {
		return nil, err
	}

	return &MySQLStorage{
		db:     db,
		logger: logger,
	}, nil
}

// SaveOrder 保存订单
func (s *MySQLStorage) SaveOrder(order *models.Order) error {
	record := &OrderRecord{
		ID:           order.ID,
		UserID:       order.UserID,
		Symbol:       order.Symbol,
		Side:         int(order.Side),
		Type:         int(order.Type),
		Status:       int(order.Status),
		Price:        order.Price.String(),
		Amount:       order.Amount.String(),
		FilledAmount: order.FilledAmount.String(),
		RemainAmount: order.RemainAmount.String(),
		FilledTotal:  order.FilledTotal.String(),
		Fee:          order.Fee.String(),
		FeeAsset:     order.FeeAsset,
		TimeInForce:  int(order.TimeInForce),
		StopPrice:    order.StopPrice.String(),
		CreatedAt:    order.CreatedAt,
		UpdatedAt:    order.UpdatedAt,
	}

	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"status", "filled_amount", "remain_amount", "filled_total", "fee", "updated_at"}),
	}).Create(record).Error
}

// GetOrder 获取订单
func (s *MySQLStorage) GetOrder(orderID uint64) (*models.Order, error) {
	var record OrderRecord
	if err := s.db.First(&record, orderID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return s.recordToOrder(&record), nil
}

// GetUserOrders 获取用户订单
func (s *MySQLStorage) GetUserOrders(userID uint64, symbol string, limit int, offset int) ([]*models.Order, error) {
	var records []OrderRecord
	query := s.db.Where("user_id = ?", userID)
	if symbol != "" {
		query = query.Where("symbol = ?", symbol)
	}
	if err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&records).Error; err != nil {
		return nil, err
	}

	orders := make([]*models.Order, len(records))
	for i, record := range records {
		orders[i] = s.recordToOrder(&record)
	}
	return orders, nil
}

// SaveTrade 保存成交
func (s *MySQLStorage) SaveTrade(trade *models.Trade) error {
	record := &TradeRecord{
		ID:           trade.ID,
		TakerOrderID: trade.TakerOrderID,
		MakerOrderID: trade.MakerOrderID,
		UserID:       trade.UserID,
		MakerUserID:  trade.MakerUserID,
		Symbol:       trade.Symbol,
		Price:        trade.Price.String(),
		Amount:       trade.Amount.String(),
		Total:        trade.Total.String(),
		Fee:          trade.Fee.String(),
		FeeAsset:     trade.FeeAsset,
		Side:         int(trade.Side),
		CreatedAt:    trade.CreatedAt,
	}

	return s.db.Create(record).Error
}

// SaveTrades 批量保存成交
func (s *MySQLStorage) SaveTrades(trades []*models.Trade) error {
	records := make([]*TradeRecord, len(trades))
	for i, trade := range trades {
		records[i] = &TradeRecord{
			ID:           trade.ID,
			TakerOrderID: trade.TakerOrderID,
			MakerOrderID: trade.MakerOrderID,
			UserID:       trade.UserID,
			MakerUserID:  trade.MakerUserID,
			Symbol:       trade.Symbol,
			Price:        trade.Price.String(),
			Amount:       trade.Amount.String(),
			Total:        trade.Total.String(),
			Fee:          trade.Fee.String(),
			FeeAsset:     trade.FeeAsset,
			Side:         int(trade.Side),
			CreatedAt:    trade.CreatedAt,
		}
	}

	return s.db.CreateInBatches(records, 100).Error
}

// GetTrades 获取成交记录
func (s *MySQLStorage) GetTrades(symbol string, limit int, offset int) ([]*models.Trade, error) {
	var records []TradeRecord
	query := s.db.Where("symbol = ?", symbol)
	if err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&records).Error; err != nil {
		return nil, err
	}

	trades := make([]*models.Trade, len(records))
	for i, record := range records {
		trades[i] = s.recordToTrade(&record)
	}
	return trades, nil
}

// recordToOrder 记录转订单
func (s *MySQLStorage) recordToOrder(record *OrderRecord) *models.Order {
	price, _ := decimal.NewFromString(record.Price)
	amount, _ := decimal.NewFromString(record.Amount)
	filledAmount, _ := decimal.NewFromString(record.FilledAmount)
	remainAmount, _ := decimal.NewFromString(record.RemainAmount)
	filledTotal, _ := decimal.NewFromString(record.FilledTotal)
	fee, _ := decimal.NewFromString(record.Fee)
	stopPrice, _ := decimal.NewFromString(record.StopPrice)

	return &models.Order{
		ID:           record.ID,
		UserID:       record.UserID,
		Symbol:       record.Symbol,
		Side:         models.OrderSide(record.Side),
		Type:         models.OrderType(record.Type),
		Status:       models.OrderStatus(record.Status),
		Price:        price,
		Amount:       amount,
		FilledAmount: filledAmount,
		RemainAmount: remainAmount,
		FilledTotal:  filledTotal,
		Fee:          fee,
		FeeAsset:     record.FeeAsset,
		TimeInForce:  models.TimeInForce(record.TimeInForce),
		StopPrice:    stopPrice,
		CreatedAt:    record.CreatedAt,
		UpdatedAt:    record.UpdatedAt,
	}
}

// recordToTrade 记录转成交
func (s *MySQLStorage) recordToTrade(record *TradeRecord) *models.Trade {
	price, _ := decimal.NewFromString(record.Price)
	amount, _ := decimal.NewFromString(record.Amount)
	total, _ := decimal.NewFromString(record.Total)
	fee, _ := decimal.NewFromString(record.Fee)

	return &models.Trade{
		ID:           record.ID,
		TakerOrderID: record.TakerOrderID,
		MakerOrderID: record.MakerOrderID,
		UserID:       record.UserID,
		MakerUserID:  record.MakerUserID,
		Symbol:       record.Symbol,
		Price:        price,
		Amount:       amount,
		Total:        total,
		Fee:          fee,
		FeeAsset:     record.FeeAsset,
		Side:         models.OrderSide(record.Side),
		CreatedAt:    record.CreatedAt,
	}
}

// Close 关闭连接
func (s *MySQLStorage) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
