package orderbook

import (
	"container/list"
	"sync"

	"github.com/hzx/matchengine/internal/models"
	"github.com/shopspring/decimal"
)

// PriceLevel 价格档位
type PriceLevel struct {
	Price  decimal.Decimal
	Orders *list.List // 订单双向链表
	Total  decimal.Decimal // 总数量
}

// NewPriceLevel 创建价格档位
func NewPriceLevel(price decimal.Decimal) *PriceLevel {
	return &PriceLevel{
		Price:  price,
		Orders: list.New(),
		Total:  decimal.Zero,
	}
}

// AddOrder 添加订单
func (pl *PriceLevel) AddOrder(order *models.Order) {
	pl.Orders.PushBack(order)
	pl.Total = pl.Total.Add(order.RemainAmount)
}

// RemoveOrder 移除订单
func (pl *PriceLevel) RemoveOrder(orderID uint64) *models.Order {
	for e := pl.Orders.Front(); e != nil; e = e.Next() {
		if order, ok := e.Value.(*models.Order); ok && order.ID == orderID {
			pl.Orders.Remove(e)
			pl.Total = pl.Total.Sub(order.RemainAmount)
			return order
		}
	}
	return nil
}

// GetFirst 获取第一个订单
func (pl *PriceLevel) GetFirst() *models.Order {
	if pl.Orders.Len() == 0 {
		return nil
	}
	return pl.Orders.Front().Value.(*models.Order)
}

// UpdateTotal 更新总数量
func (pl *PriceLevel) UpdateTotal() {
	total := decimal.Zero
	for e := pl.Orders.Front(); e != nil; e = e.Next() {
		if order, ok := e.Value.(*models.Order); ok {
			total = total.Add(order.RemainAmount)
		}
	}
	pl.Total = total
}

// IsEmpty 是否为空
func (pl *PriceLevel) IsEmpty() bool {
	return pl.Orders.Len() == 0
}

// OrderBook 订单簿
type OrderBook struct {
	Symbol string

	// 买单（价格降序）
	Bids *RBTree
	// 卖单（价格升序）
	Asks *RBTree

	// 订单索引（快速查找）
	orders map[uint64]*OrderEntry

	mu sync.RWMutex
}

// OrderEntry 订单条目
type OrderEntry struct {
	Order     *models.Order
	PriceLevel *PriceLevel
	Element   *list.Element
}

// NewOrderBook 创建订单簿
func NewOrderBook(symbol string) *OrderBook {
	return &OrderBook{
		Symbol: symbol,
		Bids:   NewRBTree(true),  // 降序
		Asks:   NewRBTree(false), // 升序
		orders: make(map[uint64]*OrderEntry),
	}
}

// AddOrder 添加订单
func (ob *OrderBook) AddOrder(order *models.Order) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	var tree *RBTree
	if order.IsBuy() {
		tree = ob.Bids
	} else {
		tree = ob.Asks
	}

	// 获取或创建价格档位
	priceLevel := tree.Get(order.Price)
	if priceLevel == nil {
		priceLevel = NewPriceLevel(order.Price)
		tree.Insert(order.Price, priceLevel)
	}

	// 添加到价格档位
	element := priceLevel.Orders.PushBack(order)
	priceLevel.Total = priceLevel.Total.Add(order.RemainAmount)

	// 添加到索引
	ob.orders[order.ID] = &OrderEntry{
		Order:      order,
		PriceLevel: priceLevel,
		Element:    element,
	}
}

// RemoveOrder 移除订单
func (ob *OrderBook) RemoveOrder(orderID uint64) *models.Order {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	entry, exists := ob.orders[orderID]
	if !exists {
		return nil
	}

	// 从价格档位移除
	entry.PriceLevel.Orders.Remove(entry.Element)
	entry.PriceLevel.Total = entry.PriceLevel.Total.Sub(entry.Order.RemainAmount)

	// 如果价格档位为空，从树中删除
	if entry.PriceLevel.IsEmpty() {
		if entry.Order.IsBuy() {
			ob.Bids.Delete(entry.Order.Price)
		} else {
			ob.Asks.Delete(entry.Order.Price)
		}
	}

	// 从索引中删除
	delete(ob.orders, orderID)

	return entry.Order
}

// GetOrder 获取订单
func (ob *OrderBook) GetOrder(orderID uint64) *models.Order {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	if entry, exists := ob.orders[orderID]; exists {
		return entry.Order
	}
	return nil
}

// GetBestBid 获取最优买价
func (ob *OrderBook) GetBestBid() *PriceLevel {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	return ob.Bids.Max()
}

// GetBestAsk 获取最优卖价
func (ob *OrderBook) GetBestAsk() *PriceLevel {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	return ob.Asks.Min()
}

// GetDepth 获取盘口深度
func (ob *OrderBook) GetDepth(limit int) *models.Depth {
	ob.mu.RLock()
	defer ob.mu.RUnlock()

	depth := &models.Depth{
		Symbol: ob.Symbol,
		Bids:   make([]*models.DepthLevel, 0, limit),
		Asks:   make([]*models.DepthLevel, 0, limit),
	}

	// 获取买单深度
	count := 0
	ob.Bids.Ascend(func(price decimal.Decimal, level *PriceLevel) bool {
		if count >= limit {
			return false
		}
		depth.Bids = append(depth.Bids, &models.DepthLevel{
			Price:  price,
			Amount: level.Total,
		})
		count++
		return true
	})

	// 获取卖单深度
	count = 0
	ob.Asks.Ascend(func(price decimal.Decimal, level *PriceLevel) bool {
		if count >= limit {
			return false
		}
		depth.Asks = append(depth.Asks, &models.DepthLevel{
			Price:  price,
			Amount: level.Total,
		})
		count++
		return true
	})

	return depth
}

// UpdateOrder 更新订单
func (ob *OrderBook) UpdateOrder(order *models.Order) {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	entry, exists := ob.orders[order.ID]
	if !exists {
		return
	}

	// 更新订单
	entry.Order = order

	// 更新价格档位总数量
	entry.PriceLevel.UpdateTotal()
}

// GetOrderCount 获取订单数量
func (ob *OrderBook) GetOrderCount() int {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return len(ob.orders)
}

// GetBidCount 获取买单数量
func (ob *OrderBook) GetBidCount() int {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return ob.Bids.Size()
}

// GetAskCount 获取卖单数量
func (ob *OrderBook) GetAskCount() int {
	ob.mu.RLock()
	defer ob.mu.RUnlock()
	return ob.Asks.Size()
}
