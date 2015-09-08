package util

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"strings"
	"sync"
	"time"
)

var StartedNodes map[string]time.Time = make(map[string]time.Time)
var StartedNodesLock sync.Mutex

func GetStartedNodes() []string {
	nodes := make([]string, len(StartedNodes))
	for n := range StartedNodes {
		nodes = append(nodes, n)
	}

	return nodes
}

type Listener int

type NodeStartEventArgs struct {
	Timestamp time.Time `json:"timestamp"`
	Node      string    `json:"node"`
}

func (l *Listener) PushNodeStartEvent(args *NodeStartEventArgs, ack *bool) error {
	timestamp := args.Timestamp
	node := strings.TrimSpace(args.Node)

	if node == "" || timestamp.IsZero() {
		*ack = false
		return nil
	}

	StartedNodesLock.Lock()
	defer StartedNodesLock.Unlock()
	StartedNodes[args.Node] = args.Timestamp
	*ack = true
	return nil
}

func (l *Listener) PushEvent(line string, ack *bool) error {
	// TODO: this needs to be handled properly
	fmt.Println(line)
	*ack = true
	return nil
}

func StartEventListener(eventSocket string) {
	listener := new(Listener)
	server := rpc.NewServer()
	server.Register(listener)

	l, e := net.Listen("unix", eventSocket)
	if e != nil {
		log.Fatal("listen error:", e)
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}

		go server.ServeCodec(jsonrpc.NewServerCodec(conn))
	}
}
