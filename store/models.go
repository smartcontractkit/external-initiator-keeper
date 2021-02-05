package store

import "time"

type RuntimeConfig struct {
	KeeperEthEndpoint          string
	KeeperRegistrySyncInterval time.Duration
}
