package client

import (
	pb "Go_Team01/src/config/proto"
	"bufio"
	"context"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"os"
	"strings"
	"time"
)

type Client struct {
	conn          *grpc.ClientConn
	client        pb.RaftServiceClient
	serverList    []string
	currentServer string
	stopped       bool
	countReplicas int
}

func NewClient(addr string) (*Client, error) {
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed connection to initial server: %v", err)
	}

	client := pb.NewRaftServiceClient(conn)
	log.Println("connect to:", addr)
	fmt.Println("Connected to address:", addr)

	return &Client{
		conn:          conn,
		client:        client,
		currentServer: addr,
		stopped:       false,
	}, nil
}

func (c *Client) Initialize() error {
	if err := c.connectToLeader(); err != nil {
		return fmt.Errorf("failed initialize: %v", err)
	}

	c.countReplicas = len(c.serverList)
	return nil
}

func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) Run() {
	defer c.Close()

	log.Println("currentServer:", c.currentServer)
	go c.checkConnection()

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			log.Fatalf("failed to read input: %v", err)
		}

		cmd, arg, err := c.processingValue(input)
		if err != nil {
			continue
		}

		switch cmd {
		case "SET":
			args := strings.SplitN(arg, " ", 2)
			if len(args) != 2 {
				fmt.Println("Usage: SET <key> <value>")
				continue
			}
			key := args[0]
			value := args[1]
			setReq := &pb.KeyValue{Key: key, Value: value}
			resp, err := c.client.Set(context.Background(), setReq)
			if err != nil {
				fmt.Printf("SET request failed: %v\n", err)
				continue
			}
			fmt.Println(resp.Message)

		case "GET":
			key := arg
			getReq := &pb.Key{Key: key}
			resp, err := c.client.Get(context.Background(), getReq)
			if err != nil {
				fmt.Printf("GET request failed: %v\n", err)
				continue
			}
			fmt.Println(resp.Value)

		case "DELETE":
			key := arg
			deleteReq := &pb.Key{Key: key}
			resp, err := c.client.Delete(context.Background(), deleteReq)
			if err != nil {
				fmt.Printf("DELETE request failed: %v\n", err)
				continue
			}
			fmt.Printf("%s: %s\n", resp.Message)

		case "STOP":
			c.stopped = true
			_, err = c.client.StopServer(context.Background(), &pb.Empty{})
			if err != nil {
				log.Fatalf("could not stop server: %v", err)
			}
			log.Println("sever at stopped")
			c.stopped = false
		case "EXIT":
			fmt.Println("Exiting...")
			return

		default:
			fmt.Println("Unknown command. Available commands: SET, GET, DELETE, HEARTBEAT, EXIT")
		}
	}
}

func (c *Client) checkConnection() {
	for {
		if c.stopped == false {
			resp, err := c.client.Heartbeat(context.Background(), &pb.Empty{})
			if err != nil || resp.State == "Shutdown" {
				log.Printf("Connection lost with server %s, attempting to switch...", c.currentServer)
				answer := c.updateListServers()
				log.Println("update list servers", answer)
				c.switchServer()
				_ = c.connectToLeader()
			}
			time.Sleep(5 * time.Second)
		}
	}
}

func (c *Client) switchServer() {
	for _, server := range c.serverList {
		err := c.reconnectToServer(server)
		if err == nil {
			if err == nil {
				log.Printf("Switched to server: %s\n", c.currentServer)
				return
			}
		} else {
			log.Printf("Failed to connect to server %s: %v\n", server, err)
		}

	}
	log.Println("All servers are unreachable")
}

func (c *Client) reconnectToServer(server string) error {
	ctx := context.Background()
	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			return fmt.Errorf("failed to close previous connection: %v", err)
		}
	}

	conn, err := grpc.DialContext(ctx, server, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed connection to initial server: %v", err)
	}

	c.conn = conn
	c.client = pb.NewRaftServiceClient(conn)
	c.currentServer = server
	return nil
}

func (c *Client) connectToLeader() error {
	log.Printf("connecting to leader...")
	ctx := context.Background()
	leaderResp, err := c.client.GetLeader(ctx, &pb.Empty{})
	if err != nil {
		return fmt.Errorf("failed to get leader information: %v", err)
	}

	if leaderResp.Leader == "" {
		return fmt.Errorf("no leader elected")
	}

	leaderAddr := leaderResp.Leader

	if err = c.conn.Close(); err != nil {
		return fmt.Errorf("failed to close initial connection: %v", err)
	}

	c.conn, err = grpc.DialContext(ctx, leaderAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to leader: %v", err)
	}

	c.client = pb.NewRaftServiceClient(c.conn)

	serverResp, err := c.client.GetAllServers(ctx, &pb.Empty{})
	if err != nil {
		return fmt.Errorf("failed to get list addresses: %v", err)
	}

	log.Println("connect to:", leaderAddr)
	c.currentServer = leaderAddr
	c.serverList = serverResp.List

	c.showServers()

	return nil
}

func (c *Client) updateListServers() bool {
	log.Println("current server:", c.currentServer)
	log.Println("servers:", c.serverList)
	for idx, server := range c.serverList {
		if c.currentServer == server {
			c.serverList = append(c.serverList[:idx], c.serverList[idx+1:]...)
			return true
		}
	}

	return false
}

func (c *Client) processingValue(input string) (string, string, error) {
	var cmd, arg string
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", fmt.Errorf("empty input")
	}

	spaceIndex := strings.Index(input, " ")

	if spaceIndex != -1 {
		cmd = input[:spaceIndex]
		arg = input[spaceIndex+1:]
	} else {
		cmd = input
	}

	return cmd, arg, nil
}

func (c *Client) showServers() {
	fmt.Println("Know nodes:")
	for _, server := range c.serverList {
		fmt.Println(server)
	}

	if len(c.serverList) == 1 {
		fmt.Printf("WARNING: cluster size (1) is smaller than a replication factor (%v)!\n", c.countReplicas)
	}
}
