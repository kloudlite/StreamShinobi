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

func withUdp(msg *nats.Msg, nc *nats.Conn, connectionPool map[string]*net.UDPConn, destAddr string) {

	sDec, _ := base64.StdEncoding.DecodeString(destAddr)

	u, err := net.ResolveUDPAddr("udp", string(sDec))
	if err != nil {
		fmt.Println(err)
		return
	}

	dial, e := net.DialUDP("udp", nil, u)
	if e != nil {
		fmt.Println(e)
		return
	}

	connectionPool[msg.Subject] = dial
	go func() {
		buffer := make([]byte, 1024)
		conn := connectionPool[msg.Subject]
		for {
			n, err := conn.Read(buffer)

			if err == io.EOF {
				continue
			}

			if err != nil {
				break
			}

			if err := nc.Publish(fmt.Sprint(msg.Subject, ".reply"), buffer[:n]); err != nil {
				fmt.Println(err)
				continue
			}
		}
	}()

}

func WithTcp(msg *nats.Msg, nc *nats.Conn, connectionPool map[string]net.Conn, destAddr string) {

	// splits := strings.Split(msg.Subject, ".")
	// // firstSegment := splits[2]
	// secondSegment := splits[3]
	// // fmt.Println(firstSegment, secondSegment)

	sDec, _ := base64.StdEncoding.DecodeString(destAddr)
	dial, e := net.Dial("tcp", string(sDec))
	if e != nil {
		fmt.Println(e)
		return
	}
	connectionPool[msg.Subject] = dial
	go func() {
		buffer := make([]byte, 1024)
		conn := connectionPool[msg.Subject]
		for {
			n, err := conn.Read(buffer)

			if err == io.EOF {
				break
			}

			if err != nil {
				break
			}

			if err := nc.Publish(fmt.Sprint(msg.Subject, ".reply"), buffer[:n]); err != nil {
				fmt.Println(err)
				break
			}
		}

		if err := nc.Publish(fmt.Sprint(msg.Subject, ".done"), []byte("done")); err != nil {
			fmt.Println(err)
		}
	}()
}

func StartClient(nc *nats.Conn) error {

	connectionPool := make(map[string]net.Conn)
	udpConnectionPool := make(map[string]*net.UDPConn)

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

			if _, err := connectionPool[msg.Subject].Write(msg.Data); err != nil {
				fmt.Println(err)
			}
		} else {

			fmt.Println("here")

			if udpConnectionPool[msg.Subject] == nil {
				withUdp(msg, nc, udpConnectionPool, destAddr)
			}

			if _, err := udpConnectionPool[msg.Subject].Write(msg.Data); err != nil {
				fmt.Println(err)
			}
		}
	}); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(1)
	wg.Wait()

	return nil
}
