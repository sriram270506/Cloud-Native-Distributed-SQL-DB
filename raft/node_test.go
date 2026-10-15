package raft

import (
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// counterFSM records applied commands so replicas can be compared.
type counterFSM struct {
	mu      sync.Mutex
	entries []string
}

func (f *counterFSM) apply(data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, string(data))
	return nil
}
func (f *counterFSM) snapshot() ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return json.Marshal(f.entries)
}
func (f *counterFSM) restore(data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return json.Unmarshal(data, &f.entries)
}
func (f *counterFSM) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.entries)
}

func freeAddr(t *testing.T) string {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().String()
}

func startNode(t *testing.T, dir string, peer Peer, peers []Peer, fsm *counterFSM) *Node {
	node, err := NewNode(Config{ID: peer.ID, RaftBind: peer.RaftAddr, RaftAdvertise: peer.RaftAddr, DataDir: filepath.Join(dir, peer.ID), Peers: peers, Apply: fsm.apply, Snapshot: fsm.snapshot, Restore: fsm.restore})
	if err != nil {
		t.Fatal(err)
	}
	return node
}

func waitFor(t *testing.T, what string, condition func() bool) {
	deadline := time.Now().Add(15 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func leaderOf(nodes map[string]*Node) *Node {
	for _, node := range nodes {
		if node != nil && node.IsLeader() {
			return node
		}
	}
	return nil
}

func TestReplicationFailoverAndRecovery(t *testing.T) {
	dir := t.TempDir()
	peers := make([]Peer, 3)
	for i := range peers {
		peers[i] = Peer{ID: fmt.Sprintf("node-%d", i+1), RaftAddr: freeAddr(t), HTTPAddr: "unused"}
	}
	fsms := map[string]*counterFSM{}
	nodes := map[string]*Node{}
	for _, peer := range peers {
		fsms[peer.ID] = &counterFSM{}
		nodes[peer.ID] = startNode(t, dir, peer, peers, fsms[peer.ID])
	}
	defer func() {
		for _, node := range nodes {
			if node != nil {
				node.Close()
			}
		}
	}()

	waitFor(t, "first leader", func() bool { return leaderOf(nodes) != nil })
	leader := leaderOf(nodes)
	for i := 0; i < 5; i++ {
		if err := leader.Apply([]byte(fmt.Sprintf("cmd-%d", i)), 5*time.Second); err != nil {
			t.Fatal(err)
		}
	}
	if err := leader.ConsistentRead(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	for id, fsm := range fsms {
		waitFor(t, id+" replication", func() bool { return fsm.count() == 5 })
	}

	// Stop the leader without handing off leadership; the other two still form a majority.
	oldID := leader.ID
	if err := leader.raft.Shutdown().Error(); err != nil {
		t.Fatal(err)
	}
	leader.logs.Close()
	nodes[oldID] = nil
	waitFor(t, "new leader", func() bool { return leaderOf(nodes) != nil })
	if err := leaderOf(nodes).Apply([]byte("after-failover"), 5*time.Second); err != nil {
		t.Fatal(err)
	}

	// Restart the old leader from its BoltDB log; it must catch up with the write it missed.
	var oldPeer Peer
	for _, peer := range peers {
		if peer.ID == oldID {
			oldPeer = peer
		}
	}
	fsms[oldID] = &counterFSM{}
	nodes[oldID] = startNode(t, dir, oldPeer, peers, fsms[oldID])
	waitFor(t, "restarted node catch-up", func() bool { return fsms[oldID].count() == 6 })
}
