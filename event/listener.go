package event

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/skyrings/skyring/conf"
	"github.com/skyrings/skyring/db"
	"github.com/skyrings/skyring/models"
	"github.com/skyrings/skyring/tools/logger"
	"github.com/skyrings/skyring/tools/uuid"
	"gopkg.in/mgo.v2/bson"
	"net"
	"net/http"
	"net/rpc"
	"net/rpc/jsonrpc"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//centralized architecture: A central Hub is going to receive all ingoing events and to broadcast them to each connected Client
type hub struct {
	clients    map[*client]bool
	broadcast  chan string
	register   chan *client
	unregister chan *client
	content    string
}

var (
	CenterHub hub
)

func New() *hub {
	CenterHub = hub{
		broadcast:  make(chan string),
		register:   make(chan *client),
		unregister: make(chan *client),
		clients:    make(map[*client]bool),
		content:    "",
	}
	return &CenterHub
}

//  At the end, we instantiate our hub
func (CenterHub *hub) Run() {
	for {
		select {
		case c := <-CenterHub.register:
			CenterHub.clients[c] = true
			c.send <- []byte(CenterHub.content)
			break

		case c := <-CenterHub.unregister:
			_, ok := CenterHub.clients[c]
			if ok {
				delete(CenterHub.clients, c)
				close(c.send)
			}
			break

		case m := <-CenterHub.broadcast:
			CenterHub.content = m
			CenterHub.broadcastMessage()
			break
		}
	}
}

// Broadcasting the message to all connected clients
func (CenterHub *hub) broadcastMessage() {
	for c := range CenterHub.clients {
		select {
		case c.send <- []byte(CenterHub.content):
			break

		// We can't reach the client
		default:
			close(c.send)
			delete(CenterHub.clients, c)
		}
	}
}

//Let's focus on Client code now
const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 1024 * 1024
)

type client struct {
	ws   *websocket.Conn
	send chan []byte
}

// Upgrading the websocket upgrader function
var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

// serveWs handles websocket requests from the peer.
func ServeWs(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Get().Error("Websocket upgrader function Failed: %s", err)
		return
	}
	c := &client{
		send: make(chan []byte, maxMessageSize),
		ws:   ws,
	}
	CenterHub.register <- c
	go c.writeEvents()
	c.readEvents()
}

//Read events
func (c *client) readEvents() {
	defer func() {
		CenterHub.unregister <- c
		c.ws.Close()
	}()
	c.ws.SetReadLimit(maxMessageSize)
	c.ws.SetReadDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error {
		c.ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		_, message, err := c.ws.ReadMessage()
		if err != nil {
			logger.Get().Error("Not able to read message from client: %s", err)
			break
		}
		fmt.Println("This is message recieved from client ", message)
	}
}

// write events
func (c *client) writeEvents() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.ws.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.write(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.write(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.write(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}

func (c *client) write(mt int, message []byte) error {
	c.ws.SetWriteDeadline(time.Now().Add(writeWait))
	return c.ws.WriteMessage(mt, message)
}

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
	var node []models.Node
	coll := sessionCopy.DB(conf.SystemConfig.DBConfig.Database).C(models.COLL_NAME_STORAGE_NODES)
	if err := coll.Find(bson.M{"hostname": event.Node}).All(&node); err != nil {
		logger.Get().Error("Node information read from DB failed for node: %s", err)
	}

	// Push the event to DB only if the node is managed
	if !node[0].Enabled {
		return
	}
	e.ClusterId = node[0].ClusterId
	e.NodeId = node[0].NodeId

	// For upcoming any new event ,adding it into CenterHub.broadcast
	eventObj, err := json.Marshal(e)
	if err != nil {
		logger.Get().Error("Error while converting into json marshal: %s", err)
	}
	CenterHub.broadcast <- string(eventObj)

	// Invoking the event handler
	for tag, handler := range handlermap {
		if match, err := filepath.Match(tag, e.Tag); err == nil {
			if match {
				if err := handler.(func(models.Event) error)(e); err != nil {
					logger.Get().Error("Event Handling Failed for event: %s", err)
					return
				}
				if err := persist_event(e); err != nil {
					logger.Get().Error("Could not persist the event to DB: %s", err)
					return
				} else {
					return
				}
			}
		} else {
			logger.Get().Error("Error while maping handler:", err)
			return
		}
	}
	logger.Get().Warning("Handler not defined for event %s", e.Tag)
	return
}

func (l *Listener) PersistNodeDbusEvent(args *models.NodeEvent, ack *bool) error {
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
