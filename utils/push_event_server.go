package push_event_server

import (
    "fmt"
    "log"
    "net"
    "net/rpc"
    "net/rpc/jsonrpc"
)

type Listener int

func (l *Listener) PushEvent(line string, ack *bool) error {
     fmt.Println(line)
     *ack = true
     return nil
}

func startServer() {
    listener := new(Listener)

    server := rpc.NewServer()
    server.Register(listener)

    l, e := net.Listen("unix", "/tmp/echo.sock")

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

// func main() {
//     startServer()
// }
