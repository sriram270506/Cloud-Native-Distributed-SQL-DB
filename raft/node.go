package raft

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
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

// Peer is one voting member of the static cluster configuration.
type Peer struct {
	ID       string `json:"id"`
	RaftAddr string `json:"raft_addr"`
	HTTPAddr string `json:"http_addr"`
}

type Config struct {
	ID            string
	RaftBind      string
	RaftAdvertise string
	DataDir       string
	Peers         []Peer
	Apply         ApplyFunc
	Snapshot      SnapshotFunc
	Restore       RestoreFunc
}

// Node is a single Raft member running in this process. Peers are reached over TCP.
type Node struct {
	ID    string
	raft  *raft.Raft
	logs  *raftboltdb.BoltStore
	peers map[string]Peer
}

func NewNode(cfg Config) (*Node, error) {
	if cfg.ID == "" {
		return nil, fmt.Errorf("node id is required")
	}
	peers := make(map[string]Peer, len(cfg.Peers))
	servers := make([]raft.Server, 0, len(cfg.Peers))
	for _, peer := range cfg.Peers {
		peers[peer.ID] = peer
		servers = append(servers, raft.Server{ID: raft.ServerID(peer.ID), Address: raft.ServerAddress(peer.RaftAddr)})
	}
	if _, ok := peers[cfg.ID]; !ok {
		return nil, fmt.Errorf("node %q is not in the peer list", cfg.ID)
	}
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return nil, err
	}

	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(cfg.ID)
	config.LogOutput = os.Stderr
	config.LogLevel = "WARN"

	advertise, err := resolveWithRetry(cfg.RaftAdvertise, 60*time.Second)
	if err != nil {
		return nil, err
	}
	transport, err := raft.NewTCPTransport(cfg.RaftBind, advertise, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return nil, err
	}
	// The BoltDB store keeps the Raft log, current term and vote on disk, which Raft safety requires.
	logs, err := raftboltdb.NewBoltStore(filepath.Join(cfg.DataDir, "raft.db"))
	if err != nil {
		transport.Close()
		return nil, err
	}
	snapshots, err := raft.NewFileSnapshotStore(cfg.DataDir, 2, os.Stderr)
	if err != nil {
		logs.Close()
		transport.Close()
		return nil, err
	}
	hasState, err := raft.HasExistingState(logs, logs, snapshots)
	if err != nil {
		logs.Close()
		transport.Close()
		return nil, err
	}
	instance, err := raft.NewRaft(config, &fsm{apply: cfg.Apply, snapshot: cfg.Snapshot, restore: cfg.Restore}, logs, logs, snapshots, transport)
	if err != nil {
		logs.Close()
		transport.Close()
		return nil, err
	}
	// Every member bootstraps with the same configuration; Raft ignores it once state exists.
	if !hasState {
		if err := instance.BootstrapCluster(raft.Configuration{Servers: servers}).Error(); err != nil && err != raft.ErrCantBootstrap {
			instance.Shutdown()
			logs.Close()
			return nil, err
		}
	}
	return &Node{ID: cfg.ID, raft: instance, logs: logs, peers: peers}, nil
}

// resolveWithRetry waits for DNS, since Kubernetes and Compose publish peer names shortly after start.
func resolveWithRetry(address string, timeout time.Duration) (*net.TCPAddr, error) {
	deadline := time.Now().Add(timeout)
	for {
		resolved, err := net.ResolveTCPAddr("tcp", address)
		if err == nil {
			return resolved, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("resolve raft advertise address %q: %w", address, err)
		}
		time.Sleep(time.Second)
	}
}

func (n *Node) IsLeader() bool { return n.raft.State() == raft.Leader }

// Leader returns the current leader peer, if one is known.
func (n *Node) Leader() (Peer, bool) {
	_, id := n.raft.LeaderWithID()
	peer, ok := n.peers[string(id)]
	return peer, ok
}

// WaitForLeader blocks until a leader is known or the timeout passes.
func (n *Node) WaitForLeader(timeout time.Duration) (Peer, error) {
	deadline := time.Now().Add(timeout)
	for {
		if peer, ok := n.Leader(); ok {
			return peer, nil
		}
		if time.Now().After(deadline) {
			return Peer{}, fmt.Errorf("no Raft leader elected within %s", timeout)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Apply replicates a command to a majority and applies it locally. It must run on the leader.
func (n *Node) Apply(command []byte, timeout time.Duration) error {
	future := n.raft.Apply(command, timeout)
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

// ConsistentRead confirms leadership with a quorum and waits for the local FSM to catch up,
// so a read served afterwards is linearizable.
func (n *Node) ConsistentRead(timeout time.Duration) error {
	if err := n.raft.VerifyLeader().Error(); err != nil {
		return err
	}
	commit := n.raft.CommitIndex()
	deadline := time.Now().Add(timeout)
	for n.raft.AppliedIndex() < commit {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for index %d to apply", commit)
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil
}

func (n *Node) Status() map[string]interface{} {
	leaderAddr, leaderID := n.raft.LeaderWithID()
	members := make([]map[string]string, 0)
	if future := n.raft.GetConfiguration(); future.Error() == nil {
		for _, server := range future.Configuration().Servers {
			members = append(members, map[string]string{"id": string(server.ID), "address": string(server.Address), "suffrage": server.Suffrage.String()})
		}
	}
	return map[string]interface{}{
		"id":            n.ID,
		"state":         n.raft.State().String(),
		"leader":        leaderID != "",
		"leader_id":     string(leaderID),
		"leader_addr":   string(leaderAddr),
		"term":          n.raft.Stats()["term"],
		"commit_index":  n.raft.CommitIndex(),
		"applied_index": n.raft.AppliedIndex(),
		"members":       members,
	}
}

// Close hands leadership to another member when possible, then stops Raft.
func (n *Node) Close() error {
	if n.IsLeader() && len(n.peers) > 1 {
		_ = n.raft.LeadershipTransfer().Error()
	}
	if err := n.raft.Shutdown().Error(); err != nil {
		return err
	}
	return n.logs.Close()
}
