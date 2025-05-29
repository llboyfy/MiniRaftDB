package raft

import (
	"context"
	"time"

	"github.com/llboyfy/MiniRaftDB/pkg/raftpb"
)

func (rn *RaftNode) RequestVote(ctx context.Context, req *raftpb.RequestVoteRequest) (*raftpb.RequestVoteResponse, error) {
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

func (rn *RaftNode) AppendEntries(ctx context.Context, req *raftpb.AppendEntriesRequest) (*raftpb.AppendEntriesResponse, error) {
	// 1. 收到 leader 的心跳或日志复制，重置选举定时器
	select {
	case rn.heartbeatCh <- struct{}{}:
	default:
	}

	// 2. 对请求进行 term 检查、状态转换、日志一致性检查与写入、commitIndex 更新等
	rn.mu.Lock()
	defer rn.mu.Unlock()

	// 如果 leader 任期过旧，拒绝
	if req.Term < rn.currentTerm {
		return &raftpb.AppendEntriesResponse{
			Term:    rn.currentTerm,
			Success: false,
		}, nil
	}

	// 如果发现自己是 candidate 或 leader，降级为 follower
	if req.Term > rn.currentTerm {
		rn.currentTerm = req.Term
		rn.votedFor = 0
		rn.state = Follower
	}

	// 日志一致性检查：确认 prevLogIndex/prevLogTerm 匹配
	// 然后追加新条目，或者在冲突时删除冲突后的条目
	// 更新 commitIndex = min(req.LeaderCommit, len(rn.log))

	// 返回成功
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
