package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeleteController(t *testing.T) {
	// TODO
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
	key := "testKey"
	secret := "testSecretAbcdæøå"

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
