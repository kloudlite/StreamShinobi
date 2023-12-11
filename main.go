package main

import (
	"fmt"
	"os"

	"nproxy.io/apps/client"
	"nproxy.io/apps/server"
	nats_client "nproxy.io/pkg/nats-client"
)

func main() {
	nc, err := nats_client.GetConnection()
	if err != nil {
		panic(err)
	}
	defer nc.Close()

	s := os.Getenv("MODE")

	if s == "client" {
		if err := client.StartClient(nc); err != nil {
			fmt.Println(err)
		}
	} else {
		if err := server.StartServer(nc); err != nil {
			fmt.Println(err)
		}
	}

}
