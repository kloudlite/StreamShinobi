package server

import (
	"io"
	"strings"

	"nproxy.io/config"

	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/nats-io/nats.go"
)

var ServiceMap map[string]*config.Service

func reloadConfig(configData []byte, nc *nats.Conn) error {
	var data *config.Config

	if configData == nil {
		var err error
		data, err = config.GetConfig()
		if err != nil {
			return err
		}
	} else {
		err := json.Unmarshal(configData, &data)
		if err != nil {
			return err
		}
	}
	oldServiceMap := make(map[string]*config.Service)
	for _, service := range ServiceMap {
		oldServiceMap[getKey(service)] = service
	}
	ServiceMap = make(map[string]*config.Service)
	for key := range data.Services {
		s := data.Services[key]
		if _, ok := oldServiceMap[getKey(&s)]; !ok {
			ServiceMap[getKey(&s)] = &s
		} else {
			ServiceMap[getKey(&s)] = oldServiceMap[getKey(&s)]
		}
	}

	for key := range oldServiceMap {
		s := oldServiceMap[key]
		if _, ok := ServiceMap[key]; !ok {
			err := stopService(s)
			if err != nil {
				return err
			}
		}
	}

	for key := range ServiceMap {
		s := ServiceMap[key]
		if _, ok := oldServiceMap[getKey(s)]; !ok {
			err := startService(s, nc)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func getKey(service *config.Service) string {
	return fmt.Sprint(service.Target.Host, ":", service.Port, ":", service.Target.Port)
}

func stopService(service *config.Service) error {
	if service.Protocol == "udp" {
		if service.UdpConn != nil {
			err := service.UdpConn.Close()
			service.Closed = true
			fmt.Println("- stopping :: ", getKey(service))
			return err
		}
	} else {
		if service.Listener != nil {
			err := service.Listener.Close()
			service.Closed = true
			fmt.Println("- stopping :: ", getKey(service))
			return err
		}
	}
	return nil
}

func startService(service *config.Service, nc *nats.Conn) error {
	if service.Protocol == "udp" {
		udpAddr, err := net.ListenUDP("udp", &net.UDPAddr{Port: service.Port})
		if err != nil {
			return err
		}
		service.UdpConn = udpAddr
	} else {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", service.Port))
		if err != nil {
			return err
		}

		service.Listener = listener
	}

	service.Closed = false
	go runLoop(service, nc)
	return nil
}

func withUdpConn(nc *nats.Conn, service *config.Service) error {

	if service.Closed || service.UdpConn == nil {
		return nil
	}

	// rAddress := service.UdpConn.RemoteAddr()
	// fmt.Println("rAddress", rAddress)
	//
	// encodedAddr := base64.StdEncoding.EncodeToString([]byte(rAddress.String()))
	// // fmt.Println("EncodedAddr", encodedAddr)

	encodedDestAddr := base64.StdEncoding.EncodeToString([]byte(fmt.Sprint(service.Target.Host, ":", service.Target.Port)))
	// fmt.Println("EncodedDestAddr", encodedDestAddr)

	go func() {
		nc.Subscribe(fmt.Sprint("connection.", "myid.", "*", ".", encodedDestAddr, ".udp.reply"), func(msg *nats.Msg) {
			splits := strings.Split(msg.Subject, ".")

			srcAddr, err := base64.StdEncoding.DecodeString(splits[2])

			if err != nil {
				fmt.Println(err)
				return
			}

			u, err := net.ResolveUDPAddr("udp", string(srcAddr))
			if err != nil {
				fmt.Println(err)
				return
			}

			_, err = service.UdpConn.WriteToUDP(msg.Data, u)
			if err != nil {
				fmt.Println(err)
				return
			}
		})
	}()

	// Read Data into buffer and write to back to client
	buffer := make([]byte, 1024)
	for {
		n, address, err2 := service.UdpConn.ReadFromUDP(buffer)
		if err2 == io.EOF {
			continue
		}

		if err2 != nil {
			panic(err2)
		}

		fmt.Println(string(buffer[:n]))
		// base64 encode remote addr

		encodedAddr := base64.StdEncoding.EncodeToString([]byte(address.String()))

		err := nc.Publish(fmt.Sprint("connection.", "myid.", encodedAddr, ".", encodedDestAddr, ".udp"), buffer[:n])
		if err != nil {
			panic(err)
		}
	}

}

func withTcpConn(nc *nats.Conn, service *config.Service) error {

	if service.Closed || service.Listener == nil {
		return nil
	}

	accept, err := service.Listener.Accept()
	if err != nil {
		return err
	}

	encodedAddr := base64.StdEncoding.EncodeToString([]byte(accept.RemoteAddr().String()))
	// fmt.Println("EncodedAddr", encodedAddr)

	encodedDestAddr := base64.StdEncoding.EncodeToString([]byte(fmt.Sprint(service.Target.Host, ":", service.Target.Port)))
	// fmt.Println("EncodedDestAddr", encodedDestAddr)

	go func() {
		nc.Subscribe(fmt.Sprint("connection.", "myid.", encodedAddr, ".", encodedDestAddr, ".tcp.reply"), func(msg *nats.Msg) {
			_, err := accept.Write(msg.Data)
			if err != nil {
				fmt.Println(err)
				return
			}
		})
	}()

	go func() {
		nc.Subscribe(fmt.Sprint("connection.", "myid.", encodedAddr, ".", encodedDestAddr, ".tcp.done"), func(msg *nats.Msg) {
			err := accept.Close()
			if err != nil {
				fmt.Println(err)
				return
			}
		})
	}()

	go func() {
		// Read Data into buffer and write to back to client
		defer func() {
			err := accept.Close()
			if err != nil {
				return
			}
		}()
		buffer := make([]byte, 1024)
		for {
			n, err2 := accept.Read(buffer)
			if err2 == io.EOF {
				break
			}
			if err != nil {
				break
			}
			// base64 encode remote addr

			err := nc.Publish(fmt.Sprint("connection.", "myid.", encodedAddr, ".", encodedDestAddr, ".tcp"), buffer[:n])
			if err != nil {
				break
			}
		}
	}()

	return nil
}

func runLoop(service *config.Service, nc *nats.Conn) error {
	fmt.Println("+ starting :: ", getKey(service))
	for {

		if service.Protocol == "tcp" {
			if err := withTcpConn(nc, service); err != nil {
				fmt.Println(err)
			}
		} else {
			if err := withUdpConn(nc, service); err != nil {
				fmt.Println(err)
			}

			fmt.Println("udp")
			// fmt.Println("Unknown protocol: ", service.Protocol)
		}
	}
}

func startApi(nc *nats.Conn) {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Post("/post", func(c *fiber.Ctx) error {
		err := reloadConfig(c.Body(), nc)
		if err != nil {
			return err
		}
		c.Send([]byte("done"))
		return nil
	})
	app.Listen(":2999")
}

func StartServer(nc *nats.Conn) error {

	go startApi(nc)

	if err := reloadConfig(nil, nc); err != nil {
		panic(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	wg.Wait()

	return nil
}
