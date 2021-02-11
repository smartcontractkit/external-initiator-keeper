package eitest

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jinzhu/gorm"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
)

const (
	DBWaitTimeout     = 10 * time.Second
	DBPollingInterval = 100 * time.Millisecond
)

func Must(err error) {
	if err != nil {
		panic(err)
	}
}

type closeable interface {
	Close() error
}

func MustClose(toClose closeable) {
	Must(toClose.Close())
}

func WaitForCount(t *testing.T, db *gorm.DB, model interface{}, expected uint) {
	g := gomega.NewGomegaWithT(t)
	g.Eventually(func() uint {
		var count uint
		err := db.Model(model).Count(&count).Error
		assert.NoError(t, err)
		return count
	}, DBWaitTimeout, DBPollingInterval).Should(gomega.Equal(expected))
}

func NewHash() common.Hash {
	return common.BytesToHash(randomBytes(32))
}

func NewAddress() common.Address {
	return common.BytesToAddress(randomBytes(20))
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}
