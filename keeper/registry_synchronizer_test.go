package keeper

import (
	"testing"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/smartcontractkit/external-initiator/internal/mocks"
	"github.com/smartcontractkit/external-initiator/store"
)

const syncTime = 5 * time.Second

func setupRegistrySync(t *testing.T) (*gorm.DB, RegistrySynchronizer, func()) {
	db, cleanup := store.SetupTestDB(t)
	ethClient := new(mocks.Client)
	regStore := NewRegistrySynchronizer(db.DB(), ethClient, syncTime)
	return db.DB(), regStore, cleanup
}

func Test_RegistrySynchronizer_Start(t *testing.T) {

}
