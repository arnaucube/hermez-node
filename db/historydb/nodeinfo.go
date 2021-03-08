package historydb

import (
	"time"

	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/hermeznetwork/hermez-node/common"
	"github.com/hermeznetwork/tracerr"
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

type NextForgerAPI struct {
	Coordinator CoordinatorAPI `json:"coordinator"`
	Period      Period         `json:"period"`
}

type NetworkAPI struct {
	LastEthBlock  int64           `json:"lastEthereumBlock"`
	LastSyncBlock int64           `json:"lastSynchedBlock"`
	LastBatch     *BatchAPI       `json:"lastBatch"`
	CurrentSlot   int64           `json:"currentSlot"`
	NextForgers   []NextForgerAPI `json:"nextForgers"`
}

// NodePublicConfig is the configuration of the node that is exposed via API
type NodePublicConfig struct {
	// ForgeDelay in seconds
	ForgeDelay float64 `json:"forgeDelay"`
}

type StateAPI struct {
	// NodePublicConfig is the configuration of the node that is exposed via API
	NodePublicConfig  NodePublicConfig         `json:"nodeConfig"`
	Network           NetworkAPI               `json:"network"`
	Metrics           MetricsAPI               `json:"metrics"`
	Rollup            RollupVariablesAPI       `json:"rollup"`
	Auction           AuctionVariablesAPI      `json:"auction"`
	WithdrawalDelayer common.WDelayerVariables `json:"withdrawalDelayer"`
	RecommendedFee    common.RecommendedFee    `json:"recommendedFee"`
}

type Constants struct {
	// RollupConstants   common.RollupConstants
	// AuctionConstants  common.AuctionConstants
	// WDelayerConstants common.WDelayerConstants
	common.SCConsts
	ChainID       uint16
	HermezAddress ethCommon.Address
}

type NodeConfig struct {
	MaxPoolTxs uint32  `meddler:"max_pool_txs"`
	MinFeeUSD  float64 `meddler:"min_fee"`
}

type NodeInfo struct {
	ItemID     int         `meddler:"item_id,pk"`
	APIState   *StateAPI   `meddler:"state,json"`
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

func (hdb *HistoryDB) GetStateInternalAPI() (*StateAPI, error) {
	return hdb.getStateAPI(hdb.dbRead)
}

func (hdb *HistoryDB) getStateAPI(d meddler.DB) (*StateAPI, error) {
	var nodeInfo NodeInfo
	err := meddler.QueryRow(
		d, &nodeInfo,
		"SELECT state FROM node_info WHERE item_id = 1;",
	)
	return nodeInfo.APIState, tracerr.Wrap(err)
}

func (hdb *HistoryDB) SetAPIState(apiState *StateAPI) error {
	_apiState := struct {
		APIState *StateAPI `meddler:"state,json"`
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

// apiSlotToBigInts converts from [6]*apitypes.BigIntStr to [6]*big.Int
// func apiSlotToBigInts(defaultSlotSetBid [6]*apitypes.BigIntStr) ([6]*big.Int, error) {
// 	var slots [6]*big.Int
//
// 	for i, slot := range defaultSlotSetBid {
// 		bigInt, ok := new(big.Int).SetString(string(*slot), 10)
// 		if !ok {
// 			return slots, tracerr.Wrap(fmt.Errorf("can't convert %T into big.Int", slot))
// 		}
// 		slots[i] = bigInt
// 	}
//
// 	return slots, nil
// }

// func (hdb *HistoryDB) updateNodeInfo(setUpdatedNodeInfo func(*sqlx.Tx, *NodeInfo) error) error {
// 	// Create a SQL transaction or read and update atomicaly
// 	txn, err := hdb.dbWrite.Beginx()
// 	if err != nil {
// 		return tracerr.Wrap(err)
// 	}
// 	defer func() {
// 		if err != nil {
// 			db.Rollback(txn)
// 		}
// 	}()
// 	// Read current node info
// 	ni := &NodeInfo{}
// 	if err := meddler.QueryRow(
// 		txn, ni, "SELECT * FROM node_info;",
// 	); err != nil {
// 		return tracerr.Wrap(err)
// 	}
// 	// Update NodeInfo struct
// 	if err := setUpdatedNodeInfo(txn, ni); err != nil {
// 		return tracerr.Wrap(err)
// 	}
// 	// Update NodeInfo at DB
// 	if _, err := txn.Exec("DELETE FROM node_info;"); err != nil {
// 		return tracerr.Wrap(err)
// 	}
// 	if err := meddler.Insert(txn, "node_info", ni); err != nil {
// 		return tracerr.Wrap(err)
// 	}
// 	// Commit NodeInfo update
// 	return tracerr.Wrap(txn.Commit())
// }
