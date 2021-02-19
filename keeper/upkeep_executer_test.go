package keeper

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jinzhu/gorm"
	"github.com/smartcontractkit/chainlink/core/store/models"
	"github.com/smartcontractkit/external-initiator/eitest"
	"github.com/smartcontractkit/external-initiator/internal/mocks"
	"github.com/smartcontractkit/external-initiator/store"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func setupExecuter(t *testing.T) (
	*gorm.DB,
	UpkeepExecuter,
	*mocks.ChainlinkClient,
	*mocks.EthClient,
	func(),
) {
	db, cleanup := store.SetupTestDB(t)
	clMock := new(mocks.ChainlinkClient)
	ethMock := new(mocks.EthClient)
	regStore := NewRegistryStore(db.DB())
	executer := NewUpkeepExecuter(regStore, clMock, ethMock)
	return db.DB(), executer, clMock, ethMock, cleanup
}

// setupHeadsSubscription sets the mock calls for the head tracker and returns a blocking
// function that yields the new heads channel for triggering new heads
func setupHeadsSubscription(ethMock *mocks.EthClient) (getHeadsChannel func() chan<- *models.Head) {
	sub := new(mocks.EthSubscription)
	sub.On("Err").Return(nil)
	sub.On("Unsubscribe").Return(nil).Once()
	chchHeaders := make(chan chan<- *models.Head)
	ethMock.
		On("SubscribeNewHead", mock.Anything, mock.Anything).
		Return(sub, nil).
		Run(func(args mock.Arguments) {
			chchHeaders <- args.Get(1).(chan<- *models.Head)
		}).
		Once()

	return func() chan<- *models.Head {
		return <-chchHeaders
	}
}

var checkUpkeepResponse = struct {
	PerformData    []byte
	MaxLinkPayment *big.Int
	GasLimit       *big.Int
	GasWei         *big.Int
	LinkEth        *big.Int
}{
	PerformData:    common.Hex2Bytes("0x1234"),
	MaxLinkPayment: big.NewInt(0), // doesn't matter
	GasLimit:       big.NewInt(2_000_000),
	GasWei:         big.NewInt(0), // doesn't matter
	LinkEth:        big.NewInt(0), // doesn't matter
}

func Test_UpkeepExecuter_ErrorsIfStartedTwice(t *testing.T) {
	_, executer, _, ethMock, cleanup := setupExecuter(t)
	defer cleanup()
	setupHeadsSubscription(ethMock)

	err := executer.Start()
	require.NoError(t, err)
	defer executer.Stop()

	err = executer.Start()
	require.Error(t, err)

}
func Test_UpkeepExecuter_PerformsUpkeep_Happy(t *testing.T) {
	db, executer, clMock, ethMock, cleanup := setupExecuter(t)
	defer cleanup()
	getHeadsChannel := setupHeadsSubscription(ethMock)

	err := executer.Start()
	require.NoError(t, err)
	defer executer.Stop()
	chHeads := getHeadsChannel()
	chJobWasRun := make(chan struct{})

	reg := newRegistry()
	err = db.Create(&reg).Error
	require.NoError(t, err)

	upkeep := newRegistration(reg, 0)
	err = db.Create(&upkeep).Error
	require.NoError(t, err)

	registryMock := eitest.NewContractMockReceiver(t, ethMock, UpkeepRegistryABI, reg.Address)
	registryMock.MockResponse("checkUpkeep", checkUpkeepResponse)

	clMock.
		On("TriggerJob", reg.JobID.String(), mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			chJobWasRun <- struct{}{}
		})

	t.Run("runs upkeep on triggering block number", func(t *testing.T) {
		head := models.NewHead(big.NewInt(20), eitest.NewHash(), eitest.NewHash(), 1000)
		chHeads <- &head

		select {
		case <-time.NewTimer(2 * time.Second).C:
			t.Fatal("new job run never triggered")
		case <-chJobWasRun:
		}
	})

	t.Run("skips upkeep on non-triggering block number", func(t *testing.T) {
		head := models.NewHead(big.NewInt(21), eitest.NewHash(), eitest.NewHash(), 1000)
		chHeads <- &head

		select {
		case <-time.NewTimer(2 * time.Second).C:
		case <-chJobWasRun:
			t.Fatal("new job not supposed to run")
		}
	})

	clMock.AssertExpectations(t)
	ethMock.AssertExpectations(t)
}

func Test_UpkeepExecuter_PerformsUpkeep_Error(t *testing.T) {
	db, executer, clMock, ethMock, cleanup := setupExecuter(t)
	defer cleanup()
	getHeadsChannel := setupHeadsSubscription(ethMock)

	err := executer.Start()
	require.NoError(t, err)
	defer executer.Stop()
	chHeads := getHeadsChannel()

	reg := newRegistry()
	err = db.Create(&reg).Error
	require.NoError(t, err)

	upkeep := newRegistration(reg, 0)
	err = db.Create(&upkeep).Error
	require.NoError(t, err)

	chUpkeepCalled := make(chan struct{})
	ethMock.
		On("CallContract", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("err")).
		Run(func(args mock.Arguments) {
			chUpkeepCalled <- struct{}{}
		})

	head := models.NewHead(big.NewInt(20), eitest.NewHash(), eitest.NewHash(), 1000)
	chHeads <- &head

	select {
	case <-time.NewTimer(2 * time.Second).C:
		t.Fatal("checkUpkeep never called")
	case <-chUpkeepCalled:
	}

	clMock.AssertExpectations(t)
	ethMock.AssertExpectations(t)
}
