package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/hashicorp/raft"
	"github.com/hzx/matchengine/internal/models"
	"github.com/shopspring/decimal"
)

// MatchStateMachine 状态机
type MatchStateMachine struct {
	orders    map[uint64]*models.Order
	sequence  uint64
	mu        sync.RWMutex
}

// NewMatchStateMachine 创建状态机
func NewMatchStateMachine() *MatchStateMachine {
	return &MatchStateMachine{
		orders: make(map[uint64]*models.Order),
	}
}

// Apply 应用日志
func (f *MatchStateMachine) Apply(log *raft.Log) interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()

	var order models.Order
	if err := json.Unmarshal(log.Data, &order); err != nil {
		return err
	}

	f.orders[order.ID] = &order
	f.sequence++
	return f.sequence
}

// Snapshot 创建快照
func (f *MatchStateMachine) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return &MatchSnapshot{
		orders:   f.orders,
		sequence: f.sequence,
	}, nil
}

// Restore 恢复快照
func (f *MatchStateMachine) Restore(rc io.ReadCloser) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	defer rc.Close()

	var snapshot MatchSnapshot
	if err := json.NewDecoder(rc).Decode(&snapshot); err != nil {
		return err
	}

	f.orders = snapshot.orders
	f.sequence = snapshot.sequence
	return nil
}

// GetOrders 获取订单
func (f *MatchStateMachine) GetOrders() map[uint64]*models.Order {
	f.mu.RLock()
	defer f.mu.RUnlock()
	result := make(map[uint64]*models.Order)
	for k, v := range f.orders {
		result[k] = v
	}
	return result
}

// GetSequence 获取序列号
func (f *MatchStateMachine) GetSequence() uint64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.sequence
}

// MatchSnapshot 快照
type MatchSnapshot struct {
	orders   map[uint64]*models.Order
	sequence uint64
}

// Persist 持久化快照
func (s *MatchSnapshot) Persist(sink raft.SnapshotSink) error {
	err := json.NewEncoder(sink).Encode(s)
	if err != nil {
		sink.Cancel()
		return err
	}
	return sink.Close()
}

// Release 释放资源
func (s *MatchSnapshot) Release() {}

// RaftNode Raft节点
type RaftNode struct {
	raft   *raft.Raft
	fsm    *MatchStateMachine
	nodeID string
}

// NewRaftNode 创建Raft节点
func NewRaftNode(nodeID, dataDir string, peers []string) (*RaftNode, error) {
	fsm := NewMatchStateMachine()

	// Raft配置
	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(nodeID)
	config.HeartbeatTimeout = 500 * time.Millisecond
	config.ElectionTimeout = 500 * time.Millisecond
	config.LeaderLeaseTimeout = 500 * time.Millisecond
	config.CommitTimeout = 50 * time.Millisecond
	config.SnapshotInterval = 10 * time.Second
	config.SnapshotThreshold = 10

	// 创建目录
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	// 日志存储
	logStore, err := raft.NewFileSnapshotStore(dataDir, 1, os.Stderr)
	if err != nil {
		return nil, err
	}

	// 传输层
	port := 7000 + parseNodeID(nodeID)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	transport, err := raft.NewTCPTransport(addr, nil, 3, 5*time.Second, os.Stderr)
	if err != nil {
		return nil, err
	}

	// 创建Raft实例
	r, err := raft.NewRaft(config, fsm, raft.NewInmemStore(), raft.NewInmemStore(), logStore, transport)
	if err != nil {
		return nil, err
	}

	// 配置集群
	servers := make([]raft.Server, len(peers))
	for i, peer := range peers {
		servers[i] = raft.Server{
			ID:      raft.ServerID(fmt.Sprintf("node%d", i+1)),
			Address: raft.ServerAddress(peer),
		}
	}

	r.BootstrapCluster(raft.Configuration{Servers: servers})

	return &RaftNode{
		raft:   r,
		fsm:    fsm,
		nodeID: nodeID,
	}, nil
}

func parseNodeID(nodeID string) uint64 {
	var id uint64
	fmt.Sscanf(nodeID, "node%d", &id)
	return id
}

// IsLeader 是否是Leader
func (n *RaftNode) IsLeader() bool {
	return n.raft.State() == raft.Leader
}

// GetLeaderAddr 获取Leader地址
func (n *RaftNode) GetLeaderAddr() string {
	return string(n.raft.Leader())
}

// SubmitOrder 提交订单
func (n *RaftNode) SubmitOrder(order *models.Order) error {
	data, err := json.Marshal(order)
	if err != nil {
		return err
	}

	future := n.raft.Apply(data, 5*time.Second)
	return future.Error()
}

// GetOrders 获取订单
func (n *RaftNode) GetOrders() map[uint64]*models.Order {
	return n.fsm.GetOrders()
}

// GetSequence 获取序列号
func (n *RaftNode) GetSequence() uint64 {
	return n.fsm.GetSequence()
}

// Shutdown 关闭
func (n *RaftNode) Shutdown() {
	n.raft.Shutdown()
}

