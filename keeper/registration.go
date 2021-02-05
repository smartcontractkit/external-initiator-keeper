package keeper

import (
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/smartcontractkit/chainlink/core/utils"
)

type registration struct {
	ID                  int32 `gorm:"primary_key"`
	CheckData           []byte
	ExecuteGas          uint32
	RegistryID          uint32
	Registry            registry `gorm:"association_autoupdate:false"`
	UpkeepID            uint64
	PositioningConstant uint32
}

func (registration) TableName() string {
	return "keeper_registrations"
}

func CalcPositioningConstant(upkeepID uint64, registryAddress common.Address, numKeepers uint32) (uint32, error) {
	if numKeepers == 0 {
		return 0, errors.New("cannot calc positioning constant with 0 keepers")
	}

	upkeepBytes := make([]byte, binary.MaxVarintLen64)
	binary.PutUvarint(upkeepBytes, upkeepID)
	bytesToHash := utils.ConcatBytes(upkeepBytes, registryAddress.Bytes())
	hash, err := utils.Keccak256(bytesToHash)
	if err != nil {
		return 0, err
	}
	hashUint := big.NewInt(0).SetBytes(hash)
	constant := big.NewInt(0).Mod(hashUint, big.NewInt(int64(numKeepers)))

	return uint32(constant.Uint64()), nil
}
