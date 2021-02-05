package keeper

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pborman/uuid"
	"github.com/smartcontractkit/chainlink/core/store/models"
	"github.com/smartcontractkit/external-initiator/keeper/keeper_registry_contract"
)

type registry struct {
	ID                uint32         `gorm:"primary_key"`
	Address           common.Address `gorm:"default:null"`
	BlockCountPerTurn uint32
	CheckGas          uint32
	From              common.Address `gorm:"default:null"`
	JobID             *models.ID     `gorm:"default:null"`
	KeeperIndex       uint32
	NumKeepers        uint32
	ReferenceID       string `gorm:"default:null"`
}

func NewRegistry(address common.Address, from common.Address, jobID *models.ID) registry {
	return registry{
		Address:     address,
		From:        from,
		JobID:       jobID,
		ReferenceID: uuid.New(),
	}
}

func (registry) TableName() string {
	return "keeper_registries"
}

func (reg registry) SyncFromContract(contract *keeper_registry_contract.KeeperRegistryContract) (registry, error) {
	config, err := contract.GetConfig(nil)
	if err != nil {
		return registry{}, err
	}
	reg.CheckGas = config.CheckGasLimit
	reg.BlockCountPerTurn = uint32(config.BlockCountPerTurn.Uint64())
	keeperAddresses, err := contract.GetKeeperList(nil)
	if err != nil {
		return registry{}, err
	}
	found := false
	for idx, address := range keeperAddresses {
		if address == reg.From {
			reg.KeeperIndex = uint32(idx)
			found = true
		}
	}
	if !found {
		return registry{}, fmt.Errorf("unable to find %s in keeper list on registry %s", reg.From.Hex(), reg.Address.Hex())
	}

	reg.NumKeepers = uint32(len(keeperAddresses))

	return reg, nil
}
