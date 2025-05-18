package raft

import (
	"math/rand"
	"sync"
	"time"
)

type RaftNode struct {
	mu          sync.Mutex
	id          uint64         // 本节点唯一标识
	peers       map[int]string // 集群中其他节点的地址列表
	state       RaftNodeStatus // 节点当前状态（Follower/Candidate/Leader/Shutdown）
	currentTerm uint64         // 当前任期号
	votedFor    uint64         // 当前任期内投票给的候选人ID，0表示未投票
	log         []RaftLogEntry // 该节点维护的日志条目列表
	commitIndex uint64         // 已提交的最高日志索引
	lastApplied uint64         // 已应用到状态机的最高日志索引

	nextIndex  map[uint64]uint64 // Leader对每个Follower的下一个待发送日志索引
	matchIndex map[uint64]uint64 // Leader对每个Follower已复制的最高日志索引

	electionTimeout  time.Duration // 选举超时时间
	heartbeatTimeout time.Duration // 心跳超时时间
	electionTimer    *time.Timer   // 选举计时器
	heartbeatTimer   *time.Timer   // 心跳计时器

	applyCh chan RaftLogEntry // 日志提交到状态机的通道

	// 本地测试
	stopCh      chan struct{}
	heartbeatCh chan struct{}

	// TODO: 网络通信相关通道或接口（RPC、消息等）
}

func randTimeout(min, max time.Duration, defaults ...bool) time.Duration {
	if len(defaults) > 0 && defaults[0] {
		min = 150 * time.Millisecond
		max = 300 * time.Millisecond
	}
	return min + time.Duration(rand.Int63n(int64(max-min)))
}

func NewRaftNode(id uint64, peers map[int]string, applyCh chan RaftLogEntry) *RaftNode {
	return &RaftNode{
		id:               id,
		peers:            peers,
		state:            Follower,
		currentTerm:      0,
		votedFor:         0, // 0 表示未投票，与你设计保持一致
		log:              make([]RaftLogEntry, 0),
		commitIndex:      0,
		lastApplied:      0,
		nextIndex:        make(map[uint64]uint64),
		matchIndex:       make(map[uint64]uint64),
		electionTimeout:  150 * time.Millisecond, // 可根据需要改成随机超时
		heartbeatTimeout: 50 * time.Millisecond,  // 常规心跳间隔
		electionTimer:    nil,                    // 后续启动时创建
		heartbeatTimer:   nil,                    // 后续启动时创建
		applyCh:          applyCh,
		stopCh:           make(chan struct{}),
		heartbeatCh:      make(chan struct{}, 1), // 带缓冲防止阻塞
	}
}
