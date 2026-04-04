package raft

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	"github.com/hzx/matchengine/internal/config"
	"github.com/hzx/matchengine/internal/models"
	"go.uber.org/zap"
)

// OrderApplier 订单应用接口
// 由engine实现，状态机调用此接口来真正执行订单
type OrderApplier interface {
	ApplyOrder(order *models.Order) error
	GetAllOrders() (map[string][]*models.Order, error) // 获取所有订单（用于快照）
	RestoreOrders(orders map[string][]*models.Order) error // 恢复订单（从快照）
}

// RaftNode Raft节点
type RaftNode struct {
	raft      *raft.Raft
	fsm       *MatchStateMachine
	cfg       *config.RaftConfig
	logger    *zap.Logger
	nodeID    string
	applier   OrderApplier
	mu        sync.RWMutex
}

// MatchStateMachine 撮合状态机
type MatchStateMachine struct {
	applier  OrderApplier
	sequence uint64
	mu       sync.RWMutex
}

// NewMatchStateMachine 创建状态机
func NewMatchStateMachine(applier OrderApplier) *MatchStateMachine {
	return &MatchStateMachine{
		applier: applier,
	}
}

// Apply 应用日志 - Raft共识成功后会调用此方法
func (f *MatchStateMachine) Apply(log *raft.Log) interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()

	var cmd Command
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		return err
	}

	switch cmd.Type {
	case CommandTypeOrder:
		// 解析订单
		var orderCmd OrderCommand
		if err := json.Unmarshal(cmd.Data, &orderCmd); err != nil {
			return err
		}

		// 真正执行订单！
		if f.applier != nil {
			if err := f.applier.ApplyOrder(orderCmd.Order); err != nil {
				return err
			}
		}

		f.sequence++
		return f.sequence

	case CommandTypeSnapshot:
		// 应用快照
		var snapshot StateSnapshot
		if err := json.Unmarshal(cmd.Data, &snapshot); err != nil {
			return err
		}

		// 恢复订单
		if f.applier != nil {
			if err := f.applier.RestoreOrders(snapshot.Orders); err != nil {
				return err
			}
		}

		f.sequence = snapshot.Sequence
		return nil
	}

	return nil
}

// Snapshot 创建快照
func (f *MatchStateMachine) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// 获取所有订单
	orders := make(map[string][]*models.Order)
	if f.applier != nil {
		var err error
		orders, err = f.applier.GetAllOrders()
		if err != nil {
			return nil, err
		}
	}

	return &MatchSnapshot{
		orders:   orders,
		sequence: f.sequence,
	}, nil
}

// Restore 恢复快照
func (f *MatchStateMachine) Restore(rc io.ReadCloser) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	defer rc.Close()

	var snapshot StateSnapshot
	if err := json.NewDecoder(rc).Decode(&snapshot); err != nil {
		return err
	}

	// 恢复订单
	if f.applier != nil {
		if err := f.applier.RestoreOrders(snapshot.Orders); err != nil {
			return err
		}
	}

	f.sequence = snapshot.Sequence
	return nil
}

// MatchSnapshot 快照
type MatchSnapshot struct {
	orders   map[string][]*models.Order
	sequence uint64
}

// Persist 持久化快照
func (s *MatchSnapshot) Persist(sink raft.SnapshotSink) error {
	err := json.NewEncoder(sink).Encode(&StateSnapshot{
		Orders:   s.orders,
		Sequence: s.sequence,
	})
	if err != nil {
		sink.Cancel()
		return err
	}
	return sink.Close()
}

// Release 释放资源
func (s *MatchSnapshot) Release() {}

// Command 命令
type Command struct {
	Type int             `json:"type"`
	Data json.RawMessage `json:"data"`
}

const (
	CommandTypeOrder = iota + 1
	CommandTypeSnapshot
)

// StateSnapshot 状态快照
type StateSnapshot struct {
	Orders   map[string][]*models.Order `json:"orders"`
	Sequence uint64                     `json:"sequence"`
}

