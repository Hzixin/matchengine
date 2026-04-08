package engine

import "github.com/hzx/matchengine/internal/models"

// EventHandler 事件处理器接口
type EventHandler interface {
	// OnTrade 成交事件
	OnTrade(trade *models.Trade) error

	// OnOrderUpdate 订单更新事件
	OnOrderUpdate(order *models.Order) error

	// Name 处理器名称
	Name() string
}

// EventHandlerChain 事件处理器链
type EventHandlerChain struct {
	handlers []EventHandler
}

// NewEventHandlerChain 创建事件处理器链
func NewEventHandlerChain(handlers ...EventHandler) *EventHandlerChain {
	return &EventHandlerChain{handlers: handlers}
}

// OnTrade 处理成交事件
func (c *EventHandlerChain) OnTrade(trade *models.Trade) error {
	for _, h := range c.handlers {
		if err := h.OnTrade(trade); err != nil {
			// 记录错误但继续执行其他处理器
			// 具体的错误日志在处理器中记录
		}
	}
	return nil
}

// OnOrderUpdate 处理订单更新事件
func (c *EventHandlerChain) OnOrderUpdate(order *models.Order) error {
	for _, h := range c.handlers {
		if err := h.OnOrderUpdate(order); err != nil {
			// 同上,继续执行
		}
	}
	return nil
}
