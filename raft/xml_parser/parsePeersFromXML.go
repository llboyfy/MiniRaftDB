package raft

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
)

type Node struct {
	ID      uint64 `xml:"id,attr"`
	Address string `xml:"address,attr"`
}
type Cluster struct {
	Nodes []Node `xml:"node"`
}

// 传入相对路径（如 "./cluster.xml"），自动定位并解析
func ParsePeersFromXML(xmlRelPath string) (map[uint64]string, error) {
	// 获取当前工作目录
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working dir error: %w", err)
	}
	absPath := xmlRelPath
	if !filepath.IsAbs(xmlRelPath) {
		absPath = filepath.Join(wd, xmlRelPath)
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read xml file %s failed: %w", absPath, err)
	}
	var cluster Cluster
	if err := xml.Unmarshal(data, &cluster); err != nil {
		return nil, fmt.Errorf("xml unmarshal failed: %w", err)
	}
	peers := make(map[uint64]string)
	for _, n := range cluster.Nodes {
		peers[n.ID] = n.Address
	}
	return peers, nil
}
