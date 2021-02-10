// Package blockchain provides functionality to interact with
// different blockchain interfaces.
package blockchain

import (
	"errors"
)

var (
	ErrConnectionType = errors.New("unknown connection type")
	ErrSubscriberType = errors.New("unknown subscriber type")
)

type Params struct {
	Address string `json:"address"`
	From    string `json:"from"`
}
