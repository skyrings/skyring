package event

import (
	"fmt"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/logger"
	"github.com/skyrings/skyring/tools/task"
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

func RouteEvent(event models.NodeEvent, t *task.Task) error {
	t.UpdateStatus("Building the event structure")
	var e models.Event
	e.Timestamp = event.Timestamp
	e.Tag = event.Tag
	e.Tags = event.Tags
	e.Message = event.Message
	e.Severity = event.Severity
	eventId, err := uuid.New()
	if err != nil {
		t.UpdateStatus("Uuid generation for event failed")
		logger.Get().Error("Uuid generation for the event failed: %s", event.Tag)
		return err
	}

	e.EventId = *eventId

	// querying DB to get node ID and Cluster ID for the event
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var node []models.StorageNode
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Find(bson.M{"hostname": event.Node}).All(&node); err != nil {
		t.UpdateStatus("Node information read from DB failed")
		logger.Get().Error("Node information read from DB failed for node: %s", event.Node)
		return err
	}
	e.ClusterId = node[0].ClusterId
	e.NodeId = node[0].UUID

	// Invoking the event handler
	for tag, handler := range handlermap {
		if match, _ := filepath.Match(tag, e.Tag); match {
			t.UpdateStatus("Invoking the handler")
			if err := handler.(func(models.Event) error)(e); err != nil {
				t.UpdateStatus("Event Handling Failed")
				logger.Get().Error("Event Handling Failed for event: %s", e.EventId)
				return err
			} else {
				return nil
			}
		}
	}
	logger.Get().Warning("Handler not defined for event %s", e.Tag)
	return nil
}

func (l *Listener) PersistNodeDbusEvent(args *models.NodeEvent, ack *bool) error {
	if args.Timestamp.IsZero() || args.Node == "" || args.Tag == "" || args.Message == "" || args.Severity == "" {
		logger.Get().Error("Incomplete details in the event", *args)
		*ack = false
		return nil
	}
	asyncEventRouter := func(t *task.Task) {
		t.UpdateStatus("started the task for RouteEvent : %s", t.ID)
		if err := RouteEvent(*args, t); err != nil {
			t.UpdateStatus("Failed")
		} else {
			t.UpdateStatus("Success")
		}
		t.Done()
	}
	if taskId, err := task.GetManager().Run("RouteEvent", asyncEventRouter); err != nil {
		logger.Get().Error("Unable to create the task for RouteEvent", err)
		*ack = false
		return nil
	} else {
		logger.Get().Debug("Task Created: ", taskId.String())
		*ack = true
		return nil
	}
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
