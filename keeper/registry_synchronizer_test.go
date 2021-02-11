package keeper

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jinzhu/gorm"
	"github.com/smartcontractkit/external-initiator/eitest"
	"github.com/smartcontractkit/external-initiator/internal/mocks"
	"github.com/smartcontractkit/external-initiator/store"
	"github.com/stretchr/testify/require"
)

const syncTime = 3 * time.Second

var regConfig = struct {
	PaymentPremiumPPB uint32
	BlockCountPerTurn *big.Int
	CheckGasLimit     uint32
	StalenessSeconds  *big.Int
	FallbackGasPrice  *big.Int
	FallbackLinkPrice *big.Int
}{
	PaymentPremiumPPB: 100,
	BlockCountPerTurn: big.NewInt(20),
	CheckGasLimit:     2_000_000,
	StalenessSeconds:  big.NewInt(3600),
	FallbackGasPrice:  big.NewInt(1000000),
	FallbackLinkPrice: big.NewInt(1000000),
}

var upkeep = struct {
	Target              common.Address
	ExecuteGas          uint32
	CheckData           []byte
	Balance             *big.Int
	LastKeeper          common.Address
	Admin               common.Address
	MaxValidBlocknumber uint64
}{
	Target:              common.HexToAddress("0x0000000000000000000000000000000000000123"),
	ExecuteGas:          2_000_000,
	CheckData:           common.Hex2Bytes("0x1234"),
	Balance:             big.NewInt(1000000000000000000),
	LastKeeper:          common.HexToAddress("0x0000000000000000000000000000000000000456"),
	Admin:               common.HexToAddress("0x0000000000000000000000000000000000000789"),
	MaxValidBlocknumber: 1_000_000_000,
}

func setupRegistrySync(t *testing.T) (*gorm.DB, RegistrySynchronizer, *mocks.EthClient, func()) {
	db, cleanup := store.SetupTestDB(t)
	ethMock := new(mocks.EthClient)
	synchronizer := NewRegistrySynchronizer(db.DB(), ethMock, syncTime)
	return db.DB(), synchronizer, ethMock, cleanup
}

func Test_RegistrySynchronizer_AddsAndRemovesUpkeeps(t *testing.T) {
	t.Parallel()
	db, synchronizer, ethMock, cleanup := setupRegistrySync(t)
	defer cleanup()
	reg := newRegistry()
	err := db.Create(&reg).Error
	require.NoError(t, err)

	registryMock := eitest.NewContractMockReceiver(t, ethMock, upkeepRegistryABI, reg.Address)
	cancelledUpkeeps := []*big.Int{big.NewInt(0)}
	registryMock.MockResponse("getConfig", regConfig).Once()
	registryMock.MockResponse("getKeeperList", []common.Address{reg.From}).Once()
	registryMock.MockResponse("getCanceledUpkeepList", cancelledUpkeeps).Once()
	registryMock.MockResponse("getUpkeepCount", big.NewInt(3)).Once()
	registryMock.MockResponse("getUpkeep", upkeep).Twice() // upkeeps 1 & 2

	synchronizer.Start()
	defer synchronizer.Stop()

	eitest.WaitForCount(t, db, registration{}, 2)
	eitest.WaitForCount(t, db, registry{}, 1)
	ethMock.AssertExpectations(t)

	var upkeepRegistration registration
	err = db.Model(registration{}).First(&upkeepRegistration).Error
	require.NoError(t, err)

	require.Equal(t, upkeep.CheckData, upkeepRegistration.CheckData)
	require.Equal(t, upkeep.ExecuteGas, upkeepRegistration.ExecuteGas)

	cancelledUpkeeps = []*big.Int{big.NewInt(0), big.NewInt(1), big.NewInt(2)}
	registryMock.MockResponse("getConfig", regConfig).Once()
	registryMock.MockResponse("getKeeperList", []common.Address{reg.From}).Once()
	registryMock.MockResponse("getCanceledUpkeepList", cancelledUpkeeps).Once()
	registryMock.MockResponse("getUpkeepCount", big.NewInt(3)).Once()

	eitest.WaitForCount(t, db, registration{}, 0)
	eitest.WaitForCount(t, db, registry{}, 1)
	ethMock.AssertExpectations(t)
}
