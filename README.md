# MatchEngine

一个高性能、高可用的数字货币撮合引擎，使用Go语言实现。

## 特性

- **高性能**: 红黑树+双向链表实现的订单簿，O(log N)插入，O(1)撮合
- **完整订单类型**: 支持限价单、市价单、止损单、冰山单、FOK、IOC
- **高可用**: 基于Raft协议的集群，支持Leader选举和故障切换
- **完整技术栈**: Kafka + Redis + MySQL
- **水平扩展**: 支持多交易对并发撮合

## 架构

```
┌─────────────────┐
│   API Gateway   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│     Kafka       │
│  (订单消息队列)   │
└────────┬────────┘
         │
         ▼
┌────────────────────────────────────────┐
│          撮合引擎集群 (Raft)             │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐ │
│  │ Leader  │  │Follower │  │Follower │ │
│  └────┬────┘  └─────────┘  └─────────┘ │
└───────┼────────────────────────────────┘
        │
┌───────┼───────────────────────┐
│       │                       │
▼       ▼                       ▼
┌─────────┐ ┌─────────┐  ┌─────────────┐
│  Redis  │ │  MySQL  │  │    Kafka    │
│ (订单簿) │ │ (持久化) │  │ (成交广播)  │
└─────────┘ └─────────┘  └─────────────┘
```

## 快速开始

### 环境要求

- Go 1.21+
- Docker & Docker Compose
- Make (可选)

### 使用Docker Compose启动

```bash
# 启动所有服务
cd scripts
docker-compose up -d

# 查看日志
docker-compose logs -f matchengine
```

### 本地开发

```bash
# 安装依赖
go mod download

# 启动基础设施
cd scripts
docker-compose up -d zookeeper kafka redis mysql

# 等待服务就绪
sleep 10

# 运行服务
go run cmd/matchengine/main.go -config configs/config.yaml
```

## API接口

### REST API

#### 创建订单
```bash
POST /api/v1/order
Content-Type: application/json

{
  "user_id": 1,
  "symbol": "BTC_USDT",
  "side": 1,
  "type": 1,
  "price": "50000.00",
  "amount": "0.1",
  "time_in_force": 1
}
```

#### 取消订单
```bash
DELETE /api/v1/order/BTC_USDT/123456789
```

#### 获取订单
```bash
GET /api/v1/order/BTC_USDT/123456789
```

#### 获取盘口深度
```bash
GET /api/v1/depth/BTC_USDT?limit=20
```

#### 获取行情
```bash
GET /api/v1/ticker/BTC_USDT
```

### gRPC API

gRPC服务监听在端口50051，proto定义见`proto/`目录。

## 配置说明

配置文件位于`configs/config.yaml`:

```yaml
server:
  grpc_port: 50051
  http_port: 8080

kafka:
  brokers:
    - localhost:9092
  order_topic: "matchengine-orders"
  trade_topic: "matchengine-trades"

redis:
  addr: "localhost:6379"

mysql:
  host: "localhost"
  port: 3306
  user: "root"
  password: "password"
  database: "matchengine"

markets:
  - symbol: "BTC_USDT"
    base_precision: 8
    quote_precision: 2
    min_amount: "0.001"
    price_tick: "0.01"
```

## 核心模块

### 订单簿 (OrderBook)

- 使用红黑树按价格排序
- 每个价格档位使用双向链表存储订单（FIFO）
- 时间复杂度：
  - 插入: O(log N)
  - 撮合: O(1)
  - 取消: O(1)

### 撮合算法

- **价格优先、时间优先** (Price-Time Priority)
- 支持多种订单类型:
  - 限价单 (Limit Order)
  - 市价单 (Market Order)
  - 止损限价单 (Stop Limit Order)
  - 止损市价单 (Stop Market Order)
  - 冰山单 (Iceberg Order)
  - FOK (Fill-Or-Kill)
  - IOC (Immediate-Or-Cancel)

### 高可用

- 基于Raft协议实现主从选举
- 日志复制保证数据一致性
- 自动故障切换

## 性能优化

- 无锁队列 (channel)
- 内存池 (sync.Pool)
- 批量处理
- 协程池

## 监控

Prometheus指标暴露在`/metrics`端点。

关键指标:
- `matchengine_orders_total`: 订单总数
- `matchengine_trades_total`: 成交总数
- `matchengine_orderbook_depth`: 订单簿深度
- `matchengine_matching_duration_seconds`: 撮合耗时

## 目录结构

```
matchengine/
├── cmd/matchengine/      # 入口
├── internal/
│   ├── config/           # 配置
│   ├── models/           # 数据模型
│   ├── orderbook/        # 订单簿
│   ├── matching/         # 撮合算法
│   ├── engine/           # 撮合引擎
│   ├── consumer/         # Kafka消费者
│   ├── producer/         # Kafka生产者
│   ├── storage/          # 存储层
│   ├── raft/             # Raft集群
│   └── server/           # API服务
├── pkg/                  # 公共库
├── proto/                # Protobuf定义
├── configs/              # 配置文件
└── scripts/              # 部署脚本
```

## License

MIT