func main() {
	fmt.Println("========================================")
	fmt.Println("   Raft集群故障切换测试")
	fmt.Println("========================================")
	fmt.Println()

	// 创建3个节点
	peers := []string{
		"127.0.0.1:7001",
		"127.0.0.1:7002",
		"127.0.0.1:7003",
	}

	nodes := make([]*RaftNode, 3)
	for i := 0; i < 3; i++ {
		nodeID := fmt.Sprintf("node%d", i+1)
		dataDir := fmt.Sprintf("/tmp/raft-test-%s", nodeID)
		os.RemoveAll(dataDir) // 清理旧数据

		node, err := NewRaftNode(nodeID, dataDir, peers)
		if err != nil {
			fmt.Printf("❌ 创建节点%d失败: %v\n", i+1, err)
			os.Exit(1)
		}
		nodes[i] = node
		fmt.Printf("✓ 节点%d启动成功 (端口: %s)\n", i+1, peers[i])
	}

	fmt.Println()
	fmt.Println("等待集群选举Leader...")

	// 等待Leader选举
	time.Sleep(3 * time.Second)

	// 找到Leader
	var leader *RaftNode
	var leaderIndex int
	for i, node := range nodes {
		if node.IsLeader() {
			leader = node
			leaderIndex = i
			break
		}
	}

	if leader == nil {
		fmt.Println("❌ 未选举出Leader")
		return
	}

	fmt.Printf("✓ Leader选举成功: 节点%d\n", leaderIndex+1)
	fmt.Println()

	// 提交一些订单
	fmt.Println("=== 提交订单测试 ===")
	for i := 1; i <= 5; i++ {
		order := models.NewOrder(
			uint64(i),
			1,
			"BTC_USDT",
			models.OrderSideBuy,
			models.OrderTypeLimit,
			decimal.RequireFromString("50000"),
			decimal.RequireFromString("1"),
			models.TimeInForceGTC,
		)

		if err := leader.SubmitOrder(order); err != nil {
			fmt.Printf("❌ 订单%d提交失败: %v\n", i, err)
		} else {
			fmt.Printf("✓ 订单%d提交成功\n", i)
		}
	}

	time.Sleep(500 * time.Millisecond)

	// 验证数据一致性
	fmt.Println()
	fmt.Println("=== 验证数据一致性 ===")
	for i, node := range nodes {
		orders := node.GetOrders()
		seq := node.GetSequence()
		fmt.Printf("节点%d: 订单数=%d, 序列号=%d\n", i+1, len(orders), seq)
	}

	// 模拟Leader故障
	fmt.Println()
	fmt.Println("=== 模拟Leader故障 ===")
	fmt.Printf("⏳ 关闭节点%d (Leader)...\n", leaderIndex+1)
	nodes[leaderIndex].Shutdown()
	time.Sleep(100 * time.Millisecond)
	fmt.Printf("✓ 节点%d已关闭\n", leaderIndex+1)

	fmt.Println()
	fmt.Println("等待新Leader选举...")

	// 等待新Leader
	var newLeader *RaftNode
	var newLeaderIndex int
	for retry := 0; retry < 10; retry++ {
		time.Sleep(500 * time.Millisecond)
		for i, node := range nodes {
			if i != leaderIndex && node.IsLeader() {
				newLeader = node
				newLeaderIndex = i
				break
			}
		}
		if newLeader != nil {
			break
		}
		fmt.Printf("  重试 %d/10...\n", retry+1)
	}

	if newLeader == nil {
		fmt.Println("❌ 新Leader选举失败")
	} else {
		fmt.Printf("✓ 新Leader选举成功: 节点%d\n", newLeaderIndex+1)
	}

	// 验证数据是否丢失
	fmt.Println()
	fmt.Println("=== 验证数据是否丢失 ===")
	for i, node := range nodes {
		if i == leaderIndex {
			fmt.Printf("节点%d: 已关闭\n", i+1)
			continue
		}
		orders := node.GetOrders()
		seq := node.GetSequence()
		fmt.Printf("节点%d: 订单数=%d, 序列号=%d\n", i+1, len(orders), seq)

		if len(orders) >= 5 {
			fmt.Printf("  ✅ 数据完整，无丢失!\n")
		}
	}

	// 继续提交订单（使用新Leader）
	fmt.Println()
	fmt.Println("=== 新Leader继续服务 ===")
	for i := 6; i <= 8; i++ {
		order := models.NewOrder(
			uint64(i),
			1,
			"BTC_USDT",
			models.OrderSideSell,
			models.OrderTypeLimit,
			decimal.RequireFromString("51000"),
			decimal.RequireFromString("1"),
			models.TimeInForceGTC,
		)

		if newLeader == nil {
			fmt.Printf("❌ 无可用Leader，订单%d提交失败\n", i)
			continue
		}

		if err := newLeader.SubmitOrder(order); err != nil {
			fmt.Printf("❌ 订单%d提交失败: %v\n", i, err)
		} else {
			fmt.Printf("✓ 订单%d提交成功\n", i)
		}
	}

	time.Sleep(500 * time.Millisecond)

	// 最终数据验证
	fmt.Println()
	fmt.Println("=== 最终数据验证 ===")
	for i, node := range nodes {
		if i == leaderIndex {
			continue
		}
		orders := node.GetOrders()
		seq := node.GetSequence()
		fmt.Printf("节点%d: 订单数=%d, 序列号=%d\n", i+1, len(orders), seq)
	}

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("   测试完成")
	fmt.Println("========================================")

	// 等待信号退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	// 清理
	for _, node := range nodes {
		node.Shutdown()
	}
}
