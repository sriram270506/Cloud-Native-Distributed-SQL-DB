package raft

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hashicorp/raft"
)

type ApplyFunc func([]byte) error
type SnapshotFunc func() ([]byte, error)
type RestoreFunc func([]byte) error

type fsm struct {
	apply    ApplyFunc
	snapshot SnapshotFunc
	restore  RestoreFunc
}

func (f *fsm) Apply(entry *raft.Log) interface{} {
	if err := f.apply(entry.Data); err != nil {
		return err
	}
	return nil
}

func (f *fsm) Snapshot() (raft.FSMSnapshot, error) {
	data, err := f.snapshot()
	if err != nil {
		return nil, err
	}
	return snapshot{data: data}, nil
}

func (f *fsm) Restore(reader io.ReadCloser) error {
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	return f.restore(data)
}

type snapshot struct{ data []byte }

func (s snapshot) Persist(sink raft.SnapshotSink) error {
	if _, err := sink.Write(s.data); err != nil {
		_ = sink.Cancel()
		return err
	}
	return sink.Close()
}
func (s snapshot) Release() {}

type Node struct {
	ID        string
	Raft      *raft.Raft
	Transport *raft.InmemTransport
}
type Cluster struct {
	mu    sync.RWMutex
	nodes []*Node
}

func NewCluster(count int, dataDir string, apply []ApplyFunc, snapshots []SnapshotFunc, restores []RestoreFunc) (*Cluster, error) {
	if count < 1 {
		return nil, fmt.Errorf("cluster must contain at least one node")
	}
	if len(apply) != count || len(snapshots) != count || len(restores) != count {
		return nil, fmt.Errorf("callback count does not match node count")
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}
	transports := make([]*raft.InmemTransport, count)
	for i := range transports {
		_, transports[i] = raft.NewInmemTransport(raft.ServerAddress(fmt.Sprintf("node-%d", i+1)))
	}
	for i := range transports {
		for j := range transports {
			if i != j {
				transports[i].Connect(transports[j].LocalAddr(), transports[j])
			}
		}
	}
	servers := make([]raft.Server, count)
	for i := range servers {
		servers[i] = raft.Server{ID: raft.ServerID(fmt.Sprintf("node-%d", i+1)), Address: transports[i].LocalAddr()}
	}
	configuration := raft.Configuration{Servers: servers}
	cluster := &Cluster{}
	for i := 0; i < count; i++ {
		config := raft.DefaultConfig()
		config.LocalID = servers[i].ID
		config.LogOutput = io.Discard
		config.HeartbeatTimeout = 300 * time.Millisecond
		config.ElectionTimeout = 600 * time.Millisecond
		config.LeaderLeaseTimeout = 300 * time.Millisecond
		config.CommitTimeout = 50 * time.Millisecond
		snapshotStore, err := raft.NewFileSnapshotStore(filepath.Join(dataDir, string(servers[i].ID)), 2, io.Discard)
		if err != nil {
			return nil, err
		}
		instance, err := raft.NewRaft(config, &fsm{apply: apply[i], snapshot: snapshots[i], restore: restores[i]}, raft.NewInmemStore(), raft.NewInmemStore(), snapshotStore, transports[i])
		if err != nil {
			return nil, err
		}
		cluster.nodes = append(cluster.nodes, &Node{ID: string(servers[i].ID), Raft: instance, Transport: transports[i]})
	}
	for _, node := range cluster.nodes {
		if err := node.Raft.BootstrapCluster(configuration).Error(); err != nil && err != raft.ErrCantBootstrap {
			return nil, err
		}
	}
	return cluster, nil
}

func (c *Cluster) Leader() *Node {
	c.mu.RLock()
	defer c.mu.RUnlock()
	address := c.nodes[0].Raft.Leader()
	for _, node := range c.nodes {
		if node.Transport.LocalAddr() == address {
			return node
		}
	}
	return nil
}

func (c *Cluster) Apply(command []byte, timeout time.Duration) error {
	node := c.Leader()
	if node == nil {
		return fmt.Errorf("no Raft leader available")
	}
	future := node.Raft.Apply(command, timeout)
	if err := future.Error(); err != nil {
		return err
	}
	if result := future.Response(); result != nil {
		if err, ok := result.(error); ok {
			return err
		}
	}
	return nil
}

func (c *Cluster) Status() map[string]string {
	result := make(map[string]string)
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, node := range c.nodes {
		result[node.ID] = node.Raft.State().String()
	}
	return result
}
func (c *Cluster) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, node := range c.nodes {
		if err := node.Raft.Shutdown().Error(); err != nil {
			return err
		}
	}
	return nil
}