// OrderCommand 订单命令
type OrderCommand struct {
	Order *models.Order `json:"order"`
}

// NewRaftNode 创建Raft节点
func NewRaftNode(cfg *config.RaftConfig, logger *zap.Logger, applier OrderApplier) (*RaftNode, error) {
	fsm := NewMatchStateMachine(applier)

	nodeID := fmt.Sprintf("node-%d", cfg.NodeID)

	// Raft配置
	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(nodeID)
	raftConfig.SnapshotInterval = 30 * time.Second
	raftConfig.SnapshotThreshold = 1000
	raftConfig.HeartbeatTimeout = 1 * time.Second
	raftConfig.ElectionTimeout = 1 * time.Second
	raftConfig.LeaderLeaseTimeout = 1 * time.Second
	raftConfig.CommitTimeout = 50 * time.Millisecond

	// 创建传输层
	addr := fmt.Sprintf(":%d", 7000+cfg.NodeID)
	transport, err := raft.NewTCPTransport(addr, nil, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return nil, err
	}

	// 创建快照存储
	snapshots, err := raft.NewFileSnapshotStore(cfg.DataDir, 3, os.Stderr)
	if err != nil {
		return nil, err
	}

	// 创建日志存储
	logStore := raft.NewInmemStore()
	stableStore := raft.NewInmemStore()

	// 创建Raft实例
	r, err := raft.NewRaft(raftConfig, fsm, logStore, stableStore, snapshots, transport)
	if err != nil {
		return nil, err
	}

	node := &RaftNode{
		raft:    r,
		fsm:     fsm,
		cfg:     cfg,
		logger:  logger,
		nodeID:  nodeID,
		applier: applier,
	}

	// 监听Leader变化
	go node.watchLeader()

	// 启动集群
	if err := node.bootstrapCluster(); err != nil {
		logger.Warn("bootstrap cluster failed", zap.Error(err))
	}

	return node, nil
}

// bootstrapCluster 启动集群
func (n *RaftNode) bootstrapCluster() error {
	servers := make([]raft.Server, len(n.cfg.Peers))
	for i, peer := range n.cfg.Peers {
		servers[i] = raft.Server{
			ID:      raft.ServerID(fmt.Sprintf("node%d", i+1)),
			Address: raft.ServerAddress(peer),
		}
	}

	configuration := raft.Configuration{
		Servers: servers,
	}

	return n.raft.BootstrapCluster(configuration).Error()
}

// watchLeader 监听Leader变化
func (n *RaftNode) watchLeader() {
	for {
		select {
		case isLeader := <-n.raft.LeaderCh():
			n.mu.Lock()
			n.logger.Info("leadership changed",
				zap.Bool("is_leader", isLeader),
				zap.String("node_id", n.nodeID))
			n.mu.Unlock()
		}
	}
}

// IsLeader 是否是Leader
func (n *RaftNode) IsLeader() bool {
	return n.raft.State() == raft.Leader
}

// ProposeOrder 提交订单到Raft共识
func (n *RaftNode) ProposeOrder(order *models.Order) error {
	// 包装成命令
	cmd := Command{
		Type: CommandTypeOrder,
	}

	orderCmd := OrderCommand{Order: order}
	data, err := json.Marshal(orderCmd)
	if err != nil {
		return err
	}
	cmd.Data = data

	cmdData, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	// 提交到Raft，等待共识完成
	future := n.raft.Apply(cmdData, 5*time.Second)
	return future.Error()
}

// GetState 获取状态
func (n *RaftNode) GetState() raft.RaftState {
	return n.raft.State()
}

// LeaderCh 获取Leader变化通道
func (n *RaftNode) LeaderCh() <-chan bool {
	return n.raft.LeaderCh()
}

// Shutdown 关闭节点
func (n *RaftNode) Shutdown() error {
	return n.raft.Shutdown().Error()
}

// GetSequence 获取当前序列号
func (n *RaftNode) GetSequence() uint64 {
	n.fsm.mu.RLock()
	defer n.fsm.mu.RUnlock()
	return n.fsm.sequence
}
