package server

import (
	pb "Go_Team01/src/config/proto"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"log"
	"strconv"
	"strings"
	"time"
)

func (s *Server) Set(ctx context.Context, kv *pb.KeyValue) (*pb.Response, error) {
	if _, err := uuid.Parse(kv.Key); err != nil {
		return &pb.Response{Success: false, Message: "Key is not a proper UUID4"}, err
	}

	data, err := json.Marshal(kv)
	if err != nil {
		return nil, err
	}

	applyFuture := s.raft.Apply(data, 10*time.Second)
	if err = applyFuture.Error(); err != nil {
		return &pb.Response{Success: false, Message: err.Error()}, err
	}

	countReplicas := len(s.raft.GetConfiguration().Configuration().Servers)
	return &pb.Response{Success: true, Message: fmt.Sprintf("Created (%v replicas)", countReplicas)}, nil
}

func (s *Server) Get(ctx context.Context, key *pb.Key) (*pb.KeyValue, error) {
	s.fsm.mux.Lock()
	defer s.fsm.mux.Unlock()

	value, ok := s.fsm.Storage[key.Key]
	if !ok {
		return nil, fmt.Errorf("not found")
	}

	return &pb.KeyValue{Key: key.Key, Value: value}, nil
}

func (s *Server) Delete(ctx context.Context, key *pb.Key) (*pb.Response, error) {
	s.fsm.mux.Lock()
	defer s.fsm.mux.Unlock()

	if _, ok := s.fsm.Storage[key.Key]; !ok {
		return &pb.Response{Success: false, Message: fmt.Sprintf("Key '%s' not found", key.Key)}, nil
	}

	delete(s.fsm.Storage, key.Key)
	return &pb.Response{Success: true, Message: fmt.Sprintf("Deleted key '%s'", key.Key)}, nil
}

func (s *Server) Heartbeat(ctx context.Context, _ *pb.Empty) (*pb.HeartbeatResponse, error) {
	state := s.raft.State().String()
	return &pb.HeartbeatResponse{
		ReplicationFactor: int32(len(s.raft.GetConfiguration().Configuration().Servers)),
		Issue:             "",
		State:             state,
	}, nil
}

func (s *Server) GetLeader(ctx context.Context, _ *pb.Empty) (*pb.LeaderResponse, error) {
	leaderAddr, leaderID := s.raft.LeaderWithID()
	if leaderAddr == "" {
		return &pb.LeaderResponse{
			Leader:   "",
			LeaderID: "",
			Issue:    "No leader elected",
		}, nil
	}
	return &pb.LeaderResponse{
		Leader:   generateAddress(string(leaderAddr), 100),
		LeaderID: string(leaderID),
		Issue:    "",
	}, nil
}

func (s *Server) StopServer(ctx context.Context, _ *pb.Empty) (*pb.Response, error) {
	_, leaderID := s.raft.LeaderWithID()
	s.raft.RemoveServer(leaderID, 0, 0)

	time.Sleep(2 * time.Second)
	log.Println("STOP", s.raft.GetConfiguration().Configuration())
	return &pb.Response{Success: true, Message: "Server stopped"}, nil
}

func (s *Server) GetAllServers(ctx context.Context, _ *pb.Empty) (*pb.AddressList, error) {
	servers := s.raft.GetConfiguration().Configuration().Servers
	var addressList []string

	for _, server := range servers {
		dataList := strings.Split(string(server.Address), ":")
		hostStr, portStr := dataList[0], dataList[1]
		port, _ := strconv.Atoi(portStr)
		addressList = append(addressList, hostStr+":"+strconv.Itoa(port+100))
	}
	log.Println("AddressList:", addressList)

	return &pb.AddressList{List: addressList}, nil
}
