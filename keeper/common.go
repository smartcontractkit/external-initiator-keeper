package keeper

import (
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/smartcontractkit/external-initiator/keeper/keeper_registry_contract"
)

var upkeepRegistryABI = mustGetABI(keeper_registry_contract.KeeperRegistryContractABI)

func mustGetABI(json string) abi.ABI {
	abi, err := abi.JSON(strings.NewReader(json))
	if err != nil {
		panic("could not parse ABI: " + err.Error())
	}
	return abi
}
