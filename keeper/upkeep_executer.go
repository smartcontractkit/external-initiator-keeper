package keeper

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/smartcontractkit/chainlink/core/services/eth"
	"github.com/smartcontractkit/chainlink/core/store/models"
	"github.com/smartcontractkit/chainlink/core/utils"
	"github.com/smartcontractkit/external-initiator/chainlink"
	"go.uber.org/atomic"
)

const (
	checkUpkeep        = "checkUpkeep"
	performUpkeep      = "performUpkeep"
	executionQueueSize = 10
	gasBuffer          = uint32(200_000)
	refreshInterval    = 5 * time.Second
)

var (
	performUpkeepHex = utils.AddHexPrefix(common.Bytes2Hex(UpkeepRegistryABI.Methods[performUpkeep].ID))
)

type UpkeepExecuter interface {
	Start() error
	Stop()
}

func NewUpkeepExecuter(keeperStore Store, clNode chainlink.Client, ethClient eth.Client) UpkeepExecuter {
	return upkeepExecuter{
		blockHeight:    atomic.NewUint64(0),
		chainlinkNode:  clNode,
		ethClient:      ethClient,
		keeperStore:    keeperStore,
		isRunning:      atomic.NewBool(false),
		executionQueue: make(chan struct{}, executionQueueSize),
		chDone:         make(chan struct{}),
		chSignalRun:    make(chan struct{}, 1),
	}
}

type upkeepExecuter struct {
	blockHeight   *atomic.Uint64
	chainlinkNode chainlink.Client
	ethClient     eth.Client
	keeperStore   Store
	isRunning     *atomic.Bool

	executionQueue chan struct{}
	chDone         chan struct{}
	chSignalRun    chan struct{}
}

func (executer upkeepExecuter) Start() error {
	if executer.isRunning.Load() {
		return errors.New("already started")
	}
	executer.isRunning.Store(true)
	go executer.setRunsOnHeadSubscription()
	go executer.run()
	return nil
}

func (executer upkeepExecuter) Stop() {
	close(executer.chDone)
}

func (executer upkeepExecuter) run() {
	for {
		select {
		case <-executer.chDone:
			return
		case <-executer.chSignalRun:
			executer.processActiveRegistrations()
		}
	}
}

func (executer upkeepExecuter) processActiveRegistrations() {
	// Keepers could miss their turn in the turn taking algo if they are too overloaded
	// with work because processActiveRegistrations() blocks - this could be parallelized
	// but will need a cap
	logger.Debug("received new block, running checkUpkeep for keeper registrations")

	// TODO - RYAN - this should be batched to avoid congestgion
	activeRegistrations, err := executer.keeperStore.EligibleUpkeeps(executer.blockHeight.Load())
	if err != nil {
		logger.Errorf("unable to load active registrations: %v", err)
		return
	}

	for _, reg := range activeRegistrations {
		executer.concurrentExecute(reg)
	}
}

func (executer upkeepExecuter) concurrentExecute(registration registration) {
	executer.executionQueue <- struct{}{}
	go executer.execute(registration)
}

// execute will call checkForUpkeep and, if it succeeds, triger a job on the CL node
func (executer upkeepExecuter) execute(registration registration) {
	// pop queue when done executing
	defer func() {
		<-executer.executionQueue
	}()

	checkPayload, err := UpkeepRegistryABI.Pack(
		checkUpkeep,
		big.NewInt(int64(registration.UpkeepID)),
		registration.Registry.From,
	)
	if err != nil {
		logger.Error(err)
		return
	}

	msg := ethereum.CallMsg{
		From: utils.ZeroAddress,
		To:   &registration.Registry.Address,
		Gas:  uint64(registration.Registry.CheckGas),
		Data: checkPayload,
	}

	logger.Debugf("Checking upkeep on registry: %s, upkeepID %d", registration.Registry.Address.Hex(), registration.UpkeepID)

	result, err := executer.ethClient.CallContract(context.Background(), msg, nil)
	if err != nil {
		logger.Debugf("checkUpkeep failed on registry: %s, upkeepID %d", registration.Registry.Address.Hex(), registration.UpkeepID)
		return
	}

	res, err := UpkeepRegistryABI.Unpack(checkUpkeep, result)
	if err != nil {
		logger.Error(err)
		return
	}

	performData, ok := res[0].([]byte)
	if !ok {
		logger.Error("checkupkeep payload not as expected")
		return
	}

	performPayload, err := UpkeepRegistryABI.Pack(
		performUpkeep,
		big.NewInt(int64(registration.UpkeepID)),
		performData,
	)
	if err != nil {
		logger.Error(err)
		return
	}

	performPayloadString := utils.AddHexPrefix(common.Bytes2Hex(performPayload[4:]))

	chainlinkPayloadJSON := map[string]interface{}{
		"format":           "preformatted",
		"address":          registration.Registry.Address.Hex(),
		"functionSelector": performUpkeepHex,
		"result":           performPayloadString,
		"fromAddresses":    []string{registration.Registry.From.Hex()},
		"gasLimit":         registration.ExecuteGas + gasBuffer,
	}

	chainlinkPayload, err := json.Marshal(chainlinkPayloadJSON)
	if err != nil {
		logger.Error(err)
		return
	}

	logger.Debugf("Performing upkeep on registry: %s, upkeepID %d", registration.Registry.Address.Hex(), registration.UpkeepID)
	err = executer.chainlinkNode.TriggerJob(registration.Registry.JobID.String(), chainlinkPayload)
	if err != nil {
		logger.Errorf("Unable to trigger job on chainlink node: %v", err)
	}
}

func (executer upkeepExecuter) setRunsOnHeadSubscription() {
	headers := make(chan *models.Head)
	sub, err := executer.ethClient.SubscribeNewHead(context.Background(), headers)
	defer sub.Unsubscribe()
	if err != nil {
		logger.Fatal(err)
	}

	for {
		select {
		case <-executer.chDone:
			return
		case err := <-sub.Err():
			logger.Errorf("error in keeper head subscription: %v", err)
		case head := <-headers:
			executer.blockHeight.Store(uint64(head.Number))
			executer.signalRun()
		}
	}
}

func (executer upkeepExecuter) signalRun() {
	// avoid blocking if signal already in buffer
	select {
	case executer.chSignalRun <- struct{}{}:
	default:
	}
}
