package utils

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	nodeBits       uint8 = 10
	sequenceBits   uint8 = 12
	nodeMax        int64 = -1 ^ (-1 << nodeBits)
	sequenceMax    int64 = -1 ^ (-1 << sequenceBits)
	nodeShift      uint8 = sequenceBits
	timestampShift uint8 = sequenceBits + nodeBits
	epoch          int64 = 1704067200000 // 2024-01-01 00:00:00 UTC
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
func NewSnowflake(nodeID int64) *Snowflake {
	return &Snowflake{
		nodeID: nodeID,
	}
}

// Generate 生成唯一ID
func (s *Snowflake) Generate() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixMilli()

	if now == s.timestamp {
		s.sequence = (s.sequence + 1) & sequenceMax
		if s.sequence == 0 {
			for now <= s.timestamp {
				now = time.Now().UnixMilli()
			}
		}
	} else {
		s.sequence = 0
	}

	s.timestamp = now

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
