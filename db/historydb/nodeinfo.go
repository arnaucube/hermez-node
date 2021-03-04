package historydb

import (
	"database/sql"
	"fmt"
	"math"
	"math/big"
	"time"

	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/hermeznetwork/hermez-node/apitypes"
	"github.com/hermeznetwork/hermez-node/common"
	"github.com/hermeznetwork/hermez-node/db"
	"github.com/hermeznetwork/tracerr"
	"github.com/jmoiron/sqlx"
	"github.com/russross/meddler"
)

const (
	createAccountExtraFeePercentage         float64 = 2
	createAccountInternalExtraFeePercentage float64 = 2.5
)

type Period struct {
	SlotNum       int64     `json:"slotNum"`
	FromBlock     int64     `json:"fromBlock"`
	ToBlock       int64     `json:"toBlock"`
	FromTimestamp time.Time `json:"fromTimestamp"`
	ToTimestamp   time.Time `json:"toTimestamp"`
}

type NextForger struct {
	Coordinator CoordinatorAPI `json:"coordinator"`
	Period      Period         `json:"period"`
}

type Network struct {
	LastEthBlock  int64        `json:"lastEthereumBlock"`
	LastSyncBlock int64        `json:"lastSynchedBlock"`
	LastBatch     *BatchAPI    `json:"lastBatch"`
	CurrentSlot   int64        `json:"currentSlot"`
	NextForgers   []NextForger `json:"nextForgers"`
}

// NodePublicConfig is the configuration of the node that is exposed via API
type NodePublicConfig struct {
	// ForgeDelay in seconds
	ForgeDelay float64 `json:"forgeDelay"`
}

type APIState struct {
	// NodePublicConfig is the configuration of the node that is exposed via API
	NodePublicConfig  NodePublicConfig         `json:"nodeConfig"`
	Network           Network                  `json:"network"`
	Metrics           Metrics                  `json:"metrics"`
	Rollup            RollupVariablesAPI       `json:"rollup"`
	Auction           AuctionVariablesAPI      `json:"auction"`
	WithdrawalDelayer common.WDelayerVariables `json:"withdrawalDelayer"`
	RecommendedFee    common.RecommendedFee    `json:"recommendedFee"`
}

type Constants struct {
	RollupConstants   common.RollupConstants
	AuctionConstants  common.AuctionConstants
	WDelayerConstants common.WDelayerConstants
	ChainID           uint16
	HermezAddress     ethCommon.Address
}

type NodeConfig struct {
	MaxPoolTxs uint32  `meddler:"max_pool_txs"`
	MinFeeUSD  float64 `meddler:"min_fee"`
}

type NodeInfo struct {
	ItemID     int         `meddler:"item_id,pk"`
	APIState   *APIState   `meddler:"state,json"`
	NodeConfig *NodeConfig `meddler:"config,json"`
	Constants  *Constants  `meddler:"constants,json"`
}

func (hdb *HistoryDB) GetNodeInfo() (*NodeInfo, error) {
	ni := &NodeInfo{}
	err := meddler.QueryRow(
		hdb.dbRead, ni, `SELECT * FROM node_info WHERE item_id = 1;`,
	)
	return ni, tracerr.Wrap(err)
}

func (hdb *HistoryDB) GetConstants() (*Constants, error) {
	var nodeInfo NodeInfo
	err := meddler.QueryRow(
		hdb.dbRead, &nodeInfo,
		"SELECT constants FROM node_info WHERE item_id = 1;",
	)
	return nodeInfo.Constants, tracerr.Wrap(err)
}

func (hdb *HistoryDB) SetConstants(constants *Constants) error {
	_constants := struct {
		Constants *Constants `meddler:"constants,json"`
	}{constants}
	values, err := meddler.Default.Values(&_constants, false)
	if err != nil {
		return tracerr.Wrap(err)
	}
	_, err = hdb.dbWrite.Exec(
		"UPDATE node_info SET constants = $1 WHERE item_id = 1;",
		values[0],
	)
	return tracerr.Wrap(err)
}

func (hdb *HistoryDB) GetAPIState() (*APIState, error) {
	var nodeInfo NodeInfo
	err := meddler.QueryRow(
		hdb.dbRead, &nodeInfo,
		"SELECT state FROM node_info WHERE item_id = 1;",
	)
	return nodeInfo.APIState, tracerr.Wrap(err)
}

