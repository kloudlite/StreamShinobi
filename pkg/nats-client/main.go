package nats_client

import "github.com/nats-io/nats.go"

func GetConnection() (*nats.Conn, error) {
	nc, err := nats.Connect(nats.DefaultURL)
	return nc, err
}
