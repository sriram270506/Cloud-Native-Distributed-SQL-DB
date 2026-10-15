package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/sriram270506/cloud-native-distributed-sql-db/raft"
	dbsql "github.com/sriram270506/cloud-native-distributed-sql-db/sql"
	"github.com/sriram270506/cloud-native-distributed-sql-db/storage"
)

const (
	leaderWait      = 5 * time.Second
	applyTimeout    = 10 * time.Second
	forwardedHeader = "X-Cndsdb-Forwarded"
	maxBodyBytes    = 1 << 20
)

type request struct {
	SQL        string   `json:"sql"`
	Statements []string `json:"statements"`
}

type server struct {
	node  *raft.Node
	db    *dbsql.Engine
	store *storage.Store
}

func main() {
	id := flag.String("id", "node-1", "unique Raft server ID of this process")
	port := flag.Int("port", 8080, "HTTP port")
	raftBind := flag.String("raft-addr", "127.0.0.1:7000", "Raft TCP bind address")
	advertise := flag.String("advertise", "", "Raft address peers use to reach this node (default: -raft-addr)")
	peersFlag := flag.String("peers", "", "comma-separated id=raft_host:port=http_host:port for every voter, this node included (default: single-node cluster)")
	dataDir := flag.String("data", "./data/runtime", "data directory for this node")
	probe := flag.String("probe", "", "GET this URL, exit 0 on HTTP 200 and 1 otherwise (container health checks)")
	flag.Parse()
	if *probe != "" {
		os.Exit(runProbe(*probe))
	}
	if *advertise == "" {
		*advertise = *raftBind
	}
	peers, err := parsePeers(*peersFlag)
	if err != nil {
		log.Fatal(err)
	}
	if len(peers) == 0 {
		peers = []raft.Peer{{ID: *id, RaftAddr: *advertise, HTTPAddr: fmt.Sprintf("127.0.0.1:%d", *port)}}
	}

	nodeDir := filepath.Join(*dataDir, *id)
	store, err := storage.Open(filepath.Join(nodeDir, "state"))
	if err != nil {
		log.Fatal(err)
	}
	// The Raft log and snapshots are the source of truth; Raft replays them into an empty FSM on start.
	if err := store.Restore([]byte("{}")); err != nil {
		log.Fatal(err)
	}
	db, err := dbsql.New(store)
	if err != nil {
		log.Fatal(err)
	}
	srv := &server{db: db, store: store}
	node, err := raft.NewNode(raft.Config{
		ID:            *id,
		RaftBind:      *raftBind,
		RaftAdvertise: *advertise,
		DataDir:       filepath.Join(nodeDir, "raft"),
		Peers:         peers,
		Apply:         srv.apply,
		Snapshot:      store.Snapshot,
		Restore:       store.Restore,
	})
	if err != nil {
		log.Fatal(err)
	}
	srv.node = node

	mux := http.NewServeMux()
	mux.HandleFunc("/health", srv.handleHealth)
	mux.HandleFunc("/ready", srv.handleReady)
	mux.Handle("/tables", srv.onLeader(http.MethodGet, srv.handleTables))
	mux.Handle("/query", srv.onLeader(http.MethodPost, srv.handleQuery))
	mux.Handle("/transaction", srv.onLeader(http.MethodPost, srv.handleTransaction))
	httpServer := &http.Server{Addr: fmt.Sprintf(":%d", *port), Handler: mux}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	log.Printf("%s: HTTP on :%d, Raft on %s, %d voter(s)", *id, *port, *advertise, len(peers))
	if leader, err := node.WaitForLeader(30 * time.Second); err == nil {
		log.Printf("%s: leader is %s", *id, leader.ID)
	} else {
		log.Printf("%s: %v; still waiting for quorum", *id, err)
	}

	<-ctx.Done()
	log.Printf("%s: shutting down", *id)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	if err := node.Close(); err != nil {
		log.Printf("raft shutdown: %v", err)
	}
	store.Close()
}

// runProbe exists because the distroless image has no curl or wget.
func runProbe(target string) int {
	response, err := (&http.Client{Timeout: 3 * time.Second}).Get(target)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, response.Status)
		return 1
	}
	return 0
}

func parsePeers(value string) ([]raft.Peer, error) {
	peers := make([]raft.Peer, 0)
	for _, entry := range strings.Split(value, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "=")
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return nil, fmt.Errorf("invalid peer %q: want id=raft_host:port=http_host:port", entry)
		}
		peers = append(peers, raft.Peer{ID: parts[0], RaftAddr: parts[1], HTTPAddr: parts[2]})
	}
	return peers, nil
}

// apply is the Raft FSM callback. It runs on every replica in log order, so a batch is all-or-nothing everywhere.
func (s *server) apply(command []byte) error {
	var statements []string
	if err := json.Unmarshal(command, &statements); err != nil {
		return err
	}
	before, err := s.store.Snapshot()
	if err != nil {
		return err
	}
	for _, statement := range statements {
		if _, err := s.db.Execute(statement); err != nil {
			if restoreErr := s.store.Restore(before); restoreErr != nil {
				return fmt.Errorf("%v (rollback failed: %v)", err, restoreErr)
			}
			return err
		}
	}
	return nil
}

