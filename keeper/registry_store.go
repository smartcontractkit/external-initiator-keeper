package keeper

import (
	"github.com/jinzhu/gorm"
	"github.com/smartcontractkit/chainlink/core/store/models"
)

type RegistryStore interface {
	Registries() ([]registry, error)
	RegistryIDs() ([]uint32, error)
	UpdateRegistry(registry registry) error
	Upsert(registration) error
	BatchDelete(registryID uint32, upkeedIDs []uint64) error
	DeleteRegistryByJobID(jobID *models.ID) error
	Eligible(blockNumber uint64) ([]registration, error)
	NextUpkeepID(registry registry) (uint64, error)
	DB() *gorm.DB
	Close() error
}

func NewRegistryStore(dbClient *gorm.DB) RegistryStore {
	return registryStore{
		dbClient: dbClient,
	}
}

type registryStore struct {
	dbClient *gorm.DB
}

func (rm registryStore) Registries() (registries []registry, _ error) {
	err := rm.dbClient.Find(&registries).Error
	return registries, err
}

func (rm registryStore) RegistryIDs() ([]uint32, error) {
	var regs []registry
	err := rm.dbClient.Table("keeper_registries").Select("id").Find(&regs).Error
	ids := make([]uint32, len(regs))
	for idx, reg := range regs {
		ids[idx] = reg.ID
	}
	return ids, err
}

func (rm registryStore) UpdateRegistry(registry registry) error {
	return rm.dbClient.Save(&registry).Error
}

func (rm registryStore) Upsert(registration registration) error {
	return rm.dbClient.
		Set(
			"gorm:insert_option",
			`ON CONFLICT (registry_id, upkeep_id)
			DO UPDATE SET
				execute_gas = excluded.execute_gas,
				check_data = excluded.check_data
			`,
		).
		Create(&registration).
		Error
}

func (rm registryStore) BatchDelete(registryID uint32, upkeedIDs []uint64) error {
	return rm.dbClient.
		Where("registry_id = ? AND upkeep_id IN (?)", registryID, upkeedIDs).
		Delete(registration{}).
		Error
}

func (rm registryStore) DeleteRegistryByJobID(jobID *models.ID) error {
	return rm.dbClient.
		Where("job_id = ?", jobID).
		Delete(registry{}).
		Error
}

func (rm registryStore) Eligible(blockNumber uint64) (result []registration, _ error) {
	turnTakingQuery := `
		keeper_registries.keeper_index =
			(
				keeper_registrations.positioning_constant + (? / keeper_registries.block_count_per_turn)
			) % keeper_registries.num_keepers
	`

	err := rm.dbClient.
		Joins("INNER JOIN keeper_registries ON keeper_registries.id = keeper_registrations.registry_id").
		Where("? % keeper_registries.block_count_per_turn = 0", blockNumber).
		Where(turnTakingQuery, blockNumber).
		Find(&result).
		Error

	return result, err
}

// NextUpkeepID returns the largest upkeepID + 1, indicating the expected next upkeepID
// to sync from the contract
func (rm registryStore) NextUpkeepID(reg registry) (nextID uint64, err error) {
	err = rm.dbClient.
		Model(&registration{}).
		Where("registry_id = ?", reg.ID).
		Select("coalesce(max(upkeep_id), -1) + 1").
		Row().
		Scan(&nextID)
	return nextID, err
}

func (rm registryStore) DB() *gorm.DB {
	return rm.dbClient
}

func (rm registryStore) Close() error {
	return rm.dbClient.Close()
}
