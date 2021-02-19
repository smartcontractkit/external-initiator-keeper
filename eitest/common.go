package eitest

import (
	"crypto/ecdsa"
	"crypto/rand"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jinzhu/gorm"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func NewIdentity(t *testing.T) *bind.TransactOpts {
	key, err := crypto.GenerateKey()
	require.NoError(t, err, "failed to generate ethereum identity")
	return mustNewSimulatedBackendKeyedTransactor(t, key)
}

func mustNewSimulatedBackendKeyedTransactor(t *testing.T, key *ecdsa.PrivateKey) *bind.TransactOpts {
	t.Helper()
	return mustNewKeyedTransactor(t, key, 1337)
}

func mustNewKeyedTransactor(t *testing.T, key *ecdsa.PrivateKey, chainID int64) *bind.TransactOpts {
	t.Helper()
	transactor, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(chainID))
	require.NoError(t, err)
	return transactor
}

func AssertCount(t *testing.T, db *gorm.DB, model interface{}, expected int) {
	var count int
	db.Model(model).Count(&count)
	require.Equal(t, expected, count)
}
