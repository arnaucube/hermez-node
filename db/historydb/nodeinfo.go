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

// NodePublicConfig is the configuration of the node that is exposed via API
type NodePublicConfig struct {
	// ForgeDelay in seconds
	ForgeDelay float64 `json:"forgeDelay"`
}
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
type StateAPI struct {
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
type NodeInfo struct {
	MaxPoolTxs uint32    `meddler:"max_pool_txs"`
	MinFeeUSD  float64   `meddler:"min_fee"`
	StateAPI   StateAPI  `meddler:"state,json"`
	Constants  Constants `meddler:"constants,json"`
}

func (hdb *HistoryDB) GetNodeInfo() (*NodeInfo, error) {
	ni := &NodeInfo{}
	err := meddler.QueryRow(
		hdb.dbRead, ni, `SELECT * FROM node_info ORDER BY item_id DESC LIMIT 1;`,
	)
	return ni, tracerr.Wrap(err)
}

func (hdb *HistoryDB) SetInitialNodeInfo(maxPoolTxs uint32, minFeeUSD float64, constants *Constants) error {
	ni := &NodeInfo{
		MaxPoolTxs: maxPoolTxs,
		MinFeeUSD:  minFeeUSD,
		Constants:  *constants,
	}
	return tracerr.Wrap(meddler.Insert(hdb.dbWrite, "node_info", ni))
}

// SetRollupVariables set Status.Rollup variables
func (hdb *HistoryDB) SetRollupVariables(rollupVariables common.RollupVariables) error {
	setUpdatedNodeInfo := func(txn *sqlx.Tx, ni *NodeInfo) error {
		var rollupVars RollupVariablesAPI
		rollupVars.EthBlockNum = rollupVariables.EthBlockNum
		rollupVars.FeeAddToken = apitypes.NewBigIntStr(rollupVariables.FeeAddToken)
		rollupVars.ForgeL1L2BatchTimeout = rollupVariables.ForgeL1L2BatchTimeout
		rollupVars.WithdrawalDelay = rollupVariables.WithdrawalDelay

		for i, bucket := range rollupVariables.Buckets {
			var apiBucket BucketParamsAPI
			apiBucket.CeilUSD = apitypes.NewBigIntStr(bucket.CeilUSD)
			apiBucket.Withdrawals = apitypes.NewBigIntStr(bucket.Withdrawals)
			apiBucket.BlockWithdrawalRate = apitypes.NewBigIntStr(bucket.BlockWithdrawalRate)
			apiBucket.MaxWithdrawals = apitypes.NewBigIntStr(bucket.MaxWithdrawals)
			rollupVars.Buckets[i] = apiBucket
		}

		rollupVars.SafeMode = rollupVariables.SafeMode
		ni.StateAPI.Rollup = rollupVars
		return nil
	}
	return hdb.updateNodeInfo(setUpdatedNodeInfo)
}

// SetWDelayerVariables set Status.WithdrawalDelayer variables
func (hdb *HistoryDB) SetWDelayerVariables(wDelayerVariables common.WDelayerVariables) error {
	setUpdatedNodeInfo := func(txn *sqlx.Tx, ni *NodeInfo) error {
		ni.StateAPI.WithdrawalDelayer = wDelayerVariables
		return nil
	}
	return hdb.updateNodeInfo(setUpdatedNodeInfo)
}

// SetAuctionVariables set Status.Auction variables
func (hdb *HistoryDB) SetAuctionVariables(auctionVariables common.AuctionVariables) error {
	setUpdatedNodeInfo := func(txn *sqlx.Tx, ni *NodeInfo) error {
		var auctionVars AuctionVariablesAPI

		auctionVars.EthBlockNum = auctionVariables.EthBlockNum
		auctionVars.DonationAddress = auctionVariables.DonationAddress
		auctionVars.BootCoordinator = auctionVariables.BootCoordinator
		auctionVars.BootCoordinatorURL = auctionVariables.BootCoordinatorURL
		auctionVars.DefaultSlotSetBidSlotNum = auctionVariables.DefaultSlotSetBidSlotNum
		auctionVars.ClosedAuctionSlots = auctionVariables.ClosedAuctionSlots
		auctionVars.OpenAuctionSlots = auctionVariables.OpenAuctionSlots
		auctionVars.Outbidding = auctionVariables.Outbidding
		auctionVars.SlotDeadline = auctionVariables.SlotDeadline

		for i, slot := range auctionVariables.DefaultSlotSetBid {
			auctionVars.DefaultSlotSetBid[i] = apitypes.NewBigIntStr(slot)
		}

		for i, ratio := range auctionVariables.AllocationRatio {
			auctionVars.AllocationRatio[i] = ratio
		}

		ni.StateAPI.Auction = auctionVars
		return nil
	}
	return hdb.updateNodeInfo(setUpdatedNodeInfo)
}

// UpdateNetworkInfoBlock update Status.Network block related information
func (hdb *HistoryDB) UpdateNetworkInfoBlock(
	lastEthBlock, lastSyncBlock common.Block,
) error {
	setUpdatedNodeInfo := func(txn *sqlx.Tx, ni *NodeInfo) error {
		ni.StateAPI.Network.LastSyncBlock = lastSyncBlock.Num
		ni.StateAPI.Network.LastEthBlock = lastEthBlock.Num
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
		lastClosedSlot := currentSlot + int64(ni.StateAPI.Auction.ClosedAuctionSlots)
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
		for i, bucketParams := range ni.StateAPI.Rollup.Buckets {
			for _, bucketUpdate := range bucketUpdates {
				if bucketUpdate.NumBucket == i {
					bucketParams.Withdrawals = bucketUpdate.Withdrawals
					ni.StateAPI.Rollup.Buckets[i] = bucketParams
					break
				}
			}
		}
		ni.StateAPI.Network.LastSyncBlock = lastSyncBlock.Num
		ni.StateAPI.Network.LastEthBlock = lastEthBlock.Num
		ni.StateAPI.Network.LastBatch = lastBatch
		ni.StateAPI.Network.CurrentSlot = currentSlot
		ni.StateAPI.Network.NextForgers = nextForgers
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
func (hdb *HistoryDB) getNextForgers(txn *sqlx.Tx, ni *NodeInfo, lastBlock common.Block, currentSlot, lastClosedSlot int64) ([]NextForger, error) {
	secondsPerBlock := int64(15) //nolint:gomnd
	// currentSlot and lastClosedSlot included
	limit := uint(lastClosedSlot - currentSlot + 1)
	bids, _, err := hdb.getBestBidsAPI(txn, &currentSlot, &lastClosedSlot, nil, &limit, "ASC")
	if err != nil && tracerr.Unwrap(err) != sql.ErrNoRows {
		return nil, tracerr.Wrap(err)
	}
	nextForgers := []NextForger{}
	// Get min bid info
	var minBidInfo []MinBidInfo
	if currentSlot >= ni.StateAPI.Auction.DefaultSlotSetBidSlotNum {
		// All min bids can be calculated with the last update of AuctionVariables
		bigIntSlots, err := apiSlotToBigInts(ni.StateAPI.Auction.DefaultSlotSetBid)
		if err != nil {
			return nil, tracerr.Wrap(err)
		}

		minBidInfo = []MinBidInfo{{
			DefaultSlotSetBid:        bigIntSlots,
			DefaultSlotSetBidSlotNum: ni.StateAPI.Auction.DefaultSlotSetBidSlotNum,
		}}
	} else {
		// Get all the relevant updates from the DB
		minBidInfoPtrs := []*MinBidInfo{}
		query := `
			SELECT DISTINCT default_slot_set_bid, default_slot_set_bid_slot_num FROM auction_vars
			WHERE default_slot_set_bid_slot_num < $1
			ORDER BY default_slot_set_bid_slot_num DESC
			LIMIT $2;`
		if err := meddler.QueryAll(
			txn, &minBidInfoPtrs, query, lastClosedSlot, int(lastClosedSlot-currentSlot)+1,
		); err != nil {
			return nil, tracerr.Wrap(err)
		}
		minBidInfo = db.SlicePtrsToSlice(minBidInfoPtrs).([]MinBidInfo)
	}
	// Create nextForger for each slot
	for i := currentSlot; i <= lastClosedSlot; i++ {
		fromBlock := i*int64(ni.Constants.AuctionConstants.BlocksPerSlot) + ni.Constants.AuctionConstants.GenesisBlockNum
		toBlock := (i+1)*int64(ni.Constants.AuctionConstants.BlocksPerSlot) + ni.Constants.AuctionConstants.GenesisBlockNum - 1
		nextForger := NextForger{
			Period: Period{
				SlotNum:       i,
				FromBlock:     fromBlock,
				ToBlock:       toBlock,
				FromTimestamp: lastBlock.Timestamp.Add(time.Second * time.Duration(secondsPerBlock*(fromBlock-lastBlock.Num))),
				ToTimestamp:   lastBlock.Timestamp.Add(time.Second * time.Duration(secondsPerBlock*(toBlock-lastBlock.Num))),
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
						minBidSelector := slotNum % int64(len(ni.StateAPI.Auction.DefaultSlotSetBid))
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
				coordinator, err := hdb.GetCoordinatorAPI(bids[j].Bidder)
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
				Forger: ni.StateAPI.Auction.BootCoordinator,
				URL:    ni.StateAPI.Auction.BootCoordinatorURL,
			}
		}
		nextForgers = append(nextForgers, nextForger)
	}
	return nextForgers, nil
}

// UpdateMetrics update Status.Metrics information
func (hdb *HistoryDB) UpdateMetrics() error {
	setUpdatedNodeInfo := func(txn *sqlx.Tx, ni *NodeInfo) error {
		// Get the first and last batch of the last 24h and their timestamps
		if ni.StateAPI.Network.LastBatch == nil {
			return nil
		}
		type period struct {
			FromBatchNum  common.BatchNum `meddler:"from_batch_num"`
			FromTimestamp time.Time       `meddler:"from_timestamp"`
			ToBatchNum    common.BatchNum `meddler:"-"`
			ToTimestamp   time.Time       `meddler:"to_timestamp"`
		}
		p := &period{
			ToBatchNum: ni.StateAPI.Network.LastBatch.BatchNum,
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
		ni.StateAPI.Metrics.TransactionsPerSecond = float64(nTxs) / seconds
		// Set txs/batch
		nBatches := p.ToBatchNum - p.FromBatchNum
		if nBatches == 0 { // Avoid dividing by 0
			nBatches++
		}
		if (p.ToBatchNum - p.FromBatchNum) > 0 {
			ni.StateAPI.Metrics.TransactionsPerBatch = float64(nTxs) /
				float64(nBatches)
		} else {
			ni.StateAPI.Metrics.TransactionsPerBatch = 0
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
		ni.StateAPI.Metrics.BatchFrequency = seconds / float64(nBatches)
		if nTxs > 0 {
			ni.StateAPI.Metrics.AvgTransactionFee = totalFee / float64(nTxs)
		} else {
			ni.StateAPI.Metrics.AvgTransactionFee = 0
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
		ni.StateAPI.Metrics.TotalAccounts = ra.TotalIdx
		ni.StateAPI.Metrics.TotalBJJs = ra.TotalBJJ
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
		ni.StateAPI.Metrics.EstimatedTimeToForgeL1 = timeToForgeL1
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
		ni.StateAPI.RecommendedFee.ExistingAccount =
			math.Max(avgTransactionFee, ni.MinFeeUSD)
		ni.StateAPI.RecommendedFee.CreatesAccount =
			math.Max(createAccountExtraFeePercentage*avgTransactionFee, ni.MinFeeUSD)
		ni.StateAPI.RecommendedFee.CreatesAccountAndRegister =
			math.Max(createAccountInternalExtraFeePercentage*avgTransactionFee, ni.MinFeeUSD)
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
