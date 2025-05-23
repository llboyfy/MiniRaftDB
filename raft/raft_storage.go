package raft

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/llboyfy/MiniRaftDB/pkg/raftpb"
)

// 持久化接口
type Storage interface {
	AppendLog(entry raftpb.RaftLogEntry) error
	LoadLogs() ([]raftpb.RaftLogEntry, error)
}

// 简单文件持久化实现

func NewFileStorage(filePath string) (*FileStorage, error) {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return &FileStorage{
		filePath: filePath,
		file:     file,
		writer:   bufio.NewWriter(file),
	}, nil
}

// 追加日志条目到文件
func (fs *FileStorage) AppendLog(entry raftpb.RaftLogEntry) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n') // 换行分割

	if _, err := fs.writer.Write(data); err != nil {
		return err
	}
	return fs.writer.Flush()
}

// 加载所有日志条目
func (fs *FileStorage) LoadLogs() ([]raftpb.RaftLogEntry, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if _, err := fs.file.Seek(0, 0); err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(fs.file)
	logs := make([]raftpb.RaftLogEntry, 0)
	for scanner.Scan() {
		var entry raftpb.RaftLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("failed to unmarshal log entry: %v", err)
		}
		logs = append(logs, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

// 关闭文件句柄
func (fs *FileStorage) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := fs.writer.Flush(); err != nil {
		return err
	}
	return fs.file.Close()
}
