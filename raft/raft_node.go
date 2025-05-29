package raft

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/llboyfy/MiniRaftDB/pkg/raftpb"
	"github.com/llboyfy/MiniRaftDB/raft/base"
	"google.golang.org/grpc"
)

type RaftNode struct {
	mu    sync.Mutex     // protects all following fields
	state RaftNodeStatus // node current role/state
	// Persistent state on all servers
	currentTerm uint64                 // latest term server has seen
	votedFor    uint64                 // candidateId that received vote in current term (0 means none)
	log         []*raftpb.RaftLogEntry // log entries; each entry contains command for state machine, and term when entry was received by leader

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
	applyCh   chan raftpb.RaftLogEntry // channel to apply committed log entries to state machine
	applyCond *sync.Cond               // condition variable for signaling log application
	// test
	stopCh      chan struct{} // signal to stop goroutines
	heartbeatCh chan struct{} // signal to trigger heartbeat immediately
	replicateCh chan struct{}

	// Persistent storage interface
	storage Storage

	grpcClients  map[uint64]raftpb.RaftServiceClient // gRPC客户端，key是Peer ID
	grpcServer   *grpc.Server                        // gRPC服务端实例
	grpcListener net.Listener                        // 监听器，绑定服务端端口
}

func NewRaftNode(id uint64, peers map[uint64]string, applyCh chan raftpb.RaftLogEntry, storage Storage) (*RaftNode, error) {
	rn := &RaftNode{
		id:               id,
		peers:            peers,
		state:            Follower,
		currentTerm:      0,
		votedFor:         0, // 0 表示未投票
		log:              make([]*raftpb.RaftLogEntry, 0),
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
	rn.electionTimeout = base.RandTimeout(0, 0, true) // 动态超时避免选举冲突
	rn.electionTimer.Reset(rn.electionTimeout)
}

func (rn *RaftNode) becomeCandidate() {
	rn.currentTerm++
	rn.state = Candidate
	rn.votedFor = rn.id
}

func (rn *RaftNode) becomeLeader() {
	rn.mu.Lock()
	rn.state = Leader
	// 初始化 leader 特有状态
	for peerID := range rn.peers {
		rn.nextIndex[peerID] = rn.getLastLogIndex() + 1
		rn.matchIndex[peerID] = 0
	}
	base.SafeClose(&rn.stopCh) // 停止选举定时器
	rn.stopCh = make(chan struct{})
	log.Printf("Node %d becomes Leader for term %d", rn.id, rn.currentTerm)
	rn.mu.Unlock()
	go rn.runHeartbeatTimer()

}

func (rn *RaftNode) becomeFollower() {
	rn.mu.Lock()
	rn.state = Follower
	rn.votedFor = 0
	base.SafeClose(&rn.stopCh) // 停止心跳
	rn.stopCh = make(chan struct{})
	log.Printf("Node %d becomes Follower for term %d", rn.id, rn.currentTerm)

	rn.mu.Unlock()
	go rn.runElectionTimer()
}

func (rn *RaftNode) startElection() {
	rn.mu.Lock()
	rn.becomeCandidate()
	lastLogIndex, lastLogTerm := rn.getLastLogInfo()
	currentTerm := rn.currentTerm
	rn.mu.Unlock()
	votes := rn.collectVotes(currentTerm, lastLogIndex, lastLogTerm)
	if votes > (len(rn.peers)+1)/2 {
		rn.becomeLeader()
	} else {
		rn.becomeFollower()
	}
}

func (rn *RaftNode) collectVotes(term, lastLogIndex, lastLogTerm uint64) int {
	voteCount := 1 // 自己先投票
	totalNodes := len(rn.peers) + 1

	voteCh := make(chan bool, len(rn.peers))

	req := &raftpb.RequestVoteRequest{
		Term:         term,
		CandidateID:  rn.id,
		LastLogIndex: lastLogIndex,
		LastLogTerm:  lastLogTerm,
	}

	for peerID, _ := range rn.peers {
		if peerID == rn.id {
			continue
		}
		go func(peerID uint64) {
			granted, err := rn.requestVoteOnce(peerID, req)
			if err != nil {
				log.Printf("RequestVote RPC to peer %d failed: %v", peerID, err)
				voteCh <- false
				return
			}
			if granted {
				log.Printf("Node %d voted for candidate %d in term %d", peerID, rn.id, term)
				voteCh <- true
			} else {
				log.Printf("Node %d did not vote for candidate %d in term %d", peerID, rn.id, term)
				voteCh <- false
			}
		}(peerID)
	}

	timeout := time.After(300 * time.Millisecond)
	neededVotes := totalNodes/2 + 1

	for received := 1; received < totalNodes; received++ {
		select {
		case v := <-voteCh:
			if v {
				voteCount++
				if voteCount >= neededVotes {
					return voteCount // 提前返回
				}
			}
		case <-timeout:
			return voteCount
		}
	}
	return voteCount
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

func (rn *RaftNode) runHeartbeatTimer() {
	ticker := time.NewTicker(rn.heartbeatTimeout)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rn.sendHeartbeats()
		case <-rn.stopCh:
			return
		}
	}
}

