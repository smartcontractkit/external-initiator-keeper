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

func NewRegistrySynchronizer(registryStore RegistryStore, ethClient eth.Client, syncInterval time.Duration) RegistrySynchronizer {
	return registrySynchronizer{
		ethClient:     ethClient,
		registryStore: registryStore,
		interval:      syncInterval,
		isRunning:     atomic.NewBool(false),
		chDone:        make(chan struct{}),
	}
}

type registrySynchronizer struct {
	endpoint      string
	ethClient     eth.Client
	interval      time.Duration
	isRunning     *atomic.Bool
	registryStore RegistryStore

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
			// TODO - if sync takes too long? need a queue approach like in executer
			// https://www.pivotaltracker.com/story/show/176747117
			rs.performFullSync()
		}
	}
}

func (rs registrySynchronizer) performFullSync() {
	logger.Debug("performing full sync of keeper registries")

	registries, err := rs.registryStore.Registries()
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

	err = rs.registryStore.UpdateRegistry(registry)
	if err != nil {
		logger.Error(err)
		return
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
	cancelledSet := make(map[uint64]bool)
	for _, upkeepID := range cancelled {
		cancelledSet[upkeepID] = true
	}
	err = rs.registryStore.BatchDelete(registry.ID, cancelled)
	if err != nil {
		logger.Error(err)
	}

	// add new upkeeps, update existing upkeeps
	count, err := contract.GetUpkeepCount(nil)
	if err != nil {
		logger.Error(err)
		return
	}
	var needToUpsert []uint64
	for upkeepID := uint64(0); upkeepID < count.Uint64(); upkeepID++ {
		if !cancelledSet[upkeepID] {
			needToUpsert = append(needToUpsert, upkeepID)
		}
	}
	for _, upkeepID := range needToUpsert {
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
		err = rs.registryStore.Upsert(newUpkeep)
		if err != nil {
			logger.Error(err)
		}
	}
}
