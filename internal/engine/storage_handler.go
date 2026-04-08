package engine

import (
	"github.com/hzx/matchengine/internal/models"
	"github.com/hzx/matchengine/internal/storage"
	"go.uber.org/zap"
)

// StorageHandler MySQL存储事件处理器
type StorageHandler struct {
	storage *storage.MySQLStorage
	logger  *zap.Logger
}

// NewStorageHandler 创建MySQL存储事件处理器
func NewStorageHandler(storage *storage.MySQLStorage, logger *zap.Logger) *StorageHandler {
	return &StorageHandler{
		storage: storage,
		logger:  logger,
	}
}

// OnTrade 处理成交事件
func (h *StorageHandler) OnTrade(trade *models.Trade) error {
	if err := h.storage.SaveTrade(trade); err != nil {
		h.logger.Error("failed to save trade to mysql",
			zap.Uint64("trade_id", trade.ID),
			zap.Error(err))
		return err
	}
	return nil
}

// OnOrderUpdate 处理订单更新事件
func (h *StorageHandler) OnOrderUpdate(order *models.Order) error {
	if err := h.storage.SaveOrder(order); err != nil {
		h.logger.Error("failed to save order to mysql",
			zap.Uint64("order_id", order.ID),
			zap.Error(err))
		return err
	}
	return nil
}

// Name 返回处理器名称
func (h *StorageHandler) Name() string {
	return "mysql_storage"
}
