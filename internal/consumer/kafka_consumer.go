package consumer

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/IBM/sarama"
	"github.com/hzx/matchengine/internal/config"
	"github.com/hzx/matchengine/internal/models"
	"go.uber.org/zap"
)

// OrderHandler 订单处理器
type OrderHandler interface {
	HandleOrder(order *models.Order) error
}

// KafkaConsumer Kafka消费者
type KafkaConsumer struct {
	consumer sarama.ConsumerGroup
	cfg      *config.KafkaConfig
	logger   *zap.Logger
	handler  OrderHandler

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewKafkaConsumer 创建Kafka消费者
func NewKafkaConsumer(cfg *config.KafkaConfig, logger *zap.Logger, handler OrderHandler) (*KafkaConsumer, error) {
	config := sarama.NewConfig()
	config.Consumer.Return.Errors = true
	config.Consumer.Offsets.Initial = sarama.OffsetNewest
	config.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin

	consumer, err := sarama.NewConsumerGroup(cfg.Brokers, cfg.ConsumerGroup, config)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := &KafkaConsumer{
		consumer: consumer,
		cfg:      cfg,
		logger:   logger,
		handler:  handler,
		ctx:      ctx,
		cancel:   cancel,
	}

	// 启动消费协程
	c.wg.Add(2)
	go c.consume()
	go c.handleErrors()

	return c, nil
}

// consume 消费消息
func (c *KafkaConsumer) consume() {
	defer c.wg.Done()

	handler := &consumerGroupHandler{
		logger:  c.logger,
		handler: c.handler,
		ready:   make(chan bool),
	}

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			if err := c.consumer.Consume(c.ctx, []string{c.cfg.OrderTopic}, handler); err != nil {
				c.logger.Error("consumer error", zap.Error(err))
			}
		}
	}
}

// handleErrors 处理错误
func (c *KafkaConsumer) handleErrors() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		case err := <-c.consumer.Errors():
			c.logger.Error("consumer error", zap.Error(err))
		}
	}
}

// Close 关闭消费者
func (c *KafkaConsumer) Close() error {
	c.cancel()
	c.wg.Wait()
	return c.consumer.Close()
}

// consumerGroupHandler 消费者组处理器
type consumerGroupHandler struct {
	logger  *zap.Logger
	handler OrderHandler
	ready   chan bool
}

// Setup 设置
func (h *consumerGroupHandler) Setup(sarama.ConsumerGroupSession) error {
	close(h.ready)
	return nil
}

// Cleanup 清理
func (h *consumerGroupHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim 消费
func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case <-session.Context().Done():
			return nil
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}

			// 解析订单
			var order models.Order
			if err := json.Unmarshal(msg.Value, &order); err != nil {
				h.logger.Error("failed to unmarshal order",
					zap.ByteString("value", msg.Value),
					zap.Error(err))
				session.MarkMessage(msg, "")
				continue
			}

			// 处理订单
			if err := h.handler.HandleOrder(&order); err != nil {
				h.logger.Error("failed to handle order",
					zap.Uint64("order_id", order.ID),
					zap.Error(err))
				// 不标记消息，稍后重试
				continue
			}

			// 标记消息已处理
			session.MarkMessage(msg, "")

			h.logger.Debug("order processed",
				zap.Uint64("order_id", order.ID),
				zap.String("symbol", order.Symbol))
		}
	}
}
