package integration

import (
	"bytes"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind/backends"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/google/uuid"
	"github.com/onsi/gomega"
	"github.com/smartcontractkit/external-initiator/blockchain"
	"github.com/smartcontractkit/external-initiator/client"
	"github.com/smartcontractkit/external-initiator/eitest"
	"github.com/smartcontractkit/external-initiator/internal/mocks"
	"github.com/smartcontractkit/external-initiator/keeper/basic_upkeep_contract"
	"github.com/smartcontractkit/external-initiator/keeper/keeper_registry_contract"
	"github.com/smartcontractkit/external-initiator/keeper/mock_v3_aggregator_contract"
	"github.com/smartcontractkit/external-initiator/store"
	"github.com/smartcontractkit/libocr/gethwrappers/link_token_interface"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var oneEth = big.NewInt(1000000000000000000)
var oneHunEth = big.NewInt(0).Mul(oneEth, big.NewInt(100))

func TestKeeperEthIntegration(t *testing.T) {
	// setup db
	db, cleanup := store.SetupTestDB(t)
	defer cleanup()

	// setup blockchain
	sergey := eitest.NewIdentity(t) // owns all the link
	steve := eitest.NewIdentity(t)  // registry owner
	carrol := eitest.NewIdentity(t) // client
	neil := eitest.NewIdentity(t)   // node operator
	genesisData := core.GenesisAlloc{
		sergey.From: {Balance: oneEth},
		steve.From:  {Balance: oneEth},
		carrol.From: {Balance: oneEth},
		neil.From:   {Balance: oneEth},
	}

	ethClient, backend := eitest.NewClientWithSimulatedBackend(t, genesisData)

	linkAddr, _, linkToken, err := link_token_interface.DeployLinkToken(sergey, backend)
	require.NoError(t, err)
	gasFeedAddr, _, _, err := mock_v3_aggregator_contract.DeployMockV3AggregatorContract(steve, backend, 18, big.NewInt(60000000000))
	require.NoError(t, err)
	linkFeedAddr, _, _, err := mock_v3_aggregator_contract.DeployMockV3AggregatorContract(steve, backend, 18, big.NewInt(20000000000000000))
	require.NoError(t, err)
	regAddr, _, registryContract, err := keeper_registry_contract.DeployKeeperRegistryContract(steve, backend, linkAddr, linkFeedAddr, gasFeedAddr, 250_000_000, big.NewInt(3), 20_000_000, big.NewInt(3600), big.NewInt(60000000000), big.NewInt(20000000000000000))
	require.NoError(t, err)
	upkeepAddr, _, upkeepContract, err := basic_upkeep_contract.DeployBasicUpkeepContract(carrol, backend)
	require.NoError(t, err)
	_, err = linkToken.Transfer(sergey, carrol.From, oneHunEth)
	require.NoError(t, err)
	_, err = linkToken.Approve(carrol, regAddr, oneHunEth)
	require.NoError(t, err)
	_, err = registryContract.SetKeepers(steve, []common.Address{neil.From}, []common.Address{neil.From})
	require.NoError(t, err)
	_, err = registryContract.RegisterUpkeep(steve, upkeepAddr, 2_500_000, carrol.From, common.Hex2Bytes("0x"))
	require.NoError(t, err)
	_, err = upkeepContract.SetBytesToSend(carrol, common.Hex2Bytes("0x1234"))
	require.NoError(t, err)
	_, err = upkeepContract.SetShouldPerformUpkeep(carrol, true)
	require.NoError(t, err)
	backend.Commit()
	_, err = registryContract.AddFunds(carrol, big.NewInt(0), oneHunEth)
	require.NoError(t, err)
	backend.Commit()

	stopMining := mine(backend)
	defer stopMining()

	// setup keeper service
	clMock := new(mocks.ChainlinkClient)
	httpClient := &http.Client{}

	config := client.Config{
		ChainlinkToInitiatorAccessKey: "key",
		ChainlinkToInitiatorSecret:    "secret",
		Port:                          8080,
		KeeperRegistrySyncInterval:    1 * time.Second,
	}

	keeperService := client.NewService(db, clMock, ethClient, config)

	err = keeperService.Run()
	require.NoError(t, err)
	defer keeperService.Close()

	// wait until server ready
	g := gomega.NewGomegaWithT(t)
	g.Eventually(func() error {
		request, err := http.NewRequest(http.MethodGet, "http://localhost:8080/health", nil)
		require.NoError(t, err)
		_, err = httpClient.Do(request)
		return err
	}, 2*time.Second, 100*time.Millisecond).Should(gomega.BeNil())

	// create job
	jobID := strings.ReplaceAll(uuid.New().String(), "-", "")
	requestData := client.CreateSubscriptionReq{
		JobID: jobID,
		Params: blockchain.Params{
			Address: regAddr.Hex(),
			From:    neil.From.Hex(),
		},
	}

	requestBytes, err := json.Marshal(requestData)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, "http://localhost:8080/jobs", bytes.NewReader(requestBytes))
	require.NoError(t, err)

	request.Header.Set("Content-Type", "application/json")
	request.Header.Add(client.ExternalInitiatorAccessKeyHeader, config.ChainlinkToInitiatorAccessKey)
	request.Header.Add(client.ExternalInitiatorSecretHeader, config.ChainlinkToInitiatorSecret)

	response, err := httpClient.Do(request)
	require.NoError(t, err)
	require.Equal(t, 201, response.StatusCode)

	// test for job run
	chJobWasRun := make(chan struct{})
	clMock.
		On("TriggerJob", jobID, mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			chJobWasRun <- struct{}{}
		})

	select {
	case <-time.NewTimer(30 * time.Second).C:
		t.Fatal("job run never triggered")
	case <-chJobWasRun:
	}

}

func mine(backend *backends.SimulatedBackend) (stopMinning func()) {
	timer := time.NewTicker(2 * time.Second)
	chStop := make(chan struct{})
	go func() {
		for {
			select {
			case <-timer.C:
				backend.Commit()
			case <-chStop:
				return
			}
		}
	}()
	return func() { close(chStop); timer.Stop() }
}
