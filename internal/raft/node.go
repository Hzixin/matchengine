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

// RaftNode Raft节点
type RaftNode struct {
	raft      *raft.Raft
	fsm       *MatchStateMachine
	cfg       *config.RaftConfig
	logger    *zap.Logger
	nodeID    string
	isLeader  bool
	mu        sync.RWMutex
}

// MatchStateMachine 撮合状态机
type MatchStateMachine struct {
	// 订单簿快照
	orderBooks map[string][]byte

	// 最新成交价
	lastPrices map[string]string

	// 成交序列号
	sequence uint64

	mu sync.RWMutex
}

// NewMatchStateMachine 创建状态机
func NewMatchStateMachine() *MatchStateMachine {
	return &MatchStateMachine{
		orderBooks: make(map[string][]byte),
		lastPrices: make(map[string]string),
	}
}

// Apply 应用日志
func (f *MatchStateMachine) Apply(log *raft.Log) interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()

	var cmd Command
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		return err
	}

	switch cmd.Type {
	case CommandTypeOrder:
		// 应用订单到状态机
		// 这里简化处理，实际应该重放订单
		f.sequence++
		return f.sequence

	case CommandTypeSnapshot:
		// 应用快照
		var snapshot StateSnapshot
		if err := json.Unmarshal(cmd.Data, &snapshot); err != nil {
			return err
		}
		f.orderBooks = snapshot.OrderBooks
		f.lastPrices = snapshot.LastPrices
		f.sequence = snapshot.Sequence
		return nil
	}

	return nil
}

// Snapshot 创建快照
func (f *MatchStateMachine) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return &MatchSnapshot{
		orderBooks: f.orderBooks,
		lastPrices: f.lastPrices,
		sequence:   f.sequence,
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

	f.orderBooks = snapshot.OrderBooks
	f.lastPrices = snapshot.LastPrices
	f.sequence = snapshot.Sequence

	return nil
}

// MatchSnapshot 快照
type MatchSnapshot struct {
	orderBooks map[string][]byte
	lastPrices map[string]string
	sequence   uint64
}

// Persist 持久化快照
func (s *MatchSnapshot) Persist(sink raft.SnapshotSink) error {
	err := json.NewEncoder(sink).Encode(&StateSnapshot{
		OrderBooks: s.orderBooks,
		LastPrices: s.lastPrices,
		Sequence:   s.sequence,
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
	OrderBooks map[string][]byte `json:"order_books"`
	LastPrices map[string]string `json:"last_prices"`
	Sequence   uint64            `json:"sequence"`
}

// OrderCommand 订单命令
type OrderCommand struct {
	Order *models.Order `json:"order"`
}

// NewRaftNode 创建Raft节点
func NewRaftNode(cfg *config.RaftConfig, logger *zap.Logger) (*RaftNode, error) {
	fsm := NewMatchStateMachine()

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
		raft:   r,
		fsm:    fsm,
		cfg:    cfg,
		logger: logger,
		nodeID: fmt.Sprintf("node%d", cfg.NodeID),
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
			n.isLeader = isLeader
			n.mu.Unlock()

			if isLeader {
				n.logger.Info("became leader", zap.String("node_id", n.nodeID))
			} else {
				n.logger.Info("lost leadership", zap.String("node_id", n.nodeID))
			}
		}
	}
}

// IsLeader 是否是Leader
func (n *RaftNode) IsLeader() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.isLeader
}

// ProposeOrder 提交订单
func (n *RaftNode) ProposeOrder(order *models.Order) error {
	cmd := Command{
		Type: CommandTypeOrder,
	}

	data, err := json.Marshal(OrderCommand{Order: order})
	if err != nil {
		return err
	}
	cmd.Data = data

	cmdData, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

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
