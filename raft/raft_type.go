package raft

import (
	"bufio"
	"os"
	"sync"
)

type FileStorage struct {
	filePath string
	mu       sync.Mutex
	file     *os.File
	writer   *bufio.Writer
}
