package keeper

type registration struct {
	ID         int32 `gorm:"primary_key"`
	CheckData  []byte
	ExecuteGas uint32 `gorm:"default:null"`
	RegistryID uint32
	Registry   registry `gorm:"association_autoupdate:false"`
	UpkeepID   uint64
}

func (registration) TableName() string {
	return "keeper_registrations"
}
