package statemachine

import "github.com/llboyfy/MiniRaftDB/pkg/raftpb"

type StateMachine interface {
	Apply(entry *raftpb.RaftLogEntry) (interface{}, error)
	Snapshot() ([]byte, error)
	Restore(snapshot []byte) error
	Read(op []byte) (interface{}, error)
}
