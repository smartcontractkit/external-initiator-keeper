package client

import (
	"context"
	"net/url"
	"os"
	"os/signal"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/smartcontractkit/chainlink/core/services/eth"
	"github.com/smartcontractkit/external-initiator/chainlink"
	"github.com/smartcontractkit/external-initiator/keeper"
	"github.com/smartcontractkit/external-initiator/store"
)

type storeInterface interface {
	Close() error
	DB() *gorm.DB
}

// startService runs the Service in the background and gracefully stops when a
// SIGINT is received.
func startService(
	config Config,
	dbClient *store.Client,
) {
	logger.Info("Starting External Initiator")

	clUrl, err := url.Parse(normalizeLocalhost(config.ChainlinkURL))
	if err != nil {
		logger.Fatal(err)
	}

	retryConfig := chainlink.RetryConfig{
		Timeout:  config.ChainlinkTimeout,
		Attempts: config.ChainlinkRetryAttempts,
		Delay:    config.ChainlinkRetryDelay,
	}

	chainlinkClient := chainlink.NewClient(
		config.InitiatorToChainlinkAccessKey,
		config.InitiatorToChainlinkSecret,
		*clUrl,
		retryConfig,
	)

	ethClient, err := eth.NewClient(config.KeeperEthEndpoint)
	if err != nil {
		logger.Fatal(err)
	}
	err = ethClient.Dial(context.Background())
	if err != nil {
		logger.Fatal(err)
	}

	srv := NewService(dbClient, chainlinkClient, ethClient, config)

	go func() {
		err := srv.Run()
		if err != nil {
			logger.Fatal(err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	logger.Info("Shutting down...")
	srv.Close()
	os.Exit(0)
}

// Service holds the main process for running
// the external initiator.
type Service struct {
	clNode               chainlink.Client
	registryStore        keeper.RegistryStore
	config               Config
	upkeepExecuter       keeper.UpkeepExecuter
	registrySynchronizer keeper.RegistrySynchronizer
}

// NewService returns a new instance of Service, using
// the provided database client and Chainlink node config.
func NewService(
	dbClient storeInterface,
	clNode chainlink.Client,
	ethClient eth.Client,
	config Config,
) *Service {
	registryStore := keeper.NewRegistryStore(dbClient.DB())
	upkeepExecuter := keeper.NewUpkeepExecuter(registryStore, clNode, ethClient)
	registrySynchronizer := keeper.NewRegistrySynchronizer(registryStore, ethClient, config.KeeperRegistrySyncInterval)

	return &Service{
		registryStore:        registryStore,
		clNode:               clNode,
		config:               config,
		upkeepExecuter:       upkeepExecuter,
		registrySynchronizer: registrySynchronizer,
	}
}

// Run loads subscriptions, validates and subscribes to them.
func (srv *Service) Run() error {

	err := srv.upkeepExecuter.Start()
	if err != nil {
		return err
	}

	err = srv.registrySynchronizer.Start()
	if err != nil {
		return err
	}

	go RunWebserver(srv.config.ChainlinkToInitiatorAccessKey, srv.config.ChainlinkToInitiatorSecret, srv.registryStore, srv.config.Port)

	return nil
}

// Close shuts down any open subscriptions and closes
// the database client.
func (srv *Service) Close() {
	srv.upkeepExecuter.Stop()
	srv.registrySynchronizer.Stop()

	err := srv.registryStore.Close()
	if err != nil {
		logger.Error(err)
	}

	logger.Info("All connections closed. Bye!")
}

func normalizeLocalhost(endpoint string) string {
	if strings.HasPrefix(endpoint, "localhost") {
		return "http://" + endpoint
	}
	return endpoint
}
