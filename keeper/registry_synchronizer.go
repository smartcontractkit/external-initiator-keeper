package keeper

import (
	"errors"
	"math/big"
	"time"

	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/smartcontractkit/chainlink/core/services/eth"
	"github.com/smartcontractkit/external-initiator/keeper/keeper_registry_contract"
	"go.uber.org/atomic"
)

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
		isSyncing:   atomic.NewBool(false),
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

func (rs registrySynchronizer) performFullSync() {
	// skip syncing if previous sync is unfinished
	if rs.isSyncing.Load() {
		return
	}
	rs.isSyncing.Store(true)
	defer func() { rs.isSyncing.Store(false) }()

	logger.Debug("performing full sync of keeper registries")

	// DEV: assumign that number of registries is relatively low
	// otherwise, this needs to be batched
	registries, err := rs.keeperStore.Registries()
	if err != nil {
		logger.Error(err)
	}

	// TODO - parallellize this
	// https://www.pivotaltracker.com/story/show/176747117
	for _, registry := range registries {
		rs.syncRegistry(registry)
	}
}

func (rs registrySynchronizer) syncRegistry(registry registry) {
	// WARN - this could get memory intensive depending on how many upkeeps there are
	// especially because of keccak()
	logger.Debugf("syncing registry %s", registry.Address.Hex())

	contract, err := keeper_registry_contract.NewKeeperRegistryContract(registry.Address, rs.ethClient)
	if err != nil {
		logger.Error(err)
		return
	}

	registry, err = registry.SyncFromContract(contract)
	if err != nil {
		logger.Error(err)
		return
	}

	err = rs.keeperStore.UpsertRegistry(registry)
	if err != nil {
		logger.Error(err)
		return
	}

	// add new upkeeps, update existing upkeeps
	nextUpkeepID, err := rs.keeperStore.NextUpkeepIDForRegistry(registry)
	if err != nil {
		logger.Error(err)
		return
	}

	countOnContractBig, err := contract.GetUpkeepCount(nil)
	if err != nil {
		logger.Error(err)
		return
	}
	countOnContract := countOnContractBig.Uint64()

	for upkeepID := nextUpkeepID; upkeepID < countOnContract; upkeepID++ {
		upkeepConfig, err := contract.GetUpkeep(nil, big.NewInt(int64(upkeepID)))
		if err != nil {
			logger.Error(err)
			continue
		}
		positioningConstant, err := CalcPositioningConstant(upkeepID, registry.Address, registry.NumKeepers)
		if err != nil {
			logger.Error("unable to calculate positioning constant,", "err", err)
			continue
		}
		newUpkeep := registration{
			CheckData:           upkeepConfig.CheckData,
			ExecuteGas:          upkeepConfig.ExecuteGas,
			RegistryID:          registry.ID,
			PositioningConstant: positioningConstant,
			UpkeepID:            upkeepID,
		}

		// TODO - parallelize
		// https://www.pivotaltracker.com/story/show/176747117
		err = rs.keeperStore.UpsertUpkeep(newUpkeep)
		if err != nil {
			logger.Error(err)
		}
	}

	// delete cancelled upkeeps
	cancelledBigs, err := contract.GetCanceledUpkeepList(nil)
	if err != nil {
		logger.Error(err)
		return
	}
	cancelled := make([]uint64, len(cancelledBigs))
	for idx, upkeepID := range cancelledBigs {
		cancelled[idx] = upkeepID.Uint64()
	}
	err = rs.keeperStore.BatchDeleteUpkeeps(registry.ID, cancelled)
	if err != nil {
		logger.Error(err)
		return
	}
}
