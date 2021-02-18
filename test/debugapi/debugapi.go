package debugapi

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/hermeznetwork/hermez-node/common"
	"github.com/hermeznetwork/hermez-node/db/historydb"
	"github.com/hermeznetwork/hermez-node/db/statedb"
	"github.com/hermeznetwork/hermez-node/log"
	"github.com/hermeznetwork/hermez-node/synchronizer"
	"github.com/hermeznetwork/tracerr"
)

func handleNoRoute(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{
		"error": "404 page not found",
	})
}

type errorMsg struct {
	Message string
}

func badReq(err error, c *gin.Context) {
	log.Errorw("Bad request", "err", err)
	c.JSON(http.StatusBadRequest, errorMsg{
		Message: err.Error(),
	})
}

const (
	statusUpdating = "updating"
	statusOK       = "ok"
)

type tokenBalances struct {
	sync.RWMutex
	Value struct {
		Status   string
		Block    *common.Block
		Batch    *common.Batch
		Balances map[common.TokenID]*big.Int
	}
}

func (t *tokenBalances) Update(historyDB *historydb.HistoryDB, sdb *statedb.StateDB) (err error) {
	var block *common.Block
	var batch *common.Batch
	var balances map[common.TokenID]*big.Int
	defer func() {
		t.Lock()
		if err == nil {
			t.Value.Status = statusOK
			t.Value.Block = block
			t.Value.Batch = batch
			t.Value.Balances = balances
		} else {
			t.Value.Status = fmt.Sprintf("tokenBalances.Update: %v", err)
			t.Value.Block = nil
			t.Value.Batch = nil
			t.Value.Balances = nil
		}
		t.Unlock()
	}()

	if block, err = historyDB.GetLastBlock(); err != nil {
		return tracerr.Wrap(err)
	}
	if batch, err = historyDB.GetLastBatch(); err != nil {
		return tracerr.Wrap(err)
	}
	balances = make(map[common.TokenID]*big.Int)
	sdb.LastRead(func(sdbLast *statedb.Last) error {
		return tracerr.Wrap(
			statedb.AccountsIter(sdbLast.DB(), func(a *common.Account) (bool, error) {
				if balance, ok := balances[a.TokenID]; !ok {
					balances[a.TokenID] = a.Balance
				} else {
					balance.Add(balance, a.Balance)
				}
				return true, nil
			}),
		)
	})
	return nil
}

// DebugAPI is an http API with debugging endpoints
type DebugAPI struct {
	addr          string
	historyDB     *historydb.HistoryDB
	stateDB       *statedb.StateDB // synchronizer statedb
	sync          *synchronizer.Synchronizer
	tokenBalances tokenBalances
}

// NewDebugAPI creates a new DebugAPI
func NewDebugAPI(addr string, historyDB *historydb.HistoryDB, stateDB *statedb.StateDB,
	sync *synchronizer.Synchronizer) *DebugAPI {
	return &DebugAPI{
		addr:      addr,
		historyDB: historyDB,
		stateDB:   stateDB,
		sync:      sync,
	}
}

// SyncBlockHook is a hook function that the node will call after every new synchronized block
func (a *DebugAPI) SyncBlockHook() {
	a.tokenBalances.RLock()
	updateTokenBalances := a.tokenBalances.Value.Status == statusUpdating
	a.tokenBalances.RUnlock()
	if updateTokenBalances {
		if err := a.tokenBalances.Update(a.historyDB, a.stateDB); err != nil {
			log.Errorw("DebugAPI.tokenBalances.Upate", "err", err)
		}
	}
}

func (a *DebugAPI) handleTokenBalances(c *gin.Context) {
	a.tokenBalances.RLock()
	tokenBalances := a.tokenBalances.Value
	a.tokenBalances.RUnlock()
	c.JSON(http.StatusOK, tokenBalances)
}

func (a *DebugAPI) handlePostTokenBalances(c *gin.Context) {
	a.tokenBalances.Lock()
	a.tokenBalances.Value.Status = statusUpdating
	a.tokenBalances.Unlock()
	c.JSON(http.StatusOK, nil)
}

func (a *DebugAPI) handleAccount(c *gin.Context) {
	uri := struct {
		Idx uint32
	}{}
	if err := c.ShouldBindUri(&uri); err != nil {
		badReq(err, c)
		return
	}
	account, err := a.stateDB.LastGetAccount(common.Idx(uri.Idx))
	if err != nil {
		badReq(err, c)
		return
	}
	c.JSON(http.StatusOK, account)
}

func (a *DebugAPI) handleAccounts(c *gin.Context) {
	var accounts []common.Account
	if err := a.stateDB.LastRead(func(sdb *statedb.Last) error {
		var err error
		accounts, err = sdb.GetAccounts()
		return err
	}); err != nil {
		badReq(err, c)
		return
	}
	c.JSON(http.StatusOK, accounts)
}

func (a *DebugAPI) handleCurrentBatch(c *gin.Context) {
	batchNum, err := a.stateDB.LastGetCurrentBatch()
	if err != nil {
		badReq(err, c)
		return
	}
	c.JSON(http.StatusOK, batchNum)
}

func (a *DebugAPI) handleMTRoot(c *gin.Context) {
	root, err := a.stateDB.LastMTGetRoot()
	if err != nil {
		badReq(err, c)
		return
	}
	c.JSON(http.StatusOK, root)
}

func (a *DebugAPI) handleSyncStats(c *gin.Context) {
	stats := a.sync.Stats()
	c.JSON(http.StatusOK, stats)
}

// Run starts the http server of the DebugAPI.  To stop it, pass a context with
// cancelation (see `debugapi_test.go` for an example).
func (a *DebugAPI) Run(ctx context.Context) error {
	api := gin.Default()
	api.NoRoute(handleNoRoute)
	api.Use(cors.Default())
	debugAPI := api.Group("/debug")

	debugAPI.GET("sdb/batchnum", a.handleCurrentBatch)
	debugAPI.GET("sdb/mtroot", a.handleMTRoot)
	// Accounts returned by these endpoints will always have BatchNum = 0,
	// because the stateDB doesn't store the BatchNum in which an account
	// is created.
	debugAPI.GET("sdb/accounts", a.handleAccounts)
	debugAPI.GET("sdb/accounts/:Idx", a.handleAccount)
	debugAPI.POST("sdb/tokenbalances", a.handlePostTokenBalances)
	debugAPI.GET("sdb/tokenbalances", a.handleTokenBalances)

	debugAPI.GET("sync/stats", a.handleSyncStats)

	debugAPIServer := &http.Server{
		Addr:    a.addr,
		Handler: api,
		// Use some hardcoded numberes that are suitable for testing
		ReadTimeout:    30 * time.Second, //nolint:gomnd
		WriteTimeout:   30 * time.Second, //nolint:gomnd
		MaxHeaderBytes: 1 << 20,          //nolint:gomnd
	}
	go func() {
		log.Infof("DebugAPI is ready at %v", a.addr)
		if err := debugAPIServer.ListenAndServe(); err != nil && tracerr.Unwrap(err) != http.ErrServerClosed {
			log.Fatalf("Listen: %s\n", err)
		}
	}()

	<-ctx.Done()
	log.Info("Stopping DebugAPI...")
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 10*time.Second) //nolint:gomnd
	defer cancel()
	if err := debugAPIServer.Shutdown(ctxTimeout); err != nil {
		return tracerr.Wrap(err)
	}
	log.Info("DebugAPI done")
	return nil
}
