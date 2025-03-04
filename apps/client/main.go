package client

import (
	"io"
	"strings"

	"encoding/base64"
	"fmt"
	"net"
	"sync"

	"github.com/nats-io/nats.go"
)

func withUdp(msg *nats.Msg, nc *nats.Conn, connectionPool map[string]*udpConnection, destAddr string) {

	sDec, _ := base64.StdEncoding.DecodeString(destAddr)

	u, err := net.ResolveUDPAddr("udp", string(sDec))
	if err != nil {
		fmt.Println(err, "here1")
		return
	}

	dial, e := net.DialUDP("udp", nil, u)
	if e != nil {
		fmt.Println(e, "here2")
		return
	}

	connectionPool[msg.Subject] = &udpConnection{
		conn:  dial,
		mutex: sync.Mutex{},
	}

	go func() {
		buffer := make([]byte, 1024)
		conn := connectionPool[msg.Subject]
		for {
			n, err := conn.conn.Read(buffer)

			if err == io.EOF {
				continue
			}

			if err != nil {
				break
			}

			if err := nc.Publish(fmt.Sprint(msg.Subject, ".reply"), buffer[:n]); err != nil {
				fmt.Println(err, "here3")
				continue
			}
		}
	}()

}

func WithTcp(msg *nats.Msg, nc *nats.Conn, connectionPool map[string]*tcpConnection, destAddr string) {

	// splits := strings.Split(msg.Subject, ".")
	// // firstSegment := splits[2]
	// secondSegment := splits[3]
	// // fmt.Println(firstSegment, secondSegment)

	sDec, _ := base64.StdEncoding.DecodeString(destAddr)
	dial, e := net.Dial("tcp", string(sDec))
	if e != nil {
		fmt.Println(e, "here4")
		return
	}
	connectionPool[msg.Subject] = &tcpConnection{
		conn:  dial,
		mutex: sync.Mutex{},
	}
	go func() {
		buffer := make([]byte, 1024)
		conn := connectionPool[msg.Subject]
		for {
			n, err := conn.conn.Read(buffer)

			if err == io.EOF {
				break
			}

			if err != nil {
				break
			}

			if err := nc.Publish(fmt.Sprint(msg.Subject, ".reply"), buffer[:n]); err != nil {
				fmt.Println(err, "here5")
				break
			}
		}

		if err := nc.Publish(fmt.Sprint(msg.Subject, ".done"), []byte("done")); err != nil {
			fmt.Println(err, "here6")
		}
	}()
}

type tcpConnection struct {
	conn  net.Conn
	mutex sync.Mutex
}

type udpConnection struct {
	conn  *net.UDPConn
	mutex sync.Mutex
}

func StartClient(nc *nats.Conn) error {

	connectionPool := make(map[string]*tcpConnection)
	udpConnectionPool := make(map[string]*udpConnection)

	if _, err := nc.Subscribe("connection.*.*.*.*", func(msg *nats.Msg) {
		splits := strings.Split(msg.Subject, ".")
		// // firstSegment := splits[2]
		destAddr := splits[3]
		protocol := splits[4]
		// // fmt.Println(firstSegment, secondSegment)
		if protocol == "tcp" {
			if connectionPool[msg.Subject] == nil {
				WithTcp(msg, nc, connectionPool, destAddr)
			}

			connectionPool[msg.Subject].mutex.Lock()
			defer connectionPool[msg.Subject].mutex.Unlock()

			if len(msg.Data) == 0 {
				return
			}

			if _, err := connectionPool[msg.Subject].conn.Write(msg.Data); err != nil {
				// getting broken pipe error here
				fmt.Println(len(msg.Data), "len", string(msg.Data))

				connectionPool[msg.Subject].conn.Close()
				delete(connectionPool, msg.Subject)
				return
			}

		} else {

			if udpConnectionPool[msg.Subject] == nil {
				withUdp(msg, nc, udpConnectionPool, destAddr)
			}

			if len(msg.Data) == 0 {
				return
			}

			if _, err := udpConnectionPool[msg.Subject].conn.Write(msg.Data); err != nil {
				fmt.Println(err, "err2")
			}
		}
	}); err != nil {
		fmt.Println(err, "my name is khan")
		return err
	}

	var wg sync.WaitGroup
	wg.Add(1)
	wg.Wait()

	return nil
}
