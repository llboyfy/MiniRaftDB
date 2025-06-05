// raft/base/statemachine/kv_statemachine.go

package statemachine

import (
	"encoding/json"
	"errors"
	"sync"

	"github.com/llboyfy/MiniRaftDB/pkg/raftpb"
)

// KVCommand 用于描述 put/delete 操作
type KVCommand struct {
	Op    string `json:"op"` // "put" or "delete"
	Key   string `json:"key"`
	Value string `json:"value,omitempty"` // put 时需要
}

// KVStateMachine 实现 StateMachine 接口
type KVStateMachine struct {
	mu    sync.RWMutex
	store map[string]string
}

// NewKVStateMachine 构造函数
func NewKVStateMachine() *KVStateMachine {
	return &KVStateMachine{
		store: make(map[string]string),
	}
}

// Apply 应用一条 raft 日志
func (sm *KVStateMachine) Apply(entry *raftpb.RaftLogEntry) (interface{}, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	var cmd KVCommand
	if err := json.Unmarshal(entry.Data, &cmd); err != nil {
		return nil, err
	}
	switch cmd.Op {
	case "put":
		sm.store[cmd.Key] = cmd.Value
		return cmd.Value, nil
	case "delete":
		delete(sm.store, cmd.Key)
		return nil, nil
	default:
		return nil, errors.New("unknown op: " + cmd.Op)
	}
}

// Snapshot 导出当前 store 的快照
func (sm *KVStateMachine) Snapshot() ([]byte, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return json.Marshal(sm.store)
}

// Restore 用快照数据恢复 store
func (sm *KVStateMachine) Restore(snapshot []byte) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return json.Unmarshal(snapshot, &sm.store)
}

// Read 一致性读接口（只支持 get 操作）
func (sm *KVStateMachine) Read(op []byte) (interface{}, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	var cmd KVCommand
	if err := json.Unmarshal(op, &cmd); err != nil {
		return nil, err
	}
	if cmd.Op != "get" {
		return nil, errors.New("unsupported read op: " + cmd.Op)
	}
	val, ok := sm.store[cmd.Key]
	if !ok {
		return nil, errors.New("key not found")
	}
	return val, nil
}
