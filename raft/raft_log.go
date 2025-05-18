package raft

type RaftLogEntry struct {
	LogIndex      uint64             // Log index, strictly increasing and unique
	LogTerm       uint64             // Term number for Raft consistency
	EntryType     RaftLogEntryType   // Protocol-level log entry type (data, txn begin/commit/rollback, config change)
	OperationType RaftLogCommandType // Business operation type (set, delete)
	TransactionID string             // Transaction ID, empty if not part of a transaction
	DataKey       string             // Business key
	DataValue     []byte             // Business value
}
