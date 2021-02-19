package keeper

import (
	"github.com/jinzhu/gorm"
	"github.com/smartcontractkit/chainlink/core/store/models"
)

type Store interface {
	Registries() ([]registry, error)
	UpsertRegistry(registry registry) error
	UpsertUpkeep(registration) error
	BatchDeleteUpkeeps(registryID uint32, upkeedIDs []uint64) error
	DeleteRegistryByJobID(jobID *models.ID) error
	EligibleUpkeeps(blockNumber uint64) ([]registration, error)
	NextUpkeepIDForRegistry(registry registry) (uint64, error)
	DB() *gorm.DB
	Close() error
}

func NewStore(dbClient *gorm.DB) Store {
	return keeperStore{
		dbClient: dbClient,
	}
}

type keeperStore struct {
	dbClient *gorm.DB
}

func (rm keeperStore) Registries() (registries []registry, _ error) {
	err := rm.dbClient.Find(&registries).Error
	return registries, err
}

func (rm keeperStore) UpsertRegistry(registry registry) error {
	return rm.dbClient.Save(&registry).Error
}

func (rm keeperStore) UpsertUpkeep(registration registration) error {
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

func (rm keeperStore) BatchDeleteUpkeeps(registryID uint32, upkeedIDs []uint64) error {
	return rm.dbClient.
		Where("registry_id = ? AND upkeep_id IN (?)", registryID, upkeedIDs).
		Delete(registration{}).
		Error
}

func (rm keeperStore) DeleteRegistryByJobID(jobID *models.ID) error {
	return rm.dbClient.
		Where("job_id = ?", jobID).
		Delete(registry{}).
		Error
}

func (rm keeperStore) EligibleUpkeeps(blockNumber uint64) (result []registration, _ error) {
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

// NextUpkeepIDForRegistry returns the largest upkeepID + 1, indicating the expected next upkeepID
// to sync from the contract
func (rm keeperStore) NextUpkeepIDForRegistry(reg registry) (nextID uint64, err error) {
	err = rm.dbClient.
		Model(&registration{}).
		Where("registry_id = ?", reg.ID).
		Select("coalesce(max(upkeep_id), -1) + 1").
		Row().
		Scan(&nextID)
	return nextID, err
}

func (rm keeperStore) DB() *gorm.DB {
	return rm.dbClient
}

func (rm keeperStore) Close() error {
	return rm.dbClient.Close()
}
