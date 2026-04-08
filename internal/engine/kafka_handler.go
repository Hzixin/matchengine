package engine

import (
	"github.com/hzx/matchengine/internal/models"
	"github.com/hzx/matchengine/internal/producer"
	"go.uber.org/zap"
)

// KafkaHandler Kafka事件处理器
type KafkaHandler struct {
	syncProducer  *producer.KafkaProducer
	asyncProducer *producer.AsyncProducer
	useAsync      bool
	logger        *zap.Logger
}

// NewKafkaHandler 创建Kafka事件处理器
func NewKafkaHandler(
	syncProducer *producer.KafkaProducer,
	asyncProducer *producer.AsyncProducer,
	useAsync bool,
	logger *zap.Logger,
) *KafkaHandler {
	return &KafkaHandler{
		syncProducer:  syncProducer,
		asyncProducer: asyncProducer,
		useAsync:      useAsync,
		logger:        logger,
	}
}

// OnTrade 处理成交事件
func (h *KafkaHandler) OnTrade(trade *models.Trade) error {
	if h.useAsync && h.asyncProducer != nil {
		h.asyncProducer.SendTrade(trade)
		return nil
	}

	if h.syncProducer != nil {
		if err := h.syncProducer.SendTrade(trade); err != nil {
			h.logger.Error("failed to send trade to kafka",
				zap.Uint64("trade_id", trade.ID),
				zap.Error(err))
			return err
		}
	}
	return nil
}

// OnOrderUpdate 处理订单更新事件
func (h *KafkaHandler) OnOrderUpdate(order *models.Order) error {
	if h.useAsync && h.asyncProducer != nil {
		h.asyncProducer.SendOrder(order)
		return nil
	}

	if h.syncProducer != nil {
		if err := h.syncProducer.SendOrderUpdate(order); err != nil {
			h.logger.Error("failed to send order update to kafka",
				zap.Uint64("order_id", order.ID),
				zap.Error(err))
			return err
		}
	}
	return nil
}

// Name 返回处理器名称
func (h *KafkaHandler) Name() string {
	return "kafka_producer"
}
