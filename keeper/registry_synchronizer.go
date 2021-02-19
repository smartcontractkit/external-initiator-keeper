package keeper

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/smartcontractkit/chainlink/core/services/eth"
	"github.com/smartcontractkit/external-initiator/keeper/keeper_registry_contract"
	"go.uber.org/atomic"
)

// max goroutines is a product of these queue sizes
const syncRegistryQueueSize = 3
const syncUpkeepQueueSize = 10

type RegistrySynchronizer interface {
	Start() error
	Stop()
}

func NewRegistrySynchronizer(keeperStore Store, ethClient eth.Client, syncInterval time.Duration) RegistrySynchronizer {
	return registrySynchronizer{
		ethClient:   ethClient,
		keeperStore: keeperStore,
		interval:    syncInterval,
		isRunning:   atomic.NewBool(false),
		chDone:      make(chan struct{}),
	}
}

type registrySynchronizer struct {
	endpoint    string
	ethClient   eth.Client
	interval    time.Duration
	isRunning   *atomic.Bool
	isSyncing   *atomic.Bool
	keeperStore Store

	chDone chan struct{}
}

func (rs registrySynchronizer) Start() error {
	if rs.isRunning.Load() {
		return errors.New("already started")
	}
	rs.isRunning.Store(true)
	go rs.run()
	return nil
}

func (rs registrySynchronizer) Stop() {
	close(rs.chDone)
}

func (rs registrySynchronizer) run() {
	ticker := time.NewTicker(rs.interval)
	defer ticker.Stop()

	for {
		select {
		case <-rs.chDone:
			return
		case <-ticker.C:
			rs.performFullSync()
		}
	}
}

// performFullSync syncs every registy in the database
// It blocks until the full sync is complete
func (rs registrySynchronizer) performFullSync() {
	logger.Debug("performing full sync of all keeper registries")

	registries, err := rs.keeperStore.Registries()
	if err != nil {
		logger.Error(err)
	}

	wg := sync.WaitGroup{}
	wg.Add(len(registries))

	// batch sync registries
	chSyncRegistryQueue := make(chan struct{}, syncRegistryQueueSize)
	done := func() { <-chSyncRegistryQueue; wg.Done() }
	for _, registry := range registries {
		chSyncRegistryQueue <- struct{}{}
		go rs.syncRegistry(registry, done)
	}

	wg.Wait()
}

func (rs registrySynchronizer) syncRegistry(registry registry, doneCallback func()) {
	defer doneCallback()

	logger.Debugf("syncing registry %s", registry.Address.Hex())

	err := func() error {
		contract, err := keeper_registry_contract.NewKeeperRegistryContract(registry.Address, rs.ethClient)
		if err != nil {
			return err
		}
		registry, err = registry.SyncFromContract(contract)
		if err != nil {
			return err
		}
		if err = rs.keeperStore.UpsertRegistry(registry); err != nil {
			return err
		}
		if err = rs.addNewUpkeeps(contract, registry); err != nil {
			return err
		}
		if err = rs.deleteCanceledUpkeeps(contract, registry); err != nil {
			return err
		}
		return nil
	}()

	if err != nil {
		logger.Errorf("unable to sync registry %s, err: %v", registry.Address.Hex(), err)
	}
}

func (rs registrySynchronizer) addNewUpkeeps(
	contract *keeper_registry_contract.KeeperRegistryContract,
	reg registry,
) error {
	nextUpkeepID, err := rs.keeperStore.NextUpkeepIDForRegistry(reg)
	if err != nil {
		return err
	}

	countOnContractBig, err := contract.GetUpkeepCount(nil)
	if err != nil {
		return err
	}
	countOnContract := countOnContractBig.Uint64()

	wg := sync.WaitGroup{}
	wg.Add(int(countOnContract - nextUpkeepID))

	// batch sync registries
	chSyncUpkeepQueue := make(chan struct{}, syncUpkeepQueueSize)
	done := func() { <-chSyncUpkeepQueue; wg.Done() }
	for upkeepID := nextUpkeepID; upkeepID < countOnContract; upkeepID++ {
		chSyncUpkeepQueue <- struct{}{}
		err := rs.syncUpkeep(contract, reg, upkeepID, done)
		if err != nil {
			logger.Error(err)
		}
	}

	wg.Wait()
	return nil
}

func (rs registrySynchronizer) deleteCanceledUpkeeps(
	contract *keeper_registry_contract.KeeperRegistryContract,
	reg registry,
) error {
	canceledBigs, err := contract.GetCanceledUpkeepList(nil)
	if err != nil {
		return err
	}
	canceled := make([]uint64, len(canceledBigs))
	for idx, upkeepID := range canceledBigs {
		canceled[idx] = upkeepID.Uint64()
	}
	return rs.keeperStore.BatchDeleteUpkeeps(reg.ID, canceled)
}

func (rs registrySynchronizer) syncUpkeep(
	contract *keeper_registry_contract.KeeperRegistryContract,
	registry registry,
	upkeepID uint64,
	doneCallback func(),
) error {
	defer doneCallback()

	upkeepConfig, err := contract.GetUpkeep(nil, big.NewInt(int64(upkeepID)))
	if err != nil {
		return err
	}
	positioningConstant, err := CalcPositioningConstant(upkeepID, registry.Address, registry.NumKeepers)
	if err != nil {
		return fmt.Errorf("unable to calculate positioning constant: %v", err)
	}
	newUpkeep := registration{
		CheckData:           upkeepConfig.CheckData,
		ExecuteGas:          upkeepConfig.ExecuteGas,
		RegistryID:          registry.ID,
		PositioningConstant: positioningConstant,
		UpkeepID:            upkeepID,
	}

	return rs.keeperStore.UpsertUpkeep(newUpkeep)
}
