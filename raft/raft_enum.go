package raft

// RaftLogCommandType represents the business operation type for a Raft log entry.
type RaftLogCommandType int

const (
	// RaftLogCommandSet represents a set (write/update) operation.
	RaftLogCommandSet RaftLogCommandType = iota

	// RaftLogCommandDelete represents a delete operation.
	RaftLogCommandDelete
)

// RaftLogEntryType represents the protocol-level category of a Raft log entry.
type RaftLogEntryType int

const (
	// LogEntryTypeData indicates a standard data log entry (e.g., a normal key-value write operation).
	LogEntryTypeData RaftLogEntryType = iota

	// LogEntryTypeTxnBegin marks the beginning of a distributed transaction.
	LogEntryTypeTxnBegin

	// LogEntryTypeTxnCommit marks the successful commit point of a distributed transaction.
	LogEntryTypeTxnCommit

	// LogEntryTypeTxnRollback marks the rollback or abort of a distributed transaction.
	LogEntryTypeTxnRollback

	// LogEntryTypeConfigChange indicates a configuration change entry, such as adding or removing nodes.
	LogEntryTypeConfigChange
)

// RaftLogEntry represents a single log entry in the Raft consensus protocol.
// It includes protocol-level metadata and business-level operation data.

type RaftNodeStatus int

const (
	Follower RaftNodeStatus = iota
	Candidate
	Leader
	Shutdown
)
