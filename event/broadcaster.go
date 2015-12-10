package event

import (
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/skyrings/skyring/tools/logger"
	"net/http"
	"time"
)

// Centralized architecture: A central EventBroadcaster is going to receive all
// ingoing events and to broadcast them to each connected Client
type EventBroadcaster struct {
	clients      map[*client]bool
	chBroadcast  chan string
	chRegister   chan *client
	chUnregister chan *client
	content      string
}

var (
	Broadcaster EventBroadcaster
)

// Initialized Broadcast variable
func InitBroadcaster() {
	Broadcaster = EventBroadcaster{
		chBroadcast:  make(chan string),
		chRegister:   make(chan *client),
		chUnregister: make(chan *client),
		clients:      make(map[*client]bool),
		content:      "",
	}
}

// Getter for Broadcast variable
func GetBroadcaster() *EventBroadcaster {
	return &Broadcaster
}

//  At the end, we instantiate our EventBroadcaster
func (Broadcaster *EventBroadcaster) Run() {
	for {
		select {
		case c := <-Broadcaster.chRegister:
			Broadcaster.clients[c] = true
			c.send <- []byte(Broadcaster.content)
			break

		case c := <-Broadcaster.chUnregister:
			_, ok := Broadcaster.clients[c]
			if ok {
				delete(Broadcaster.clients, c)
				close(c.send)
			}
			break

		case m := <-Broadcaster.chBroadcast:
			Broadcaster.content = m
			Broadcaster.broadcastMessage()
			break
		}
	}
}

// Broadcasting the message to all connected clients
func (Broadcaster *EventBroadcaster) broadcastMessage() {
	for c := range Broadcaster.clients {
		select {
		case c.send <- []byte(Broadcaster.content):
			break

		// We can't reach the client
		default:
			close(c.send)
			delete(Broadcaster.clients, c)
		}
	}
}

//Let's focus on Client code now
const (
	WRITE_WAIT       = 10 * time.Second
	PONG_WAIT        = 60 * time.Second
	PING_PERIOD      = (PONG_WAIT * 9) / 10
	MAX_MESSAGE_SIZE = 1024 * 1024
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
		send: make(chan []byte, MAX_MESSAGE_SIZE),
		ws:   ws,
	}
	Broadcaster.chRegister <- c
	go c.writeEvents()
	c.readEvents()
}

//Read events
func (c *client) readEvents() {
	defer func() {
		Broadcaster.chUnregister <- c
		c.ws.Close()
	}()
	c.ws.SetReadLimit(MAX_MESSAGE_SIZE)
	c.ws.SetReadDeadline(time.Now().Add(PONG_WAIT))
	c.ws.SetPongHandler(func(string) error {
		c.ws.SetReadDeadline(time.Now().Add(PONG_WAIT))
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
	ticker := time.NewTicker(PING_PERIOD)
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
	c.ws.SetWriteDeadline(time.Now().Add(WRITE_WAIT))
	return c.ws.WriteMessage(mt, message)
}
