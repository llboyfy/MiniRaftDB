package raft

// AppendEntriesRequest 表示 Raft 的追加日志请求
type AppendEntriesRequest struct {
	Term         uint64         // Leader 的任期号
	LeaderID     uint64         // Leader 的节点ID
	PrevLogIndex uint64         // 紧跟在新日志之前的日志索引
	PrevLogTerm  uint64         // PrevLogIndex 处日志的任期号
	Entries      []RaftLogEntry // 新日志条目（可能为空，表示心跳）
	LeaderCommit uint64         // Leader 已提交的最高日志索引
}

// AppendEntriesResponse 表示追加日志请求的响应
type AppendEntriesResponse struct {
	Term          uint64 // 当前节点的任期号，用于领导人更新自己的任期
	Success       bool   // 表示是否成功追加日志（匹配 PrevLogIndex 和 PrevLogTerm）
	ConflictIndex uint64 // （可选）冲突日志索引，便于快速回退
	ConflictTerm  uint64 // （可选）冲突日志任期，用于冲突检测
}

// RequestVoteRequest 表示 Raft 的请求投票请求
type RequestVoteRequest struct {
	Term         uint64 // 请求者的任期号
	CandidateID  uint64 // 请求者的节点ID
	LastLogIndex uint64 // 请求者最后日志的索引
	LastLogTerm  uint64 // 请求者最后日志的任期号
}

// RequestVoteResponse 表示请求投票的响应
type RequestVoteResponse struct {
	Term        uint64 // 当前节点的任期号，用于请求者更新任期
	VoteGranted bool   // 表示是否投票给请求者
}
