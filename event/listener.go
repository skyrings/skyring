package event

import (
	"encoding/json"
	"fmt"
	"github.com/skyrings/skyring-common/conf"
	"github.com/skyrings/skyring-common/db"
	common_event "github.com/skyrings/skyring-common/event"
	"github.com/skyrings/skyring-common/models"
	"github.com/skyrings/skyring-common/tools/logger"
	"github.com/skyrings/skyring-common/tools/uuid"
	"github.com/skyrings/skyring/apps/skyring"
	"gopkg.in/mgo.v2/bson"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"path/filepath"
	"strings"
	"time"
)

type Listener int

type NodeStartEventArgs struct {
	Timestamp time.Time `json:"timestamp"`
	Node      string    `json:"node"`
}

func (l *Listener) PushNodeStartEvent(args *NodeStartEventArgs, ack *bool) error {
	reqId, err := uuid.New()
	if err != nil {
		logger.Get().Error("Error Creating the RequestId. error: %v", err)
		return err
	}
	ctxt := fmt.Sprintf("%v:%v", models.ENGINE_NAME, reqId.String())
	timestamp := args.Timestamp
	node := strings.TrimSpace(args.Node)
	if node == "" || timestamp.IsZero() {
		*ack = false
		return nil
	}
	handle_node_start_event(node, ctxt)
	*ack = true
	return nil
}

func RouteEvent(event models.NodeEvent) {
	reqId, err := uuid.New()
	if err != nil {
		logger.Get().Error("Error Creating the RequestId. error: %v", err)
		return
	}
	ctxt := fmt.Sprintf("%v:%v", models.ENGINE_NAME, reqId.String())

	var e models.AppEvent
	e.Timestamp = event.Timestamp
	e.Tags = event.Tags
	e.Message = event.Message
	eventId, err := uuid.New()
	if err != nil {
		logger.Get().Error("%s-Uuid generation for the event failed for node: %s. error: %v", ctxt, event.Node, err)
		return
	}

	e.EventId = *eventId
	// querying DB to get node ID and Cluster ID for the event
	sessionCopy := db.GetDatastore().Copy()
	defer sessionCopy.Close()
	var node models.Node
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Find(bson.M{"hostname": event.Node}).One(&node); err != nil {
		logger.Get().Error("%s-Node information read from DB failed for node: %s. error: %v", ctxt, event.Node, err)
		return
	}

	// Push the event to DB only if the node is managed
	if node.Hostname == "" || !node.Enabled {
		return
	}
	e.ClusterId = node.ClusterId
	e.NodeId = node.NodeId
	e.NodeName = node.Hostname

	// Invoking the event handler
	for tag, handler := range handlermap {
		if match, err := filepath.Match(tag, event.Tag); err == nil {
			if match {
				if e, err = handler.(func(models.AppEvent, string) (models.AppEvent, error))(e, ctxt); err != nil {
					logger.Get().Error("%s-Event Handling Failed for event for node: %s. error: %v", ctxt, node.Hostname, err)
					return
				}
				if e.Name == "" {
					return
				}
				if err := common_event.AuditLog(ctxt, e, skyring.GetDbProvider()); err != nil {
					logger.Get().Error("%s-Could not persist the event to DB for node: %s. error: %v", ctxt, node.Hostname, err)
					return
				} else {
					// For upcoming any new event , broadcasting to all connected clients
					eventObj, err := json.Marshal(e)
					if err != nil {
						logger.Get().Error("%s-Error marshalling the event data for node: %s. error: %v", ctxt, node.Hostname, err)
					}
					GetBroadcaster().chBroadcast <- string(eventObj)
					return
				}
			}
		} else {
			logger.Get().Error("%s-Error while maping handler for event for node: %s. error: %v", ctxt, node.Hostname, err)
			return
		}
	}
	// This has to be un commented once we start recieving events from calamari
	// Handle Provider specific events
	//app := skyring.GetApp()
	//if err := app.RouteProviderEvents(ctxt, e); err != nil {
	//	logger.Get().Error("%s-Event:%s could not be handled for node: %s. error: %v", ctxt, event.Tag, node.Hostname, err)
	//}
	return
}

func (l *Listener) PersistNodeEvent(args *models.NodeEvent, ack *bool) error {
	if args.Timestamp.IsZero() || args.Node == "" || args.Tag == "" || args.Message == "" || args.Severity == "" {
		logger.Get().Error("Incomplete details in the event %v", *args)
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

func (l *Listener) PushUnManagedNode(args *NodeStartEventArgs, ack *bool) error {
	reqId, err := uuid.New()
	if err != nil {
		logger.Get().Error("Error Creating the RequestId. error: %v", err)
		return err
	}
	ctxt := fmt.Sprintf("%v:%v", models.ENGINE_NAME, reqId.String())
	timestamp := args.Timestamp
	node := strings.TrimSpace(args.Node)
	if node == "" || timestamp.IsZero() {
		*ack = false
		return nil
	}
	if err := handle_UnManagedNode(node, ctxt); err != nil {
		*ack = false
		return nil
	}
	*ack = true
	return nil
}
