package server

import (
	pb "Go_Team01/src/config/proto"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"google.golang.org/grpc"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Server struct {
	pb.UnimplementedRaftServiceServer
	mux      sync.Mutex
	Storage  map[string]string
	Address  string
	Replicas int

	raft      *raft.Raft
	fsm       *FSM
	logStore  *raftboltdb.BoltStore
	stable    *raftboltdb.BoltStore
	snapshot  *raft.FileSnapshotStore
	transport *raft.NetworkTransport
}

func NewServer(address string, replicas int) *Server {
	return &Server{
		Storage:  make(map[string]string),
		Replicas: replicas,
		fsm: &FSM{
			Storage: make(map[string]string),
		},
		Address: address,
	}
}

type FSM struct {
	Storage map[string]string
	mux     sync.Mutex
}

type FSMSnapshot struct {
	Storage map[string]string
}

func (f *FSM) Apply(log *raft.Log) interface{} {
	var kv pb.KeyValue
	err := json.Unmarshal(log.Data, &kv)
	if err != nil {
		return err
	}

	f.mux.Lock()
	defer f.mux.Unlock()
	f.Storage[kv.Key] = kv.Value
	return nil
}

func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mux.Lock()
	defer f.mux.Unlock()

	snapshot := make(map[string]string)
	for k, v := range f.Storage {
		snapshot[k] = v
	}

	return &FSMSnapshot{
		Storage: snapshot,
	}, nil
}

func (f *FSM) Restore(snapshot io.ReadCloser) error {
	var data map[string]string
	if err := json.NewDecoder(snapshot).Decode(&data); err != nil {
		return err
	}

	f.mux.Lock()
	defer f.mux.Unlock()
	f.Storage = data
	return nil
}

func (f *FSMSnapshot) Persist(sink raft.SnapshotSink) error {
	err := json.NewEncoder(sink).Encode(f.Storage)
	if err != nil {
		sink.Cancel()
		return err
	}
	sink.Close()
	return nil
}

func (f *FSMSnapshot) Release() {}

func (s *Server) InitializeRaft(nodeID string, address string, peers []raft.Server) error {
	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(nodeID)
	var err error

	if err = os.MkdirAll("store/"+nodeID, 0755); err != nil {
		return err
	}

	s.logStore, err = raftboltdb.NewBoltStore("store/" + nodeID + "/log.bolt")
	if err != nil {
		return err
	}

	s.stable, err = raftboltdb.NewBoltStore("store/" + nodeID + "/stable.bolt")
	if err != nil {
		return err
	}

	s.snapshot, err = raft.NewFileSnapshotStore("store/"+nodeID+"/snapshots", 1, nil)
	if err != nil {
		return err
	}

	s.transport, err = raft.NewTCPTransport(address, nil, 3, 10*time.Second, nil)
	if err != nil {
		return err
	}

	s.raft, err = raft.NewRaft(config, s.fsm, s.logStore, s.stable, s.snapshot, s.transport)
	if err != nil {
		return err
	}

	configuration := raft.Configuration{Servers: peers}
	s.raft.BootstrapCluster(configuration)

	return nil
}

func StartGRPCServer(srv *Server, address string) (*grpc.Server, error) {
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	grpcServer := grpc.NewServer()
	pb.RegisterRaftServiceServer(grpcServer, srv)

	go func() {
		if err = grpcServer.Serve(lis); err != nil {
			log.Printf("Failed to serve: %v", err)
		}
	}()

	return grpcServer, nil
}

func (s *Server) Run() {
	replicas := s.Replicas
	peers := generateRaftServers(s.Address, replicas)

	for i := 0; i < replicas; i++ {
		nodeID := string(peers[i].ID)
		addr := string(peers[i].Address)
		var server *Server
		if i == 0 {
			server = s
		} else {
			server = NewServer(addr, replicas)
		}

		if err := server.InitializeRaft(nodeID, addr, peers); err != nil {
			log.Fatalf("Error initializing Raft for %s: %v\n", nodeID, err)
			return
		}

		go func(i int) {
			newAddr := generateAddress(server.Address, 100)
			_, err := StartGRPCServer(server, newAddr)
			if err != nil {
				log.Fatalf("Failed to start gRPC server on address %d: %v\n", newAddr, err)
			}
		}(i)
	}

	time.Sleep(2 * time.Second)

	log.Println("count servers:", len(s.raft.GetConfiguration().Configuration().Servers))
	log.Println(s.raft.GetConfiguration().Configuration().Servers)
	select {}
}

func generateAddress(address string, number int) string {
	addrList := strings.Split(address, ":")
	port, _ := strconv.Atoi(addrList[1])
	addr := fmt.Sprintf("%s:%d", addrList[0], port+number)

	return addr
}

func generateRaftServers(address string, replicas int) []raft.Server {
	var peers []raft.Server
	for i := 0; i < replicas; i++ {
		peers = append(peers, raft.Server{
			ID:      raft.ServerID("node" + strconv.Itoa(i+1)),
			Address: raft.ServerAddress(generateAddress(address, i)),
		})
	}

	return peers
}
