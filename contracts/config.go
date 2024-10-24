package iggcon

import "context"

type IggyConfiguration struct {
	context.Context
	BaseAddress string   `json:"baseAddress"`
	Protocol    Protocol `json:"protocol"`
}

type Protocol string

const (
	Http Protocol = "Http"
	Tcp  Protocol = "Tcp"
	Quic Protocol = "Quic"
)
