package nats

import (
	"log"

	"github.com/nats-io/nats.go"
)

var nc *nats.Conn

func Connect(url string) (*nats.Conn, error) {
	var err error
	nc, err = nats.Connect(url)
	if err != nil {
		return nil, err
	}
	log.Printf("Conectado ao NATS em %s", url)
	return nc, nil
}

func GetConn() *nats.Conn {
	return nc
}
