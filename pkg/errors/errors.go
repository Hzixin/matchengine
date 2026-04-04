package errors

import (
	"errors"
	"fmt"
)

var (
	// 订单错误
	ErrOrderNotFound      = errors.New("order not found")
	ErrOrderAlreadyExists = errors.New("order already exists")
	ErrOrderCancelled     = errors.New("order already cancelled")
	ErrOrderFilled        = errors.New("order already filled")
	ErrOrderExpired       = errors.New("order expired")

	// 市场错误
	ErrMarketNotFound    = errors.New("market not found")
	ErrMarketSuspended   = errors.New("market suspended")
	ErrInvalidSymbol     = errors.New("invalid symbol")

	// 价格/数量错误
	ErrInvalidPrice      = errors.New("invalid price")
	ErrInvalidAmount     = errors.New("invalid amount")
	ErrPriceTooLow       = errors.New("price too low")
	ErrPriceTooHigh      = errors.New("price too high")
	ErrAmountTooSmall    = errors.New("amount too small")
	ErrAmountTooLarge    = errors.New("amount too large")
	ErrInsufficientBalance = errors.New("insufficient balance")

	// 订单类型错误
	ErrInvalidOrderType  = errors.New("invalid order type")
	ErrInvalidSide       = errors.New("invalid side")
	ErrStopOrderNotTriggered = errors.New("stop order not triggered")

	// 引擎错误
	ErrEngineNotReady    = errors.New("engine not ready")
	ErrEngineStopped     = errors.New("engine stopped")

	// 存储错误
	ErrStorageFailed     = errors.New("storage operation failed")
	ErrCacheFailed       = errors.New("cache operation failed")
)

// OrderError 订单相关错误
type OrderError struct {
	OrderID uint64
	Err     error
}

func (e *OrderError) Error() string {
	return fmt.Sprintf("order %d: %v", e.OrderID, e.Err)
}

func (e *OrderError) Unwrap() error {
	return e.Err
}

// MarketError 市场相关错误
type MarketError struct {
	Symbol string
	Err    error
}

func (e *MarketError) Error() string {
	return fmt.Sprintf("market %s: %v", e.Symbol, e.Err)
}

func (e *MarketError) Unwrap() error {
	return e.Err
}

// ValidationError 验证错误
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error: %s - %s", e.Field, e.Message)
}
