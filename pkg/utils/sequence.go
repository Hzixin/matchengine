package utils

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	nodeBits       uint8 = 10
	sequenceBits   uint8 = 12
	nodeMax        int64 = -1 ^ (-1 << nodeBits)     // 节点ID最大值 1023
	sequenceMax    int64 = -1 ^ (-1 << sequenceBits) // 序列号最大值 4095
	nodeShift      uint8 = sequenceBits              // 12
	timestampShift uint8 = sequenceBits + nodeBits   // 22
	epoch          int64 = 1704067200000             // 2024-01-01 00:00:00 UTC
)

// Snowflake 雪花算法ID生成器
// 结构: 1位符号位 + 41位时间戳 + 10位节点ID + 12位序列号
type Snowflake struct {
	mu        sync.Mutex
	timestamp int64
	nodeID    int64
	sequence  int64
}

// NewSnowflake 创建雪花算法生成器
// 传入 nodeID 必须在 0 ~ 1023 之间
func NewSnowflake(nodeID int64) *Snowflake {
	// 新增：节点ID合法性校验，防止越界导致ID冲突
	if nodeID < 0 || nodeID > nodeMax {
		panic("nodeID must be between 0 and 1023")
	}
	return &Snowflake{
		nodeID: nodeID,
	}
}

// Generate 生成唯一ID
func (s *Snowflake) Generate() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixMilli()

	// 修复：防止系统时间回拨导致ID重复/乱序
	if now < s.timestamp {
		panic("clock moved backwards, refusing to generate id")
	}

	if now == s.timestamp {
		// 同一毫秒内，序列号自增
		s.sequence = (s.sequence + 1) & sequenceMax
		// 序列号耗尽，等待下一毫秒
		if s.sequence == 0 {
			for now <= s.timestamp {
				now = time.Now().UnixMilli()
			}
		}
	} else {
		// 新毫秒，重置序列号
		s.sequence = 0
	}

	s.timestamp = now

	// 位运算生成ID
	id := ((now - epoch) << timestampShift) |
		(s.nodeID << nodeShift) |
		s.sequence

	return uint64(id)
}

// UUID 生成UUID
func UUID() string {
	return uuid.New().String()
}

// Timestamp 获取当前时间戳（毫秒）
func Timestamp() int64 {
	return time.Now().UnixMilli()
}

// TimestampSec 获取当前时间戳（秒）
func TimestampSec() int64 {
	return time.Now().Unix()
}