func (hdb *HistoryDB) SetAPIState(apiState *APIState) error {
	_apiState := struct {
		APIState *APIState `meddler:"state,json"`
	}{apiState}
	values, err := meddler.Default.Values(&_apiState, false)
	if err != nil {
		return tracerr.Wrap(err)
	}
	_, err = hdb.dbWrite.Exec(
		"UPDATE node_info SET state = $1 WHERE item_id = 1;",
		values[0],
	)
	return tracerr.Wrap(err)
}

func (hdb *HistoryDB) GetNodeConfig() (*NodeConfig, error) {
	var nodeInfo NodeInfo
	err := meddler.QueryRow(
		hdb.dbRead, &nodeInfo,
		"SELECT config FROM node_info WHERE item_id = 1;",
	)
	return nodeInfo.NodeConfig, tracerr.Wrap(err)
}

func (hdb *HistoryDB) SetNodeConfig(nodeConfig *NodeConfig) error {
	_nodeConfig := struct {
		NodeConfig *NodeConfig `meddler:"config,json"`
	}{nodeConfig}
	values, err := meddler.Default.Values(&_nodeConfig, false)
	if err != nil {
		return tracerr.Wrap(err)
	}
	_, err = hdb.dbWrite.Exec(
		"UPDATE config SET state = $1 WHERE item_id = 1;",
		values[0],
	)
	return tracerr.Wrap(err)
}

// func (hdb *HistoryDB) SetInitialNodeInfo(maxPoolTxs uint32, minFeeUSD float64, constants *Constants) error {
// 	ni := &NodeInfo{
// 		MaxPoolTxs: &maxPoolTxs,
// 		MinFeeUSD:  &minFeeUSD,
// 		Constants:  constants,
// 	}
// 	return tracerr.Wrap(meddler.Insert(hdb.dbWrite, "node_info", ni))
// }

type APIStateUpdater struct {
	hdb       *HistoryDB
	state     APIState
	config    NodeConfig
	constants Constants
}

func (u *APIStateUpdater) SetSCVars(rollupVariables *common.RollupVariables,
	auctionVariables *common.AuctionVariables,
	wDelayerVariables *common.WDelayerVariables) {
	if rollupVariables != nil {
		rollupVars := NewRollupVariablesAPI(rollupVariables)
		u.state.Rollup = *rollupVars
	}
	if auctionVariables != nil {
		auctionVars := NewAuctionVariablesAPI(auctionVariables)
		u.state.Auction = *auctionVars
	}
	if wDelayerVariables != nil {
		u.state.WithdrawalDelayer = *wDelayerVariables
	}
}

func (u *APIStateUpdater) UpdateNetworkInfoBlock(lastEthBlock, lastSyncBlock common.Block) {
	u.state.Network.LastSyncBlock = lastSyncBlock.Num
	u.state.Network.LastEthBlock = lastEthBlock.Num
}

