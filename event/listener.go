package event

import (
	"encoding/json"
	"fmt"
	"github.com/skyrings/skyring/apps/skyring"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/logger"
	"github.com/skyrings/skyring/tools/uuid"
	"gopkg.in/mgo.v2/bson"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"path/filepath"
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

func RouteEvent(event models.NodeEvent) {
	var e models.Event
	e.Timestamp = event.Timestamp
	e.Tag = event.Tag
	e.Tags = event.Tags
	e.Message = event.Message
	e.Severity = event.Severity
	eventId, err := uuid.New()
	if err != nil {
		logger.Get().Error("Uuid generation for the event failed: ", err)
	}

	e.EventId = *eventId

	// querying DB to get node ID and Cluster ID for the event
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var node models.Node
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Find(bson.M{"hostname": event.Node}).One(&node); err != nil {
		logger.Get().Error("Node information read from DB failed for node: %s", err)
		return
	}

	// Push the event to DB only if the node is managed
	if node.Hostname == "" || !node.Enabled {
		return
	}
	e.ClusterId = node.ClusterId
	e.NodeId = node.NodeId

	// Invoking the event handler
	for tag, handler := range handlermap {
		if match, err := filepath.Match(tag, e.Tag); err == nil {
			if match {
				if err := handler.(func(models.Event) error)(e); err != nil {
					logger.Get().Error("Event Handling Failed for event: %s", err)
					return
				}
				if err := Persist_event(e); err != nil {
					logger.Get().Error("Could not persist the event to DB: %s", err)
					return
				} else {
					// For upcoming any new event , broadcasting to all connected clients
					eventObj, err := json.Marshal(e)
					if err != nil {
						logger.Get().Error("Error marshalling the event data. error: %v", err)
					}
					GetBroadcaster().chBroadcast <- string(eventObj)
					return
				}
			}
		} else {
			logger.Get().Error("Error while maping handler:", err)
			return
		}
	}

	// Handle Provider specific events
	app := skyring.GetApp()
	if err := app.RouteProviderEvents(e); err != nil {
		logger.Get().Error("Event could not be handled for event:%s", e.Tag)
	}

	return
}

func (l *Listener) PersistNodeEvent(args *models.NodeEvent, ack *bool) error {
	if args.Timestamp.IsZero() || args.Node == "" || args.Tag == "" || args.Message == "" || args.Severity == "" {
		logger.Get().Error("Incomplete details in the event", *args)
		*ack = false
		return nil
	}
	go RouteEvent(*args)
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
		logger.Get().Fatal("listen error:", e)
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			logger.Get().Fatal(err)
		}

		go server.ServeCodec(jsonrpc.NewServerCodec(conn))
	}
}
