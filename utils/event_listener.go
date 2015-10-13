package util

import (
	"fmt"
	"github.com/golang/glog"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"io/ioutil"
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

type NodeDbusEventArgs struct {
	Timestamp time.Time `json:"timestamp"`
	Node      string    `json:"node"`
	Tag       string    `json:"tag"`
	Message   string    `json:"message"`
	Severity  string    `json:"severity"`
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

func (l *Listener) PushNodeDbusEvent(args *NodeDbusEventArgs, ack *bool) error {
	d1 := []byte("hello\ngo\n")
	//a := []byte(fmt.Sprintf("%v", *args))
	err := ioutil.WriteFile("/tmp/dat1.txt", d1, 0644)
	if err != nil {
		*ack = false
		return nil
	}
	var event models.NodeEventStructure
	event.Timestamp = args.Timestamp
	event.Node = args.Node
	event.Tag = args.Tag
	event.Message = args.Message
	event.Severity = args.Severity

	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.NODE_EVENT_COLLECTION)
	if err := coll.Insert(event); err != nil {
		glog.Fatalf("Error adding the node event: %v", err)
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