func (u *APIStateUpdater) UpdateNetworkInfo(
	txn *sqlx.Tx,
	lastEthBlock, lastSyncBlock common.Block,
	lastBatchNum common.BatchNum, currentSlot int64,
) error {
	// Get last batch in API format
	lastBatch, err := u.hdb.getBatchAPI(txn, lastBatchNum)
	if tracerr.Unwrap(err) == sql.ErrNoRows {
		lastBatch = nil
	} else if err != nil {
		return tracerr.Wrap(err)
	}
	// Get next forrgers
	lastClosedSlot := currentSlot + int64(u.state.Auction.ClosedAuctionSlots)
	nextForgers, err := u.getNextForgers(txn, lastSyncBlock, currentSlot, lastClosedSlot)
	if tracerr.Unwrap(err) == sql.ErrNoRows {
		nextForgers = nil
	} else if err != nil {
		return tracerr.Wrap(err)
	}

	bucketUpdates, err := u.hdb.getBucketUpdatesAPI(txn)
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

// TODO: Remove
// SetRollupVariables set Status.Rollup variables
func (hdb *HistoryDB) SetRollupVariables(rollupVariables *common.RollupVariables) error {
	setUpdatedNodeInfo := func(txn *sqlx.Tx, ni *NodeInfo) error {
		rollupVars := NewRollupVariablesAPI(rollupVariables)
		ni.APIState.Rollup = *rollupVars
		return nil
	}
	return hdb.updateNodeInfo(setUpdatedNodeInfo)
}

// TODO: Remove
// SetWDelayerVariables set Status.WithdrawalDelayer variables
func (hdb *HistoryDB) SetWDelayerVariables(wDelayerVariables *common.WDelayerVariables) error {
	setUpdatedNodeInfo := func(txn *sqlx.Tx, ni *NodeInfo) error {
		ni.APIState.WithdrawalDelayer = *wDelayerVariables
		return nil
	}
	return hdb.updateNodeInfo(setUpdatedNodeInfo)
}

// TODO: Remove
// SetAuctionVariables set Status.Auction variables
func (hdb *HistoryDB) SetAuctionVariables(auctionVariables *common.AuctionVariables) error {
	setUpdatedNodeInfo := func(txn *sqlx.Tx, ni *NodeInfo) error {
		auctionVars := NewAuctionVariablesAPI(auctionVariables)
		ni.APIState.Auction = *auctionVars
		return nil
	}
	return hdb.updateNodeInfo(setUpdatedNodeInfo)
}

// TODO: Remove
// UpdateNetworkInfoBlock update Status.Network block related information
func (hdb *HistoryDB) UpdateNetworkInfoBlock(
	lastEthBlock, lastSyncBlock common.Block,
) error {
	setUpdatedNodeInfo := func(txn *sqlx.Tx, ni *NodeInfo) error {
		ni.APIState.Network.LastSyncBlock = lastSyncBlock.Num
		ni.APIState.Network.LastEthBlock = lastEthBlock.Num
		return nil
	}
	return hdb.updateNodeInfo(setUpdatedNodeInfo)
}

// UpdateNetworkInfo update Status.Network information
func (hdb *HistoryDB) UpdateNetworkInfo(
	lastEthBlock, lastSyncBlock common.Block,
	lastBatchNum common.BatchNum, currentSlot int64,
) error {
	setUpdatedNodeInfo := func(txn *sqlx.Tx, ni *NodeInfo) error {
		// Get last batch in API format
		lastBatch, err := hdb.getBatchAPI(txn, lastBatchNum)
		if tracerr.Unwrap(err) == sql.ErrNoRows {
			lastBatch = nil
		} else if err != nil {
			return tracerr.Wrap(err)
		}
		// Get next forrgers
		lastClosedSlot := currentSlot + int64(ni.APIState.Auction.ClosedAuctionSlots)
		nextForgers, err := hdb.getNextForgers(txn, ni, lastSyncBlock, currentSlot, lastClosedSlot)
		if tracerr.Unwrap(err) == sql.ErrNoRows {
			nextForgers = nil
		} else if err != nil {
			return tracerr.Wrap(err)
		}

		// Get buckets withdrawals
		var bucketUpdatesPtrs []*BucketUpdateAPI
		var bucketUpdates []BucketUpdateAPI
		err = meddler.QueryAll(
			txn, &bucketUpdatesPtrs,
			`SELECT num_bucket, withdrawals FROM bucket_update 
			WHERE item_id in(SELECT max(item_id) FROM bucket_update 
			group by num_bucket) 
			ORDER BY num_bucket ASC;`,
		)
		if err == sql.ErrNoRows {
			bucketUpdates = nil
		} else if err != nil {
			return tracerr.Wrap(err)
		} else {
			bucketUpdates = db.SlicePtrsToSlice(bucketUpdatesPtrs).([]BucketUpdateAPI)
		}
		// Update NodeInfo struct
		for i, bucketParams := range ni.APIState.Rollup.Buckets {
			for _, bucketUpdate := range bucketUpdates {
				if bucketUpdate.NumBucket == i {
					bucketParams.Withdrawals = bucketUpdate.Withdrawals
					ni.APIState.Rollup.Buckets[i] = bucketParams
					break
				}
			}
		}
		ni.APIState.Network.LastSyncBlock = lastSyncBlock.Num
		ni.APIState.Network.LastEthBlock = lastEthBlock.Num
		ni.APIState.Network.LastBatch = lastBatch
		ni.APIState.Network.CurrentSlot = currentSlot
		ni.APIState.Network.NextForgers = nextForgers
		return nil
	}
	return hdb.updateNodeInfo(setUpdatedNodeInfo)
}

// apiSlotToBigInts converts from [6]*apitypes.BigIntStr to [6]*big.Int
func apiSlotToBigInts(defaultSlotSetBid [6]*apitypes.BigIntStr) ([6]*big.Int, error) {
	var slots [6]*big.Int

	for i, slot := range defaultSlotSetBid {
		bigInt, ok := new(big.Int).SetString(string(*slot), 10)
		if !ok {
			return slots, tracerr.Wrap(fmt.Errorf("can't convert %T into big.Int", slot))
		}
		slots[i] = bigInt
	}

	return slots, nil
}

// getNextForgers returns next forgers
func (u *APIStateUpdater) getNextForgers(txn *sqlx.Tx,
	lastBlock common.Block, currentSlot, lastClosedSlot int64) ([]NextForger, error) {
	secondsPerBlock := int64(15) //nolint:gomnd
	// currentSlot and lastClosedSlot included
	limit := uint(lastClosedSlot - currentSlot + 1)
	bids, _, err := u.hdb.getBestBidsAPI(txn, &currentSlot, &lastClosedSlot, nil, &limit, "ASC")
	if err != nil && tracerr.Unwrap(err) != sql.ErrNoRows {
		return nil, tracerr.Wrap(err)
	}
	nextForgers := []NextForger{}
	// Get min bid info
	var minBidInfo []MinBidInfo
	if currentSlot >= u.state.Auction.DefaultSlotSetBidSlotNum {
		// All min bids can be calculated with the last update of AuctionVariables
		bigIntSlots, err := apiSlotToBigInts(u.state.Auction.DefaultSlotSetBid)
		if err != nil {
			return nil, tracerr.Wrap(err)
		}

		minBidInfo = []MinBidInfo{{
			DefaultSlotSetBid:        bigIntSlots,
			DefaultSlotSetBidSlotNum: u.state.Auction.DefaultSlotSetBidSlotNum,
		}}
	} else {
		// Get all the relevant updates from the DB
		minBidInfo, err = u.hdb.getMinBidInfo(txn, currentSlot, lastClosedSlot)
		if err != nil {
			return nil, tracerr.Wrap(err)
		}
	}
	// Create nextForger for each slot
	for i := currentSlot; i <= lastClosedSlot; i++ {
		fromBlock := i*int64(u.constants.AuctionConstants.BlocksPerSlot) +
			u.constants.AuctionConstants.GenesisBlockNum
		toBlock := (i+1)*int64(u.constants.AuctionConstants.BlocksPerSlot) +
			u.constants.AuctionConstants.GenesisBlockNum - 1
		nextForger := NextForger{
			Period: Period{
				SlotNum:   i,
				FromBlock: fromBlock,
				ToBlock:   toBlock,
				FromTimestamp: lastBlock.Timestamp.Add(time.Second *
					time.Duration(secondsPerBlock*(fromBlock-lastBlock.Num))),
				ToTimestamp: lastBlock.Timestamp.Add(time.Second *
					time.Duration(secondsPerBlock*(toBlock-lastBlock.Num))),
			},
		}
		foundForger := false
		// If there is a bid for a slot, get forger (coordinator)
		for j := range bids {
			slotNum := bids[j].SlotNum
			if slotNum == i {
				// There's a bid for the slot
				// Check if the bid is greater than the minimum required
				for i := 0; i < len(minBidInfo); i++ {
					// Find the most recent update
					if slotNum >= minBidInfo[i].DefaultSlotSetBidSlotNum {
						// Get min bid
						minBidSelector := slotNum % int64(len(u.state.Auction.DefaultSlotSetBid))
						minBid := minBidInfo[i].DefaultSlotSetBid[minBidSelector]
						// Check if the bid has beaten the minimum
						bid, ok := new(big.Int).SetString(string(bids[j].BidValue), 10)
						if !ok {
							return nil, tracerr.New("Wrong bid value, error parsing it as big.Int")
						}
						if minBid.Cmp(bid) == 1 {
							// Min bid is greater than bid, the slot will be forged by boot coordinator
							break
						}
						foundForger = true
						break
					}
				}
				if !foundForger { // There is no bid or it's smaller than the minimum
					break
				}
				coordinator, err := u.hdb.GetCoordinatorAPI(bids[j].Bidder)
				if err != nil {
					return nil, tracerr.Wrap(err)
				}
				nextForger.Coordinator = *coordinator
				break
			}
		}
		// If there is no bid, the coordinator that will forge is boot coordinator
		if !foundForger {
			nextForger.Coordinator = CoordinatorAPI{
				Forger: u.state.Auction.BootCoordinator,
				URL:    u.state.Auction.BootCoordinatorURL,
			}
		}
		nextForgers = append(nextForgers, nextForger)
	}
	return nextForgers, nil
}

// TODO: Rename to getMetrics and don't write anything
// UpdateMetrics update Status.Metrics information
func (hdb *HistoryDB) UpdateMetrics() error {
	setUpdatedNodeInfo := func(txn *sqlx.Tx, ni *NodeInfo) error {
		// Get the first and last batch of the last 24h and their timestamps
		if ni.APIState.Network.LastBatch == nil {
			return nil
		}
		type period struct {
			FromBatchNum  common.BatchNum `meddler:"from_batch_num"`
			FromTimestamp time.Time       `meddler:"from_timestamp"`
			ToBatchNum    common.BatchNum `meddler:"-"`
			ToTimestamp   time.Time       `meddler:"to_timestamp"`
		}
		p := &period{
			ToBatchNum: ni.APIState.Network.LastBatch.BatchNum,
		}
		if err := meddler.QueryRow(
			txn, p, `SELECT
			COALESCE (MIN(batch.batch_num), 0) as from_batch_num,
			COALESCE (MIN(block.timestamp), NOW()) AS from_timestamp, 
			COALESCE (MAX(block.timestamp), NOW()) AS to_timestamp
			FROM batch INNER JOIN block ON batch.eth_block_num = block.eth_block_num
			WHERE block.timestamp >= NOW() - INTERVAL '24 HOURS';`,
		); err != nil {
			return tracerr.Wrap(err)
		}
		// Get the amount of txs of that period
		row := txn.QueryRow(
			`SELECT COUNT(*) as total_txs FROM tx WHERE tx.batch_num between $1 AND $2;`,
			p.FromBatchNum, p.ToBatchNum,
		)
		var nTxs int
		if err := row.Scan(&nTxs); err != nil {
			return tracerr.Wrap(err)
		}
		// Set txs/s
		seconds := p.ToTimestamp.Sub(p.FromTimestamp).Seconds()
		if seconds == 0 { // Avoid dividing by 0
			seconds++
		}
		ni.APIState.Metrics.TransactionsPerSecond = float64(nTxs) / seconds
		// Set txs/batch
		nBatches := p.ToBatchNum - p.FromBatchNum
		if nBatches == 0 { // Avoid dividing by 0
			nBatches++
		}
		if (p.ToBatchNum - p.FromBatchNum) > 0 {
			ni.APIState.Metrics.TransactionsPerBatch = float64(nTxs) /
				float64(nBatches)
		} else {
			ni.APIState.Metrics.TransactionsPerBatch = 0
		}
		// Get total fee of that period
		row = txn.QueryRow(
			`SELECT COALESCE (SUM(total_fees_usd), 0) FROM batch WHERE batch_num between $1 AND $2;`,
			p.FromBatchNum, p.ToBatchNum,
		)
		var totalFee float64
		if err := row.Scan(&totalFee); err != nil {
			return tracerr.Wrap(err)
		}
		// Set batch frequency
		ni.APIState.Metrics.BatchFrequency = seconds / float64(nBatches)
		if nTxs > 0 {
			ni.APIState.Metrics.AvgTransactionFee = totalFee / float64(nTxs)
		} else {
			ni.APIState.Metrics.AvgTransactionFee = 0
		}
		// Get and set amount of registered accounts
		type registeredAccounts struct {
			TotalIdx int64 `meddler:"total_idx"`
			TotalBJJ int64 `meddler:"total_bjj"`
		}
		ra := &registeredAccounts{}
		if err := meddler.QueryRow(
			txn, ra,
			`SELECT COUNT(*) AS total_bjj, COUNT(DISTINCT(bjj)) AS total_idx FROM account;`,
		); err != nil {
			return tracerr.Wrap(err)
		}
		ni.APIState.Metrics.TotalAccounts = ra.TotalIdx
		ni.APIState.Metrics.TotalBJJs = ra.TotalBJJ
		// Get and set estimated time to forge L1 tx
		row = txn.QueryRow(
			`SELECT COALESCE (AVG(EXTRACT(EPOCH FROM (forged.timestamp - added.timestamp))), 0) FROM tx
			INNER JOIN block AS added ON tx.eth_block_num = added.eth_block_num
			INNER JOIN batch AS forged_batch ON tx.batch_num = forged_batch.batch_num
			INNER JOIN block AS forged ON forged_batch.eth_block_num = forged.eth_block_num
			WHERE tx.batch_num between $1 and $2 AND tx.is_l1 AND tx.user_origin;`,
			p.FromBatchNum, p.ToBatchNum,
		)
		var timeToForgeL1 float64
		if err := row.Scan(&timeToForgeL1); err != nil {
			return tracerr.Wrap(err)
		}
		ni.APIState.Metrics.EstimatedTimeToForgeL1 = timeToForgeL1
		return nil
	}
	return hdb.updateNodeInfo(setUpdatedNodeInfo)
}

// UpdateRecommendedFee update Status.RecommendedFee information
func (hdb *HistoryDB) UpdateRecommendedFee() error {
	setUpdatedNodeInfo := func(txn *sqlx.Tx, ni *NodeInfo) error {
		// Get total txs and the batch of the first selected tx of the last hour
		type totalTxsSinceBatchNum struct {
			TotalTxs      int             `meddler:"total_txs"`
			FirstBatchNum common.BatchNum `meddler:"batch_num"`
		}
		ttsbn := &totalTxsSinceBatchNum{}
		if err := meddler.QueryRow(
			txn, ttsbn, `SELECT COUNT(tx.*) as total_txs, 
			COALESCE (MIN(tx.batch_num), 0) as batch_num 
			FROM tx INNER JOIN block ON tx.eth_block_num = block.eth_block_num
			WHERE block.timestamp >= NOW() - INTERVAL '1 HOURS';`,
		); err != nil {
			return tracerr.Wrap(err)
		}
		// Get the amount of batches and acumulated fees for the last hour
		type totalBatchesAndFee struct {
			TotalBatches int     `meddler:"total_batches"`
			TotalFees    float64 `meddler:"total_fees"`
		}
		tbf := &totalBatchesAndFee{}
		if err := meddler.QueryRow(
			txn, tbf, `SELECT COUNT(*) AS total_batches, 
			COALESCE (SUM(total_fees_usd), 0) AS total_fees FROM batch 
			WHERE batch_num > $1;`, ttsbn.FirstBatchNum,
		); err != nil {
			return tracerr.Wrap(err)
		}
		// Update NodeInfo struct
		var avgTransactionFee float64
		if ttsbn.TotalTxs > 0 {
			avgTransactionFee = tbf.TotalFees / float64(ttsbn.TotalTxs)
		} else {
			avgTransactionFee = 0
		}
		ni.APIState.RecommendedFee.ExistingAccount =
			math.Max(avgTransactionFee, *ni.MinFeeUSD)
		ni.APIState.RecommendedFee.CreatesAccount =
			math.Max(createAccountExtraFeePercentage*avgTransactionFee, *ni.MinFeeUSD)
		ni.APIState.RecommendedFee.CreatesAccountAndRegister =
			math.Max(createAccountInternalExtraFeePercentage*avgTransactionFee, *ni.MinFeeUSD)
		return nil
	}
	return hdb.updateNodeInfo(setUpdatedNodeInfo)
}

func (hdb *HistoryDB) updateNodeInfo(setUpdatedNodeInfo func(*sqlx.Tx, *NodeInfo) error) error {
	// Create a SQL transaction or read and update atomicaly
	txn, err := hdb.dbWrite.Beginx()
	if err != nil {
		return tracerr.Wrap(err)
	}
	defer func() {
		if err != nil {
			db.Rollback(txn)
		}
	}()
	// Read current node info
	ni := &NodeInfo{}
	if err := meddler.QueryRow(
		txn, ni, "SELECT * FROM node_info;",
	); err != nil {
		return tracerr.Wrap(err)
	}
	// Update NodeInfo struct
	if err := setUpdatedNodeInfo(txn, ni); err != nil {
		return tracerr.Wrap(err)
	}
	// Update NodeInfo at DB
	if _, err := txn.Exec("DELETE FROM node_info;"); err != nil {
		return tracerr.Wrap(err)
	}
	if err := meddler.Insert(txn, "node_info", ni); err != nil {
		return tracerr.Wrap(err)
	}
	// Commit NodeInfo update
	return tracerr.Wrap(txn.Commit())
}
