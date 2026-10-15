package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/sriram270506/cloud-native-distributed-sql-db/raft"
	dbsql "github.com/sriram270506/cloud-native-distributed-sql-db/sql"
	"github.com/sriram270506/cloud-native-distributed-sql-db/storage"
)

type databaseNode struct {
	db    *dbsql.Engine
	store *storage.Store
}
type request struct {
	SQL        string   `json:"sql"`
	Statements []string `json:"statements"`
}

func main() {
	port := flag.Int("port", 8080, "HTTP port")
	nodes := flag.Int("nodes", 3, "number of local Raft nodes")
	dataDir := flag.String("data", "./data/runtime", "runtime data directory")
	flag.Parse()
	if *nodes < 1 {
		log.Fatal("nodes must be at least 1")
	}
	instances := make([]databaseNode, *nodes)
	apply := make([]raft.ApplyFunc, *nodes)
	snapshots := make([]raft.SnapshotFunc, *nodes)
	restores := make([]raft.RestoreFunc, *nodes)
	for i := range instances {
		store, err := storage.Open(filepath.Join(*dataDir, fmt.Sprintf("node-%d", i+1), "state"))
		if err != nil {
			log.Fatal(err)
		}
		db, err := dbsql.New(store)
		if err != nil {
			log.Fatal(err)
		}
		instances[i] = databaseNode{db: db, store: store}
		index := i
		apply[index] = func(command []byte) error {
			var statements []string
			if err := json.Unmarshal(command, &statements); err != nil {
				return err
			}
			before, err := instances[index].store.Snapshot()
			if err != nil {
				return err
			}
			for _, statement := range statements {
				if _, err := instances[index].db.Execute(statement); err != nil {
					_ = instances[index].store.Restore(before)
					return err
				}
			}
			return nil
		}
		snapshots[index] = func() ([]byte, error) { return instances[index].store.Snapshot() }
		restores[index] = func(data []byte) error { return instances[index].store.Restore(data) }
	}
	cluster, err := raft.NewCluster(*nodes, *dataDir+"/raft", apply, snapshots, restores)
	if err != nil {
		log.Fatal(err)
	}
	defer cluster.Close()
	defer func() {
		for _, instance := range instances {
			instance.store.Close()
		}
	}()
	log.Printf("starting %d-node in-process Raft cluster on http://localhost:%d", *nodes, *port)
	http.HandleFunc("/health", func(writer http.ResponseWriter, _ *http.Request) {
		enableCORS(writer)
		writer.Header().Set("Content-Type", "application/json")
		json.NewEncoder(writer).Encode(map[string]interface{}{"leader": cluster.Leader() != nil, "nodes": cluster.Status()})
	})
	http.HandleFunc("/tables", func(writer http.ResponseWriter, httpRequest *http.Request) {
		enableCORS(writer)
		if httpRequest.Method != http.MethodGet {
			http.Error(writer, "GET required", http.StatusMethodNotAllowed)
			return
		}
		leader := cluster.Leader()
		if leader == nil {
			http.Error(writer, "no leader", http.StatusServiceUnavailable)
			return
		}
		var leaderIndex int
		if _, err := fmt.Sscanf(leader.ID, "node-%d", &leaderIndex); err != nil || leaderIndex < 1 || leaderIndex > len(instances) {
			http.Error(writer, "invalid leader state", http.StatusServiceUnavailable)
			return
		}
		tables := make([]string, 0)
		for _, key := range instances[leaderIndex-1].store.Keys("schema:") {
			tables = append(tables, strings.TrimPrefix(key, "schema:"))
		}
		writer.Header().Set("Content-Type", "application/json")
		json.NewEncoder(writer).Encode(map[string]interface{}{"tables": tables})
	})
	http.HandleFunc("/query", func(writer http.ResponseWriter, httpRequest *http.Request) {
		handleQuery(writer, httpRequest, cluster, instances)
	})
	http.HandleFunc("/transaction", func(writer http.ResponseWriter, httpRequest *http.Request) {
		handleTransaction(writer, httpRequest, cluster)
	})
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), nil); err != nil {
		log.Fatal(err)
	}
}

func handleQuery(writer http.ResponseWriter, httpRequest *http.Request, cluster *raft.Cluster, instances []databaseNode) {
	enableCORS(writer)
	if httpRequest.Method == http.MethodOptions {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if httpRequest.Method != http.MethodPost {
		http.Error(writer, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var body request
	if err := json.NewDecoder(httpRequest.Body).Decode(&body); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(body.SQL)), "select") {
		node := cluster.Leader()
		if node == nil {
			http.Error(writer, "no leader", http.StatusServiceUnavailable)
			return
		}
		var leaderIndex int
		if _, err := fmt.Sscanf(node.ID, "node-%d", &leaderIndex); err != nil || leaderIndex < 1 || leaderIndex > len(instances) {
			http.Error(writer, "invalid leader state", http.StatusServiceUnavailable)
			return
		}
		result, err := instances[leaderIndex-1].db.Execute(body.SQL)
		writeResult(writer, result, err)
		return
	}
	if err := cluster.Apply(mustJSON([]string{body.SQL}), 10*time.Second); err != nil {
		http.Error(writer, err.Error(), http.StatusConflict)
		return
	}
	writeResult(writer, map[string]string{"message": "committed"}, nil)
}

func handleTransaction(writer http.ResponseWriter, httpRequest *http.Request, cluster *raft.Cluster) {
	enableCORS(writer)
	if httpRequest.Method == http.MethodOptions {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if httpRequest.Method != http.MethodPost {
		http.Error(writer, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var body request
	if err := json.NewDecoder(httpRequest.Body).Decode(&body); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	if len(body.Statements) == 0 {
		http.Error(writer, "statements required", http.StatusBadRequest)
		return
	}
	if err := cluster.Apply(mustJSON(body.Statements), 10*time.Second); err != nil {
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
