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

// ExpectsMock variable is set when we run in a mock context
var ExpectsMock = false

type Params struct {
	Address string `json:"address"`
	From    string `json:"from"`
}
