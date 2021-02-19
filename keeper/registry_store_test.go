package keeper

import (
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jinzhu/gorm"
	"github.com/smartcontractkit/chainlink/core/store/models"
	"github.com/smartcontractkit/external-initiator/eitest"
	"github.com/smartcontractkit/external-initiator/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var registryAddress = common.HexToAddress("0x0000000000000000000000000000000000000123")
var fromAddress = common.HexToAddress("0x0000000000000000000000000000000000000ABC")
var checkDataStr = "ABC123"
var checkData = common.Hex2Bytes(checkDataStr)
var jobID = models.NewID()
var executeGas = uint32(10_000)
var checkGas = uint32(2_000_000)
var blockCountPerTurn = uint32(20)

func setupRegistryStore(t *testing.T) (*gorm.DB, RegistryStore, func()) {
	dbClient, cleanup := store.SetupTestDB(t)
	regStore := NewRegistryStore(dbClient.DB())
	return dbClient.DB(), regStore, cleanup
}

func newRegistry() registry {
	return registry{
		Address:           registryAddress,
		BlockCountPerTurn: blockCountPerTurn,
		CheckGas:          checkGas,
		From:              fromAddress,
		JobID:             jobID,
		KeeperIndex:       0,
		NumKeepers:        1,
		ReferenceID:       models.NewID().String(),
	}
}

func newRegistration(reg registry, upkeepID uint64) registration {
	return registration{
		UpkeepID:   upkeepID,
		ExecuteGas: executeGas,
		Registry:   reg,
		CheckData:  checkData,
	}
}

func TestRegistryStore_Registries(t *testing.T) {
	db, regStore, cleanup := setupRegistryStore(t)
	defer cleanup()

	reg := newRegistry()
	err := db.Create(&reg).Error
	require.NoError(t, err)

	reg2 := registry{
		Address:     common.HexToAddress("0x0000000000000000000000000000000000000456"),
		CheckGas:    checkGas,
		JobID:       models.NewID(),
		From:        fromAddress,
		ReferenceID: models.NewID().String(),
	}

	err = db.Create(&reg2).Error
	require.NoError(t, err)

	existingRegistries, err := regStore.Registries()
	require.NoError(t, err)
	require.Equal(t, 2, len(existingRegistries))
}

func TestRegistryStore_RegistryIDs(t *testing.T) {
	db, regStore, cleanup := setupRegistryStore(t)
	defer cleanup()

	db.LogMode(true)

	reg := newRegistry()
	err := db.Create(&reg).Error
	require.NoError(t, err)

	reg2 := registry{
		Address:     common.HexToAddress("0x0000000000000000000000000000000000000456"),
		CheckGas:    checkGas,
		JobID:       models.NewID(),
		From:        fromAddress,
		ReferenceID: models.NewID().String(),
	}

	err = db.Create(&reg2).Error
	require.NoError(t, err)

	ids, err := regStore.RegistryIDs()
	require.NoError(t, err)
	require.Equal(t, 2, len(ids))
	fmt.Println(ids)
}

func TestRegistryStore_Upsert(t *testing.T) {
	db, regStore, cleanup := setupRegistryStore(t)
	defer cleanup()

	// create registry
	reg := newRegistry()
	err := db.Create(&reg).Error
	require.NoError(t, err)

	// create registration
	newRegistration := newRegistration(reg, 0)
	err = regStore.Upsert(newRegistration)
	require.NoError(t, err)

	eitest.AssertCount(t, db, &registration{}, 1)
	var existingRegistration registration
	err = db.First(&existingRegistration).Error
	require.NoError(t, err)
	require.Equal(t, executeGas, existingRegistration.ExecuteGas)
	require.Equal(t, checkData, existingRegistration.CheckData)

	// update registration
	updatedRegistration := registration{
		Registry:   reg,
		UpkeepID:   0,
		ExecuteGas: 20_000,
		CheckData:  common.Hex2Bytes("8888"),
	}
	err = regStore.Upsert(updatedRegistration)
	require.NoError(t, err)
	eitest.AssertCount(t, db, &registration{}, 1)
	err = db.First(&existingRegistration).Error
	require.NoError(t, err)
	require.Equal(t, uint32(20_000), existingRegistration.ExecuteGas)
	require.Equal(t, "8888", common.Bytes2Hex(existingRegistration.CheckData))
}

func TestRegistryStore_BatchDelete(t *testing.T) {
	db, regStore, cleanup := setupRegistryStore(t)
	defer cleanup()

	reg := newRegistry()
	err := db.Create(&reg).Error
	require.NoError(t, err)

	registrations := [3]registration{
		newRegistration(reg, 0),
		newRegistration(reg, 1),
		newRegistration(reg, 2),
	}

	for _, reg := range registrations {
		err = db.Create(&reg).Error
		require.NoError(t, err)
	}

	eitest.AssertCount(t, db, &registration{}, 3)

	err = regStore.BatchDelete(reg.ID, []uint64{0, 2})
	require.NoError(t, err)

	eitest.AssertCount(t, db, &registration{}, 1)
}

func TestRegistryStore_DeleteRegistryByJobID(t *testing.T) {
	db, regStore, cleanup := setupRegistryStore(t)
	defer cleanup()

	reg := newRegistry()
	err := db.Create(&reg).Error
	require.NoError(t, err)

	registrations := [3]registration{
		newRegistration(reg, 0),
		newRegistration(reg, 1),
		newRegistration(reg, 2),
	}

	for _, reg := range registrations {
		err = db.Create(&reg).Error
		require.NoError(t, err)
	}

	eitest.AssertCount(t, db, &registration{}, 3)

	err = regStore.DeleteRegistryByJobID(reg.JobID)
	require.NoError(t, err)

	eitest.AssertCount(t, db, registry{}, 0)
	eitest.AssertCount(t, db, &registration{}, 0)
}

func TestRegistryStore_Eligibile_BlockCountPerTurn(t *testing.T) {
	db, regStore, cleanup := setupRegistryStore(t)
	defer cleanup()

	blockheight := uint64(40)

	// create registries
	reg1 := registry{
		Address:           common.HexToAddress("0x0000000000000000000000000000000000000123"),
		BlockCountPerTurn: 20,
		CheckGas:          checkGas,
		From:              fromAddress,
		JobID:             models.NewID(),
		KeeperIndex:       0,
		NumKeepers:        1,
		ReferenceID:       models.NewID().String(),
	}
	reg2 := registry{
		Address:           common.HexToAddress("0x0000000000000000000000000000000000000321"),
		BlockCountPerTurn: 30,
		CheckGas:          checkGas,
		From:              fromAddress,
		JobID:             models.NewID(),
		KeeperIndex:       0,
		NumKeepers:        1,
		ReferenceID:       models.NewID().String(),
	}
	err := db.Create(&reg1).Error
	require.NoError(t, err)
	err = db.Create(&reg2).Error
	require.NoError(t, err)

	registrations := [3]registration{
		{ // our turn
			UpkeepID:   0,
			ExecuteGas: executeGas,
			Registry:   reg1,
		}, { // our turn
			UpkeepID:   1,
			ExecuteGas: executeGas,
			Registry:   reg1,
		}, { // not our turn
			UpkeepID:   0,
			ExecuteGas: executeGas,
			Registry:   reg2,
		},
	}

	for _, reg := range registrations {
		err = regStore.Upsert(reg)
		require.NoError(t, err)
	}

	eitest.AssertCount(t, db, &registration{}, 3)

	elligibleRegistrations, err := regStore.Eligible(blockheight)
	assert.NoError(t, err)
	assert.Len(t, elligibleRegistrations, 2)
	assert.Equal(t, uint64(0), elligibleRegistrations[0].UpkeepID)
	assert.Equal(t, uint64(1), elligibleRegistrations[1].UpkeepID)

	// preloads registry data
	assert.Equal(t, reg1.ID, elligibleRegistrations[0].RegistryID)
	assert.Equal(t, reg1.ID, elligibleRegistrations[1].RegistryID)
	assert.Equal(t, reg1.CheckGas, elligibleRegistrations[0].Registry.CheckGas)
	assert.Equal(t, reg1.CheckGas, elligibleRegistrations[1].Registry.CheckGas)
	assert.Equal(t, reg1.Address, elligibleRegistrations[0].Registry.Address)
	assert.Equal(t, reg1.Address, elligibleRegistrations[1].Registry.Address)
}

func TestRegistryStore_Eligibile_KeepersRotate(t *testing.T) {
	db, regStore, cleanup := setupRegistryStore(t)
	defer cleanup()

	reg := registry{
		Address:           common.HexToAddress("0x0000000000000000000000000000000000000123"),
		BlockCountPerTurn: 20,
		CheckGas:          checkGas,
		From:              fromAddress,
		JobID:             models.NewID(),
		KeeperIndex:       0,
		NumKeepers:        5,
		ReferenceID:       models.NewID().String(),
	}

	err := db.Create(&reg).Error
	require.NoError(t, err)

	upkeep := newRegistration(reg, 0)
	err = regStore.Upsert(upkeep)
	require.NoError(t, err)

	eitest.AssertCount(t, db, registry{}, 1)
	eitest.AssertCount(t, db, &registration{}, 1)

	// out of 5 valid block heights, with 5 keepers, we are eligible
	// to submit on exactly 1 of them
	list1, err := regStore.Eligible(20) // someone eligible
	require.NoError(t, err)
	list2, err := regStore.Eligible(30) // noone eligible
	require.NoError(t, err)
	list3, err := regStore.Eligible(40) // someone eligible
	require.NoError(t, err)
	list4, err := regStore.Eligible(41) // noone eligible
	require.NoError(t, err)
	list5, err := regStore.Eligible(60) // someone eligible
	require.NoError(t, err)
	list6, err := regStore.Eligible(80) // someone eligible
	require.NoError(t, err)
	list7, err := regStore.Eligible(99) // noone eligible
	require.NoError(t, err)
	list8, err := regStore.Eligible(100) // someone eligible
	require.NoError(t, err)

	totalEligible := len(list1) + len(list2) + len(list3) + len(list4) + len(list5) + len(list6) + len(list7) + len(list8)
	require.Equal(t, 1, totalEligible)
}

func TestRegistryStore_NextUpkeepID(t *testing.T) {
	db, regStore, cleanup := setupRegistryStore(t)
	defer cleanup()

	reg := newRegistry()
	err := db.Create(&reg).Error
	require.NoError(t, err)

	nextID, err := regStore.NextUpkeepID(reg)
	require.NoError(t, err)
	require.Equal(t, uint64(0), nextID)

	upkeep := newRegistration(reg, 0)
	err = regStore.Upsert(upkeep)
	require.NoError(t, err)

	nextID, err = regStore.NextUpkeepID(reg)
	require.NoError(t, err)
	require.Equal(t, uint64(1), nextID)

	upkeep = newRegistration(reg, 3)
	err = regStore.Upsert(upkeep)
	require.NoError(t, err)

	nextID, err = regStore.NextUpkeepID(reg)
	require.NoError(t, err)
	require.Equal(t, uint64(4), nextID)
}
