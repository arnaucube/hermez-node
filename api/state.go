package api

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hermeznetwork/hermez-node/common"
	"github.com/hermeznetwork/hermez-node/db/historydb"
	"github.com/hermeznetwork/tracerr"
)

func (a *API) getState(c *gin.Context) {
	stateAPI, err := a.h.GetStateAPI()
	if err != nil {
		retBadReq(err, c)
		return
	}
	c.JSON(http.StatusOK, stateAPI)
}

type APIStateUpdater struct {
	hdb    *historydb.HistoryDB
	state  historydb.StateAPI
	config historydb.NodeConfig
	vars   common.SCVariablesPtr
	consts historydb.Constants
}

func NewAPIStateUpdater(hdb *historydb.HistoryDB, config *historydb.NodeConfig, vars *common.SCVariables,
	consts *historydb.Constants) *APIStateUpdater {
	u := APIStateUpdater{
		hdb:    hdb,
		config: *config,
		consts: *consts,
	}
	u.SetSCVars(&common.SCVariablesPtr{&vars.Rollup, &vars.Auction, &vars.WDelayer})
	return &u
}

func (u *APIStateUpdater) Store() error {
	return tracerr.Wrap(u.hdb.SetAPIState(&u.state))
}

func (u *APIStateUpdater) SetSCVars(vars *common.SCVariablesPtr) {
	if vars.Rollup != nil {
		u.vars.Rollup = vars.Rollup
		rollupVars := historydb.NewRollupVariablesAPI(u.vars.Rollup)
		u.state.Rollup = *rollupVars
	}
	if vars.Auction != nil {
		u.vars.Auction = vars.Auction
		auctionVars := historydb.NewAuctionVariablesAPI(u.vars.Auction)
		u.state.Auction = *auctionVars
	}
	if vars.WDelayer != nil {
		u.vars.WDelayer = vars.WDelayer
		u.state.WithdrawalDelayer = *u.vars.WDelayer
	}
}

func (u *APIStateUpdater) UpdateMetrics() error {
	if u.state.Network.LastBatch == nil {
		return nil
	}
	lastBatchNum := u.state.Network.LastBatch.BatchNum
	metrics, err := u.hdb.GetMetricsInternalAPI(lastBatchNum)
	if err != nil {
		return tracerr.Wrap(err)
	}
	u.state.Metrics = *metrics
	return nil
}

func (u *APIStateUpdater) UpdateNetworkInfoBlock(lastEthBlock, lastSyncBlock common.Block) {
	u.state.Network.LastSyncBlock = lastSyncBlock.Num
	u.state.Network.LastEthBlock = lastEthBlock.Num
}

func (u *APIStateUpdater) UpdateNetworkInfo(
	lastEthBlock, lastSyncBlock common.Block,
	lastBatchNum common.BatchNum, currentSlot int64,
) error {
	// Get last batch in API format
	lastBatch, err := u.hdb.GetBatchInternalAPI(lastBatchNum)
	if tracerr.Unwrap(err) == sql.ErrNoRows {
		lastBatch = nil
	} else if err != nil {
		return tracerr.Wrap(err)
	}
	// Get next forgers
	lastClosedSlot := currentSlot + int64(u.state.Auction.ClosedAuctionSlots)
	nextForgers, err := u.hdb.GetNextForgersInternalAPI(u.vars.Auction, &u.consts.Auction,
		lastSyncBlock, currentSlot, lastClosedSlot)
	if tracerr.Unwrap(err) == sql.ErrNoRows {
		nextForgers = nil
	} else if err != nil {
		return tracerr.Wrap(err)
	}

	bucketUpdates, err := u.hdb.GetBucketUpdatesInternalAPI()
	if err == sql.ErrNoRows {
		bucketUpdates = nil
	} else if err != nil {
		return tracerr.Wrap(err)
	}
	// Update NodeInfo struct
	for i, bucketParams := range u.state.Rollup.Buckets {
		for _, bucketUpdate := range bucketUpdates {
			if bucketUpdate.NumBucket == i {
				bucketParams.Withdrawals = bucketUpdate.Withdrawals
				u.state.Rollup.Buckets[i] = bucketParams
				break
			}
		}
	}
	u.state.Network.LastSyncBlock = lastSyncBlock.Num
	u.state.Network.LastEthBlock = lastEthBlock.Num
	u.state.Network.LastBatch = lastBatch
	u.state.Network.CurrentSlot = currentSlot
	u.state.Network.NextForgers = nextForgers
	return nil
}
