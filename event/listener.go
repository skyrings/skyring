package event

import (
	"fmt"
	"github.com/golang/glog"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
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

func (l *Listener) PersistNodeDbusEvent(args *models.NodeEvent, ack *bool) error {
	var event models.NodeEvent
	event.Timestamp = args.Timestamp
	event.Node = args.Node
	event.Tag = args.Tag
	event.Message = args.Message
	event.Severity = args.Severity

	if event.Timestamp.IsZero() || event.Node == "" || event.Tag == "" || event.Message == "" || event.Severity == "" {
		*ack = false
		return nil
	}

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_NODE_EVENTS)
	if err := coll.Insert(event); err != nil {
		glog.Errorf("Error adding the node event: %v", err)
		*ack = false
		return nil
	}
	*ack = true
	return nil
}

func (l *Listener) PushEvent(line string, ack *bool) error {
	// TODO: this needs to be handled properly
	fmt.Println(line)
	*ack = true
	return nil
}

func StartListener(eventSocket string) {
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
