package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smartcontractkit/chainlink/core/store/models"
	"github.com/smartcontractkit/external-initiator/blockchain"
	"github.com/smartcontractkit/external-initiator/eitest"
	"github.com/smartcontractkit/external-initiator/keeper"
	"github.com/smartcontractkit/external-initiator/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	key    = "testKey"
	secret = "testSecretAbcdæøå"
)

func TestCreateController(t *testing.T) {
	dbClient, cleanup := store.SetupTestDB(t)
	regStore := keeper.NewStore(dbClient.DB())
	defer cleanup()

	srv := &HttpService{
		AccessKey: key,
		Secret:    secret,
		Store:     regStore,
	}
	srv.createRouter()

	jobID := models.NewID().String()
	requestData := CreateSubscriptionReq{
		JobID: jobID,
		Params: blockchain.Params{
			Address: eitest.NewAddress().Hex(),
			From:    eitest.NewAddress().Hex(),
		},
	}

	requestBytes, err := json.Marshal(requestData)
	require.NoError(t, err)

	request := httptest.NewRequest("POST", "http://localhost:8080/jobs", bytes.NewReader(requestBytes))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Add(ExternalInitiatorAccessKeyHeader, key)
	request.Header.Add(ExternalInitiatorSecretHeader, secret)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, request)
	require.Equal(t, 201, w.Code)

	var respJSON map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &respJSON)
	require.NoError(t, err)
}

func TestDeleteController(t *testing.T) {
	dbClient, cleanup := store.SetupTestDB(t)
	regStore := keeper.NewStore(dbClient.DB())
	defer cleanup()

	jobID := models.NewID()
	reg := keeper.NewRegistry(
		eitest.NewAddress(),
		eitest.NewAddress(),
		jobID,
	)
	err := regStore.UpsertRegistry(reg)
	require.NoError(t, err)

	srv := &HttpService{
		AccessKey: key,
		Secret:    secret,
		Store:     regStore,
	}
	srv.createRouter()

	t.Run("deletes existing jobs", func(t *testing.T) {
		path := fmt.Sprintf("http://localhost:8080/jobs/%s", jobID.String())
		request := httptest.NewRequest("DELETE", path, nil)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Add(ExternalInitiatorAccessKeyHeader, key)
		request.Header.Add(ExternalInitiatorSecretHeader, secret)

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, request)
		require.Equal(t, 200, w.Code)

		var respJSON map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &respJSON)
		require.NoError(t, err)
	})

	t.Run("errors for non-existing jobs", func(t *testing.T) {
		path := "http://localhost:8080/jobs/DNE"
		request := httptest.NewRequest("DELETE", path, nil)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Add(ExternalInitiatorAccessKeyHeader, key)
		request.Header.Add(ExternalInitiatorSecretHeader, secret)

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, request)
		require.Equal(t, 500, w.Code)

		var respJSON map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &respJSON)
		require.NoError(t, err)
	})
}

func TestHealthController(t *testing.T) {
	tests := []struct {
		Name       string
		StatusCode int
	}{
		{
			"Is healthy",
			http.StatusOK,
		},
	}
	for _, test := range tests {
		srv := &HttpService{}
		srv.createRouter()

		req := httptest.NewRequest("GET", "/health", nil)

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		assert.Equal(t, test.StatusCode, w.Code)

		var respJSON map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &respJSON)
		assert.NoError(t, err)
	}
}

func TestRequireAuth(t *testing.T) {
	tests := []struct {
		Name   string
		Method string
		Target string
		Auth   bool
	}{
		{
			"Health is open",
			"GET",
			"/health",
			false,
		},
		{
			"Creating jobs is protected",
			"POST",
			"/jobs",
			true,
		},
		{
			"Deleting jobs is protected",
			"DELETE",
			"/jobs/test",
			true,
		},
	}

	srv := &HttpService{
		AccessKey: key,
		Secret:    secret,
	}
	srv.createRouter()

	for _, test := range tests {
		req := httptest.NewRequest(test.Method, test.Target, nil)

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if test.Auth {
			assert.Equal(t, http.StatusUnauthorized, w.Code)

			req.Header.Set(ExternalInitiatorAccessKeyHeader, key)
			req.Header.Set(ExternalInitiatorSecretHeader, secret)

			w = httptest.NewRecorder()
			srv.ServeHTTP(w, req)
			assert.NotEqual(t, http.StatusUnauthorized, w.Code)
		} else {
			assert.NotEqual(t, http.StatusUnauthorized, w.Code)
		}
	}
}

func TestValidateKeeperRequest_Error(t *testing.T) {
	for _, request := range []CreateSubscriptionReq{
		{ // missing From
			JobID: models.NewID().String(),
			Params: blockchain.Params{
				Address: eitest.NewAddress().Hex(),
			},
		},
		{ // missing Address
			JobID: models.NewID().String(),
			Params: blockchain.Params{
				From: eitest.NewAddress().Hex(),
			},
		},
		{ // missing JobID
			Params: blockchain.Params{
				From:    eitest.NewAddress().Hex(),
				Address: eitest.NewAddress().Hex(),
			},
		},
		{ // invalid JobID
			JobID: "invalid",
			Params: blockchain.Params{
				From:    eitest.NewAddress().Hex(),
				Address: eitest.NewAddress().Hex(),
			},
		},
		{ // invalid From
			JobID: models.NewID().String(),
			Params: blockchain.Params{
				From:    "0x1234",
				Address: eitest.NewAddress().Hex(),
			},
		},
		{ // invalid Address
			JobID: models.NewID().String(),
			Params: blockchain.Params{
				From:    eitest.NewAddress().Hex(),
				Address: "0x1234",
			},
		},
	} {
		err := validateKeeperRequest(&request)
		require.Error(t, err)
	}
}

func TestValidateKeeperRequest_Happy(t *testing.T) {
	request := CreateSubscriptionReq{
		JobID: models.NewID().String(),
		Params: blockchain.Params{
			From:    eitest.NewAddress().Hex(),
			Address: eitest.NewAddress().Hex(),
		},
	}
	err := validateKeeperRequest(&request)
	require.NoError(t, err)
}