func (rn *RaftNode) sendHeartbeats() {
	rn.mu.Lock()
	if rn.state != Leader {
		rn.mu.Unlock()
		return
	}
	currentTerm := rn.currentTerm
	commitIndex := rn.commitIndex
	peers := make([]uint64, 0, len(rn.peers))
	for peerID := range rn.peers {
		if peerID != rn.id {
			peers = append(peers, peerID)
		}
	}
	rn.mu.Unlock()

	for _, peerID := range peers {
		go rn.sendHeartbeatToPeer(peerID, currentTerm, commitIndex)
	}
}

func (rn *RaftNode) sendHeartbeatToPeer(peerID uint64, currentTerm, commitIndex uint64) {
	req := rn.buildAppendEntriesRequest(peerID, currentTerm, commitIndex)
	resp, err := rn.sendAppendEntries(peerID, req)
	rn.handleAppendEntriesResponse(peerID, req, resp, err)
}

func (rn *RaftNode) buildAppendEntriesRequest(peerID, currentTerm, commitIndex uint64) *raftpb.AppendEntriesRequest {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	prevLogIndex := rn.nextIndex[peerID] - 1
	prevLogTerm := uint64(0)
	if prevLogIndex > 0 && int(prevLogIndex) <= len(rn.log) {
		if entry := rn.log[prevLogIndex-1]; entry != nil {
			prevLogTerm = entry.LogTerm
		}
	}
	var entries []*raftpb.RaftLogEntry
	for i := prevLogIndex; i < uint64(len(rn.log)); i++ {
		if rn.log[i] != nil {
			entries = append(entries, rn.log[i])
		}
	}

	return &raftpb.AppendEntriesRequest{
		Term:         currentTerm,
		LeaderID:     rn.id,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  prevLogTerm,
		Entries:      entries,
		LeaderCommit: commitIndex,
	}
}

func (rn *RaftNode) handleAppendEntriesResponse(peerID uint64, req *raftpb.AppendEntriesRequest, resp *raftpb.AppendEntriesResponse, err error) {
	if err != nil {
		// 日志/监控，什么都不做，等下一轮 tick 自动重试
		fmt.Printf("AppendEntries to peer %d failed: %v\n", peerID, err)
		return
	}
	if resp == nil {
		fmt.Printf("No response from peer %d\n", peerID)
		return
	}

	rn.mu.Lock()
	defer rn.mu.Unlock()

	if resp.Term > rn.currentTerm {
		rn.currentTerm = resp.Term
		rn.becomeFollower()
		return
	}
	if rn.state != Leader {
		return
	}
	if resp.Success {
		rn.matchIndex[peerID] = req.PrevLogIndex + uint64(len(req.Entries))
		rn.nextIndex[peerID] = rn.matchIndex[peerID] + 1
		// 可选：推进 commitIndex
	} else {
		if rn.nextIndex[peerID] > 1 {
			rn.nextIndex[peerID]--
		}
	}
}
