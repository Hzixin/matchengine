package producer

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/IBM/sarama"
	"github.com/hzx/matchengine/internal/config"
	"github.com/hzx/matchengine/internal/models"
	"go.uber.org/zap"
)

// KafkaProducer Kafka生产者
type KafkaProducer struct {
	producer sarama.SyncProducer
	cfg      *config.KafkaConfig
	logger   *zap.Logger

	wg sync.WaitGroup
}

// NewKafkaProducer 创建Kafka生产者
func NewKafkaProducer(cfg *config.KafkaConfig, logger *zap.Logger) (*KafkaProducer, error) {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 5
	config.Producer.Return.Successes = true
	config.Producer.Partitioner = sarama.NewHashPartitioner

	producer, err := sarama.NewSyncProducer(cfg.Brokers, config)
	if err != nil {
		return nil, err
	}

	return &KafkaProducer{
		producer: producer,
		cfg:      cfg,
		logger:   logger,
	}, nil
}

// SendTrade 发送成交消息
func (p *KafkaProducer) SendTrade(trade *models.Trade) error {
	data, err := json.Marshal(trade)
	if err != nil {
		return err
	}

	msg := &sarama.ProducerMessage{
		Topic: p.cfg.TradeTopic,
		Key:   sarama.StringEncoder(trade.Symbol),
		Value: sarama.ByteEncoder(data),
	}

	partition, offset, err := p.producer.SendMessage(msg)
	if err != nil {
		p.logger.Error("failed to send trade message",
			zap.Uint64("trade_id", trade.ID),
			zap.Error(err))
		return err
	}

	p.logger.Debug("trade message sent",
		zap.Uint64("trade_id", trade.ID),
		zap.Int32("partition", partition),
		zap.Int64("offset", offset))

	return nil
}

// SendTrades 批量发送成交消息
func (p *KafkaProducer) SendTrades(trades []*models.Trade) error {
	for _, trade := range trades {
		if err := p.SendTrade(trade); err != nil {
			return err
		}
	}
	return nil
}

// SendOrderUpdate 发送订单更新消息
func (p *KafkaProducer) SendOrderUpdate(order *models.Order) error {
	data, err := json.Marshal(order)
	if err != nil {
		return err
	}

	msg := &sarama.ProducerMessage{
		Topic: p.cfg.OrderTopic,
		Key:   sarama.StringEncoder(order.Symbol),
		Value: sarama.ByteEncoder(data),
	}

	_, _, err = p.producer.SendMessage(msg)
	return err
}

// Close 关闭生产者
func (p *KafkaProducer) Close() error {
	p.wg.Wait()
	return p.producer.Close()
}

// AsyncProducer 异步生产者
type AsyncProducer struct {
	producer sarama.AsyncProducer
	cfg      *config.KafkaConfig
	logger   *zap.Logger

	tradeChan chan *models.Trade
	orderChan chan *models.Order

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewAsyncProducer 创建异步生产者
func NewAsyncProducer(cfg *config.KafkaConfig, logger *zap.Logger) (*AsyncProducer, error) {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForLocal
	config.Producer.Retry.Max = 5
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true
	config.Producer.Flush.Frequency = 10 * 1000000 // 10ms
	config.Producer.Flush.Messages = 100

	producer, err := sarama.NewAsyncProducer(cfg.Brokers, config)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	p := &AsyncProducer{
		producer:  producer,
		cfg:       cfg,
		logger:    logger,
		tradeChan: make(chan *models.Trade, 10000),
		orderChan: make(chan *models.Order, 10000),
		ctx:       ctx,
		cancel:    cancel,
	}

	// 启动处理协程
	p.wg.Add(3)
	go p.handleSuccesses()
	go p.handleErrors()
	go p.processMessages()

	return p, nil
}

// SendTrade 发送成交消息（异步）
func (p *AsyncProducer) SendTrade(trade *models.Trade) {
	select {
	case p.tradeChan <- trade:
	default:
		p.logger.Warn("trade channel full, dropping message",
			zap.Uint64("trade_id", trade.ID))
	}
}

// SendOrder 发送订单消息（异步）
func (p *AsyncProducer) SendOrder(order *models.Order) {
	select {
	case p.orderChan <- order:
	default:
		p.logger.Warn("order channel full, dropping message",
			zap.Uint64("order_id", order.ID))
	}
}

// processMessages 处理消息
func (p *AsyncProducer) processMessages() {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case trade := <-p.tradeChan:
			data, err := json.Marshal(trade)
			if err != nil {
				p.logger.Error("failed to marshal trade", zap.Error(err))
				continue
			}

			msg := &sarama.ProducerMessage{
				Topic: p.cfg.TradeTopic,
				Key:   sarama.StringEncoder(trade.Symbol),
				Value: sarama.ByteEncoder(data),
			}
			p.producer.Input() <- msg

		case order := <-p.orderChan:
			data, err := json.Marshal(order)
			if err != nil {
				p.logger.Error("failed to marshal order", zap.Error(err))
				continue
			}

			msg := &sarama.ProducerMessage{
				Topic: p.cfg.OrderTopic,
				Key:   sarama.StringEncoder(order.Symbol),
				Value: sarama.ByteEncoder(data),
			}
			p.producer.Input() <- msg
		}
	}
}

// handleSuccesses 处理成功消息
func (p *AsyncProducer) handleSuccesses() {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case msg := <-p.producer.Successes():
			p.logger.Debug("message sent successfully",
				zap.String("topic", msg.Topic),
				zap.Int32("partition", msg.Partition),
				zap.Int64("offset", msg.Offset))
		}
	}
}

// handleErrors 处理错误消息
func (p *AsyncProducer) handleErrors() {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case err := <-p.producer.Errors():
			p.logger.Error("failed to send message",
				zap.String("topic", err.Msg.Topic),
				zap.Error(err.Err))
		}
	}
}

// Close 关闭生产者
func (p *AsyncProducer) Close() error {
	p.cancel()
	p.wg.Wait()
	return p.producer.Close()
}
