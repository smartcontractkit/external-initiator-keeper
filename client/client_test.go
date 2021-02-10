package client

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func Test_validateParams(t *testing.T) {
	t.Run("fails on missing required fields", func(t *testing.T) {
		v := viper.New()
		v.Set("required", "")
		err := validateParams(v, []string{"required", "required2"})
		assert.Error(t, err)
	})

	t.Run("success with required fields", func(t *testing.T) {
		v := viper.New()
		v.Set("required", "value")
		v.Set("required2", "value")
		err := validateParams(v, []string{"required", "required2"})
		assert.NoError(t, err)
	})
}
