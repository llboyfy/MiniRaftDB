package raft

func (rn *RaftNode) getLastLogInfo() (lastLogIndex uint64, lastLogTerm uint64) {
	if len(rn.log) == 0 {
		return 0, 0
	}
	lastEntry := rn.log[len(rn.log)-1] // 使用指针，避免复制锁
	return lastEntry.LogIndex, lastEntry.LogTerm
}

func (rn *RaftNode) getLastLogIndex() uint64 {
	if len(rn.log) == 0 {
		return 0
	}
	lastEntry := rn.log[len(rn.log)-1] // 使用指针
	return lastEntry.LogIndex
}

func (rn *RaftNode) findLastIndexOfTerm(targetTerm uint64) uint64 {
	// 从日志末尾向前扫描
	for i := len(rn.log) - 1; i >= 0; i-- {
		if rn.log[i].LogTerm == targetTerm {
			return uint64(i + 1) // 日志索引是 1-based
		}
	}
	return 0
}
