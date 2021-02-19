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
	"go.uber.org/atomic"
)

const syncInterval = 3 * time.Second

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

func setupRegistrySync(t *testing.T) (*gorm.DB, registrySynchronizer, *mocks.EthClient, func()) {
	db, cleanup := store.SetupTestDB(t)
	ethMock := new(mocks.EthClient)
	regStore := NewStore(db.DB())
	synchronizer := registrySynchronizer{
		ethClient:   ethMock,
		keeperStore: regStore,
		interval:    syncInterval,
		isRunning:   atomic.NewBool(false),
		isSyncing:   atomic.NewBool(false),
		chDone:      make(chan struct{}),
	}
	return db.DB(), synchronizer, ethMock, cleanup
}

func Test_RegistrySynchronizer_Start(t *testing.T) {
	db, synchronizer, _, cleanup := setupRegistrySync(t)
	defer cleanup()
	reg := newRegistry()
	err := db.Create(&reg).Error
	require.NoError(t, err)

	synchronizer.Start()
	require.NoError(t, err)
	defer synchronizer.Stop()

	err = synchronizer.Start()
	require.Error(t, err)
}

func Test_RegistrySynchronizer_AddsAndRemovesUpkeeps(t *testing.T) {
	db, synchronizer, ethMock, cleanup := setupRegistrySync(t)
	defer cleanup()
	reg := newRegistry()
	err := db.Create(&reg).Error
	require.NoError(t, err)

	registryMock := eitest.NewContractMockReceiver(t, ethMock, UpkeepRegistryABI, reg.Address)
	cancelledUpkeeps := []*big.Int{big.NewInt(1)}
	registryMock.MockResponse("getConfig", regConfig).Once()
	registryMock.MockResponse("getKeeperList", []common.Address{reg.From}).Once()
	registryMock.MockResponse("getCanceledUpkeepList", cancelledUpkeeps).Once()
	registryMock.MockResponse("getUpkeepCount", big.NewInt(3)).Once()
	registryMock.MockResponse("getUpkeep", upkeep).Times(3) // sync all 3, then delete

	synchronizer.performFullSync()

	eitest.AssertCount(t, db, registry{}, 1)
	eitest.AssertCount(t, db, registration{}, 2)
	ethMock.AssertExpectations(t)

	var upkeepRegistration registration
	err = db.Model(registration{}).First(&upkeepRegistration).Error
	require.NoError(t, err)

	require.Equal(t, upkeep.CheckData, upkeepRegistration.CheckData)
	require.Equal(t, upkeep.ExecuteGas, upkeepRegistration.ExecuteGas)

	cancelledUpkeeps = []*big.Int{big.NewInt(0), big.NewInt(1), big.NewInt(3)}
	registryMock.MockResponse("getConfig", regConfig).Once()
	registryMock.MockResponse("getKeeperList", []common.Address{reg.From}).Once()
	registryMock.MockResponse("getCanceledUpkeepList", cancelledUpkeeps).Once()
	registryMock.MockResponse("getUpkeepCount", big.NewInt(5)).Once()
	registryMock.MockResponse("getUpkeep", upkeep).Times(2) // two new upkeeps to sync

	synchronizer.performFullSync()

	eitest.AssertCount(t, db, registry{}, 1)
	eitest.AssertCount(t, db, registration{}, 2)
	ethMock.AssertExpectations(t)
}
