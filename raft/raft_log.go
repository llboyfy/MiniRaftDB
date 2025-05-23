package raft

func (rn *RaftNode) getLastLogInfo() (lastLogIndex uint64, lastLogTerm uint64) {
	if len(rn.log) == 0 {
		return 0, 0
	}
	lastEntry := &rn.log[len(rn.log)-1] // 使用指针，避免复制锁
	return lastEntry.LogIndex, lastEntry.LogTerm
}

func (rn *RaftNode) getLastLogIndex() uint64 {
	if len(rn.log) == 0 {
		return 0
	}
	lastEntry := &rn.log[len(rn.log)-1] // 使用指针
	return lastEntry.LogIndex
}