// onLeader runs the handler on the leader and proxies the request to the leader from any follower,
// so clients and load balancers can talk to any node.
func (s *server) onLeader(method string, handler http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, httpRequest *http.Request) {
		if httpRequest.Method == http.MethodOptions {
			enableCORS(writer)
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		if httpRequest.Method != method {
			enableCORS(writer)
			http.Error(writer, method+" required", http.StatusMethodNotAllowed)
			return
		}
		// Buffer the body (capped) so a forward can be retried after a failover.
		body, err := io.ReadAll(http.MaxBytesReader(writer, httpRequest.Body, maxBodyBytes))
		if err != nil {
			enableCORS(writer)
			http.Error(writer, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		httpRequest.Body.Close()
		httpRequest.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
		httpRequest.Body, _ = httpRequest.GetBody()
		deadline := time.Now().Add(leaderWait)
		for {
			leader, err := s.node.WaitForLeader(time.Until(deadline))
			if err != nil {
				enableCORS(writer)
				http.Error(writer, err.Error(), http.StatusServiceUnavailable)
				return
			}
			if s.node.IsLeader() {
				enableCORS(writer)
				handler(writer, httpRequest)
				return
			}
			if httpRequest.Header.Get(forwardedHeader) != "" {
				enableCORS(writer)
				http.Error(writer, "leadership changed while forwarding; retry", http.StatusServiceUnavailable)
				return
			}
			var dialErr error
			proxy := httputil.NewSingleHostReverseProxy(&url.URL{Scheme: "http", Host: leader.HTTPAddr})
			proxy.ErrorHandler = func(writer http.ResponseWriter, _ *http.Request, err error) {
				var opErr *net.OpError
				// A failed dial means the request never reached the old leader, so resending it is safe.
				if errors.As(err, &opErr) && opErr.Op == "dial" && time.Now().Before(deadline) {
					dialErr = err
					return
				}
				enableCORS(writer)
				http.Error(writer, "forward to leader "+leader.ID+": "+err.Error(), http.StatusBadGateway)
			}
			attempt := httpRequest.Clone(httpRequest.Context())
			attempt.Body, _ = httpRequest.GetBody()
			attempt.Header.Set(forwardedHeader, s.node.ID)
			proxy.ServeHTTP(writer, attempt)
			if dialErr == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	})
}

func (s *server) handleHealth(writer http.ResponseWriter, _ *http.Request) {
	enableCORS(writer)
	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(s.node.Status())
}

// handleReady reports 200 once this node knows a leader, for load balancer and Kubernetes readiness checks.
func (s *server) handleReady(writer http.ResponseWriter, _ *http.Request) {
	enableCORS(writer)
	if _, ok := s.node.Leader(); !ok {
		http.Error(writer, "no leader", http.StatusServiceUnavailable)
		return
	}
	writer.Write([]byte("ok\n"))
}

func (s *server) handleTables(writer http.ResponseWriter, _ *http.Request) {
	if err := s.node.ConsistentRead(applyTimeout); err != nil {
		http.Error(writer, err.Error(), http.StatusServiceUnavailable)
		return
	}
	tables := make([]string, 0)
	for _, key := range s.store.Keys("schema:") {
		tables = append(tables, strings.TrimPrefix(key, "schema:"))
	}
	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(map[string]interface{}{"tables": tables})
}

func (s *server) handleQuery(writer http.ResponseWriter, httpRequest *http.Request) {
	var body request
	if err := json.NewDecoder(httpRequest.Body).Decode(&body); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(body.SQL)), "select") {
		if err := s.node.ConsistentRead(applyTimeout); err != nil {
			http.Error(writer, err.Error(), http.StatusServiceUnavailable)
			return
		}
		result, err := s.db.Execute(body.SQL)
		writeResult(writer, result, err)
		return
	}
	if err := s.node.Apply(mustJSON([]string{body.SQL}), applyTimeout); err != nil {
		http.Error(writer, err.Error(), http.StatusConflict)
		return
	}
	writeResult(writer, map[string]string{"message": "committed"}, nil)
}

func (s *server) handleTransaction(writer http.ResponseWriter, httpRequest *http.Request) {
	var body request
	if err := json.NewDecoder(httpRequest.Body).Decode(&body); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	if len(body.Statements) == 0 {
		http.Error(writer, "statements required", http.StatusBadRequest)
		return
	}
	if err := s.node.Apply(mustJSON(body.Statements), applyTimeout); err != nil {
		http.Error(writer, err.Error(), http.StatusConflict)
		return
	}
	writeResult(writer, map[string]string{"message": "transaction committed"}, nil)
}

func enableCORS(writer http.ResponseWriter) {
	writer.Header().Set("Access-Control-Allow-Origin", "*")
	writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
}

func writeResult(writer http.ResponseWriter, result interface{}, err error) {
	writer.Header().Set("Content-Type", "application/json")
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	json.NewEncoder(writer).Encode(result)
}

func mustJSON(value interface{}) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}
