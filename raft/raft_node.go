package raft

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

type RaftNode struct {
	mu    sync.Mutex     // protects all following fields
	state RaftNodeStatus // node current role/state
	// Persistent state on all servers
	currentTerm uint64         // latest term server has seen
	votedFor    uint64         // candidateId that received vote in current term (0 means none)
	log         []RaftLogEntry // log entries; each entry contains command for state machine, and term when entry was received by leader

	// Volatile state on all servers
	commitIndex  uint64 // index of highest log entry known to be committed
	lastApplied  uint64 // index of highest log entry applied to state machine
	lastLogIndex uint64 // index of last log entry
	lastLogTerm  uint64 // term of last log entry

	// Volatile state on leaders (reinitialized after election)
	nextIndex  map[uint64]uint64 // for each server, index of the next log entry to send to that server
	matchIndex map[uint64]uint64 // for each server, index of highest log entry known to be replicated on server

	// Cluster information
	id    uint64            // this server's id
	peers map[uint64]string // map of peer id to network address

	// Timing
	electionTimeout  time.Duration
	heartbeatTimeout time.Duration
	electionTimer    *time.Timer
	heartbeatTimer   *time.Timer

	// Channels for internal coordination
	applyCh   chan RaftLogEntry // channel to apply committed log entries to state machine
	applyCond *sync.Cond        // condition variable for signaling log application
	// test
	stopCh      chan struct{} // signal to stop goroutines
	heartbeatCh chan struct{} // signal to trigger heartbeat immediately

	// Persistent storage interface
	storage Storage

	// TODO: Add networking fields (RPC clients/servers) here
}

func randTimeout(min, max time.Duration, defaults ...bool) time.Duration {
	if len(defaults) > 0 && defaults[0] {
		min = 150 * time.Millisecond
		max = 300 * time.Millisecond
	}
	return min + time.Duration(rand.Int63n(int64(max-min)))
}

func NewRaftNode(id uint64, peers map[uint64]string, applyCh chan RaftLogEntry, storage Storage) (*RaftNode, error) {
	rn := &RaftNode{
		id:               id,
		peers:            peers,
		state:            Follower,
		currentTerm:      0,
		votedFor:         0, // 0 表示未投票
		log:              make([]RaftLogEntry, 0),
		commitIndex:      0,
		lastApplied:      0,
		nextIndex:        make(map[uint64]uint64),
		matchIndex:       make(map[uint64]uint64),
		electionTimeout:  150 * time.Millisecond, // 也可以用随机函数增强防脑裂
		heartbeatTimeout: 50 * time.Millisecond,
		applyCh:          applyCh,
		stopCh:           make(chan struct{}),
		heartbeatCh:      make(chan struct{}, 1),
		storage:          storage,
	}

	// 初始化定时器，先不启动，避免立即触发
	rn.electionTimer = time.NewTimer(time.Hour * 24 * 365)
	rn.heartbeatTimer = time.NewTimer(time.Hour * 24 * 365)

	// 从持久化加载日志
	logs, err := storage.LoadLogs()
	if err != nil {
		return nil, fmt.Errorf("load logs failed: %v", err)
	}
	rn.log = logs

	// 初始化 lastLogIndex 和 lastLogTerm
	if len(rn.log) > 0 {
		lastEntry := rn.log[len(rn.log)-1]
		rn.lastLogIndex = lastEntry.LogIndex
		rn.lastLogTerm = lastEntry.LogTerm
	} else {
		rn.lastLogIndex = 0
		rn.lastLogTerm = 0
	}

	// 初始化 nextIndex 和 matchIndex
	for peerID := range peers {
		rn.nextIndex[peerID] = rn.lastLogIndex + 1
		rn.matchIndex[peerID] = 0
	}

	rn.applyCond = sync.NewCond(&rn.mu)

	return rn, nil
}

func (rn *RaftNode) resetElectionTimer() {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	if !rn.electionTimer.Stop() {
		select {
		case <-rn.electionTimer.C:
		default:
		}
	}
	rn.electionTimeout = randTimeout(0, 0, true) // 动态超时避免选举冲突
	rn.electionTimer.Reset(rn.electionTimeout)
}

func (rn *RaftNode) startElection() {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	rn.currentTerm++
	rn.state = Candidate
	rn.votedFor = rn.id
	// TODO: 实现 RequestVote 广播逻辑
	println("Node", rn.id, "starts election at term", rn.currentTerm)
}

func (rn *RaftNode) runElectionTimer() {
	for {
		select {
		case <-rn.electionTimer.C:
			// 选举超时处理，发起选举
			rn.startElection()
		case <-rn.heartbeatCh:
			// 收到心跳，重置选举定时器
			rn.resetElectionTimer()
		case <-rn.stopCh:
			return
		}
	}
}
