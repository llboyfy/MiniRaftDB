package raft

import (
	"context"
	"time"

	"github.com/llboyfy/MiniRaftDB/pkg/raftpb"
	"google.golang.org/grpc"
)

func (rn *RaftNode) RequestVote(ctx context.Context, req *raftpb.RequestVoteRequest, opts ...grpc.CallOption) (*raftpb.RequestVoteResponse, error) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	resp := &raftpb.RequestVoteResponse{
		Term:        rn.currentTerm,
		VoteGranted: false,
	}

	// 1. 如果请求任期小于自己，直接拒绝
	if req.Term < rn.currentTerm {
		return resp, nil
	}

	// 2. 如果请求任期更大，更新本地任期并转为Follower，重置投票
	if req.Term > rn.currentTerm {
		rn.currentTerm = req.Term
		rn.votedFor = 0 // 清空之前的投票
		rn.state = Follower
		// rn.persistState()
	}

	// 3. 如果已经投票且不是投给当前候选人，拒绝投票
	if rn.votedFor != 0 && rn.votedFor != req.CandidateID {
		return resp, nil
	}

	// 4. 检查候选人日志是否足够新（Raft论文 §5.4.1 关键判据）
	lastLogIndex, lastLogTerm := rn.getLastLogInfo()
	// 日志新旧判据：优先比term，term相等再比index
	if req.LastLogTerm < lastLogTerm || (req.LastLogTerm == lastLogTerm && req.LastLogIndex < lastLogIndex) {
		// 候选人日志落后，拒绝投票
		return resp, nil
	}

	// 5. 满足所有条件，投票
	rn.votedFor = req.CandidateID
	resp.VoteGranted = true

	// 6. 重置选举计时器，防止成为候选人
	rn.resetElectionTimer()

	// 7. 推荐持久化 votedFor 和 currentTerm（真实场景防崩溃丢失，见论文 §5.4.3）
	// rn.persistState()

	return resp, nil
}

// AppendEntries 处理来自 leader 的心跳和日志复制请求
func (rn *RaftNode) AppendEntries(ctx context.Context, req *raftpb.AppendEntriesRequest, opts ...grpc.CallOption) (*raftpb.AppendEntriesResponse, error) {
	// 1. 收到心跳或日志复制请求，重置选举定时器
	select {
	case rn.heartbeatCh <- struct{}{}:
	default:
	}

	// 2. 获取锁，保护共享状态
	rn.mu.Lock()
	defer rn.mu.Unlock()

	// 3. 如果请求任期落后于当前任期，拒绝
	if req.Term < rn.currentTerm {
		return &raftpb.AppendEntriesResponse{Term: rn.currentTerm, Success: false}, nil
	}

	// 4. 如果请求任期更大，更新本地任期，清空投票，并转为 Follower
	if req.Term > rn.currentTerm {
		rn.currentTerm = req.Term
		rn.becomeFollower() // 转为 Follower 状态
		// rn.persistState() // 可选：持久化 currentTerm 和 votedFor
	}
	// 保证处于 Follower，并重置选举定时器
	rn.resetElectionTimer()

	// 5. 日志一致性检查：分两种情况返回

	// 情况 A：PrevLogIndex 超出本地日志长度
	if req.PrevLogIndex > uint64(len(rn.log)) {
		return &raftpb.AppendEntriesResponse{
			Term:          rn.currentTerm,
			Success:       false,
			ConflictIndex: uint64(len(rn.log)) + 1, // 告诉 Leader 从这里重试
		}, nil
	}

	// 情况 B：PrevLogIndex 处 term 不匹配
	if req.PrevLogIndex > 0 && rn.log[req.PrevLogIndex-1].LogTerm != req.PrevLogTerm {
		conflictTerm := rn.log[req.PrevLogIndex-1].LogTerm
		// 找到该 term 在本地日志第一次出现的位置
		conflictIndex := uint64(1)
		for i, e := range rn.log {
			if e.LogTerm == conflictTerm {
				conflictIndex = uint64(i + 1)
				break
			}
		}
		return &raftpb.AppendEntriesResponse{
			Term:          rn.currentTerm,
			Success:       false,
			ConflictTerm:  conflictTerm,
			ConflictIndex: conflictIndex, // 告诉 Leader 回退到这个位置
		}, nil
	}

	// 否则匹配，继续执行追加操作…

	// 6. 删除冲突条目并追加新条目
	insertIndex := req.PrevLogIndex
	for i, entry := range req.Entries {
		pos := insertIndex + uint64(i)
		if pos-1 < uint64(len(rn.log)) {
			if rn.log[pos-1].LogTerm != entry.LogTerm {
				rn.log = rn.log[:pos-1]
				break
			}
		}
	}
	for _, entry := range req.Entries {
		rn.log = append(rn.log, entry)
		//rn.storage.AppendLog(entry)
	}

	// 7. 更新 lastLogIndex/lastLogTerm
	rn.lastLogIndex = rn.getLastLogIndex()
	if rn.lastLogIndex > 0 {
		rn.lastLogTerm = rn.log[rn.lastLogIndex-1].LogTerm
	}

	// 8. 更新 commitIndex = min(LeaderCommit, lastLogIndex)，并唤醒应用协程
	if req.LeaderCommit > rn.commitIndex {
		if req.LeaderCommit < rn.lastLogIndex {
			rn.commitIndex = req.LeaderCommit
		} else {
			rn.commitIndex = rn.lastLogIndex
		}
		rn.applyCond.Broadcast()
	}

	// 9. 返回成功
	return &raftpb.AppendEntriesResponse{
		Term:    rn.currentTerm,
		Success: true,
	}, nil
}

func (rn *RaftNode) requestVoteOnce(peerID uint64, req *raftpb.RequestVoteRequest) (bool, error) {
	client := rn.grpcClients[peerID]
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := client.RequestVote(ctx, req)
	if err != nil {
		return false, err
	}
	return resp.VoteGranted, nil
}

func (rn *RaftNode) sendAppendEntries(peerID uint64, req *raftpb.AppendEntriesRequest) (*raftpb.AppendEntriesResponse, error) {
	client := rn.grpcClients[peerID]
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := client.AppendEntries(ctx, req)
	return resp, err
}
