package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/smartcontractkit/chainlink/core/store/models"
	"github.com/smartcontractkit/external-initiator/blockchain"
	"github.com/smartcontractkit/external-initiator/keeper"
)

const (
	externalInitiatorAccessKeyHeader = "X-Chainlink-EA-AccessKey"
	externalInitiatorSecretHeader    = "X-Chainlink-EA-Secret"
)

// RunWebserver starts a new web server using the access key
// and secret as provided on protected routes.
func RunWebserver(
	accessKey, secret string,
	regStore keeper.RegistryStore,
	port int,
) {
	srv := NewHTTPService(accessKey, secret, regStore)
	addr := fmt.Sprintf(":%v", port)
	err := srv.Router.Run(addr)
	if err != nil {
		logger.Error(err)
	}
}

// HttpService encapsulates router, EI service
// and access credentials.
type HttpService struct {
	Router        *gin.Engine
	AccessKey     string
	Secret        string
	RegistryStore keeper.RegistryStore
}

// NewHTTPService creates a new HttpService instance
// with the default router.
func NewHTTPService(
	accessKey, secret string,
	regStore keeper.RegistryStore,
) *HttpService {
	srv := HttpService{
		AccessKey:     accessKey,
		Secret:        secret,
		RegistryStore: regStore,
	}
	srv.createRouter()
	return &srv
}

// ServeHTTP calls ServeHTTP on the underlying router,
// which conforms to the http.Handler interface.
func (srv *HttpService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	srv.Router.ServeHTTP(w, r)
}

func (srv *HttpService) createRouter() {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(loggerFunc())
	r.GET("/health", srv.ShowHealth)

	auth := r.Group("/")
	auth.Use(authenticate(srv.AccessKey, srv.Secret))
	{
		auth.POST("/jobs", srv.CreateSubscription)
		auth.DELETE("/jobs/:jobid", srv.DeleteSubscription)
	}

	srv.Router = r
}

func authenticate(accessKey, secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		reqAccessKey := c.GetHeader(externalInitiatorAccessKeyHeader)
		reqSecret := c.GetHeader(externalInitiatorSecretHeader)
		if reqAccessKey == accessKey && reqSecret == secret {
			c.Next()
		} else {
			c.AbortWithStatus(http.StatusUnauthorized)
		}
	}
}

// CreateSubscriptionReq holds the payload expected for job POSTs
// from the Chainlink node.
type CreateSubscriptionReq struct {
	JobID  string            `json:"jobId"`
	Type   string            `json:"type"`
	Params blockchain.Params `json:"params"`
}

type resp struct {
	ID string `json:"id"`
}

// CreateSubscription expects a CreateSubscriptionReq payload,
// validates the request and subscribes to the job.
func (srv *HttpService) CreateSubscription(c *gin.Context) {
	var req CreateSubscriptionReq

	if err := c.BindJSON(&req); err != nil {
		logger.Error(err)
		c.JSON(http.StatusBadRequest, nil)
		return
	}

	// HACK - making an exception to the normal workflow for keepers
	// since they will be removed from EI at a later date
	srv.createKeeperSubscription(req, c)
}

// DeleteSubscription deletes any job with the jobid
// provided as parameter in the request.
func (srv *HttpService) DeleteSubscription(c *gin.Context) {
	// XXX: this now only works for keeper jobs
	jobID, err := models.NewIDFromString(c.Param("jobid"))
	if err != nil {
		logger.Error(err)
		c.JSON(http.StatusInternalServerError, nil)
		return
	}
	if err := srv.RegistryStore.DeleteRegistryByJobID(jobID); err != nil {
		logger.Error(err)
		c.JSON(http.StatusInternalServerError, nil)
		return
	}

	c.JSON(http.StatusOK, resp{ID: jobID.String()})
}

// ShowHealth returns the following when online:
//  {"chainlink": true}
func (srv *HttpService) ShowHealth(c *gin.Context) {
	c.JSON(200, gin.H{"chainlink": true})
}

// Inspired by https://github.com/gin-gonic/gin/issues/961
func loggerFunc() gin.HandlerFunc {
	return func(c *gin.Context) {
		buf, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			logger.Error("Web request log error: ", err.Error())
			// Implicitly relies on limits.RequestSizeLimiter
			// overriding of c.Request.Body to abort gin's Context
			// inside ioutil.ReadAll.
			// Functions as we would like, but horrible from an architecture
			// and design pattern perspective.
			if !c.IsAborted() {
				c.AbortWithStatus(http.StatusBadRequest)
			}
			return
		}
		rdr := bytes.NewBuffer(buf)
		c.Request.Body = ioutil.NopCloser(bytes.NewBuffer(buf))

		start := time.Now()
		c.Next()
		end := time.Now()

		logger.Infow(fmt.Sprintf("%s %s", c.Request.Method, c.Request.URL.Path),
			"method", c.Request.Method,
			"status", c.Writer.Status(),
			"path", c.Request.URL.Path,
			"query", c.Request.URL.Query(),
			"body", readBody(rdr),
			"clientIP", c.ClientIP(),
			"errors", c.Errors.String(),
			"servedAt", end.Format("2006-01-02 15:04:05"),
			"latency", fmt.Sprint(end.Sub(start)),
		)
	}
}

func readBody(reader io.Reader) string {
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(reader)
	if err != nil {
		logger.Warn("unable to read from body for sanitization: ", err)
		return "*FAILED TO READ BODY*"
	}

	if buf.Len() == 0 {
		return ""
	}

	s, err := readSanitizedJSON(buf)
	if err != nil {
		logger.Warn("unable to sanitize json for logging: ", err)
		return "*FAILED TO READ BODY*"
	}
	return s
}

func readSanitizedJSON(buf *bytes.Buffer) (string, error) {
	var dst map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &dst)
	if err != nil {
		return "", err
	}

	b, err := json.Marshal(dst)
	if err != nil {
		return "", err
	}
	return string(b), err
}

func (srv *HttpService) createKeeperSubscription(req CreateSubscriptionReq, c *gin.Context) {
	if err := validateKeeperRequest(&req); err != nil {
		logger.Error(err)
		c.JSON(http.StatusBadRequest, nil)
		return
	}

	jobID, err := models.NewIDFromString(req.JobID)
	if err != nil {
		logger.Error(err)
		c.JSON(http.StatusInternalServerError, nil)
		return
	}
	address := common.HexToAddress(req.Params.Address)
	from := common.HexToAddress(req.Params.From)
	reg := keeper.NewRegistry(address, from, jobID)
	err = srv.RegistryStore.DB().Create(&reg).Error
	if err != nil {
		logger.Error(err)
		c.JSON(http.StatusInternalServerError, nil)
		return
	}

	c.JSON(http.StatusCreated, resp{ID: reg.ReferenceID})
}

func validateKeeperRequest(req *CreateSubscriptionReq) error {
	if req.Params.Address == "" || req.Params.From == "" || req.JobID == "" {
		return errors.New("missing required fields")
	}
	return nil
}
