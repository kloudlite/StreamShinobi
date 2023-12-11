package config

import (
	"net"
	"os"

	"sigs.k8s.io/yaml"
)

type Target struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type Service struct {
	Protocol string `yaml:"protocol"`
	Port     int    `yaml:"port"`
	Target   Target `yaml:"target"`
	Listener net.Listener
	UdpConn  *net.UDPConn
	Closed   bool
}

type Config struct {
	Services []Service `json:"services"`
}

func GetConfig() (*Config, error) {

	var data Config

	confFile := os.Getenv("PROXYCONFIG")
	configData, err := os.ReadFile(confFile)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(configData, &data)
	if err != nil {
		return nil, err
	}

	return &data, nil
}
