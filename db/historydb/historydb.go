package historydb

import (
	"database/sql"
	"errors"
	"fmt"

	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/gobuffalo/packr/v2"
	"github.com/hermeznetwork/hermez-node/common"
	"github.com/hermeznetwork/hermez-node/db"
	"github.com/hermeznetwork/hermez-node/log"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/jmoiron/sqlx"

	//nolint:errcheck // driver for postgres DB
	_ "github.com/lib/pq"
	migrate "github.com/rubenv/sql-migrate"
	"github.com/russross/meddler"
)

// TODO(Edu): Document here how HistoryDB is kept consistent

// HistoryDB persist the historic of the rollup
type HistoryDB struct {
	db *sqlx.DB
}

// BlockData contains the information of a Block
type BlockData struct {
	block *common.Block
	// Rollup
	// L1UserTxs that were submitted in the block
	L1UserTxs        []common.L1Tx
	Batches          []BatchData
	RegisteredTokens []common.Token
	RollupVars       *common.RollupVars
	// Auction
	Bids         []common.Bid
	Coordinators []common.Coordinator
	AuctionVars  *common.AuctionVars
	// WithdrawalDelayer
	// TODO: enable when common.WithdrawalDelayerVars is Merged from Synchronizer PR
	// WithdrawalDelayerVars *common.WithdrawalDelayerVars
}

// BatchData contains the information of a Batch
type BatchData struct {
	// L1UserTxs that were forged in the batch
	L1Batch          bool // TODO: Remove once Batch.ForgeL1TxsNum is a pointer
	L1UserTxs        []common.L1Tx
	L1CoordinatorTxs []common.L1Tx
	L2Txs            []common.L2Tx
	CreatedAccounts  []common.Account
	ExitTree         []common.ExitInfo
	Batch            *common.Batch
}

// NewHistoryDB initialize the DB
func NewHistoryDB(port int, host, user, password, dbname string) (*HistoryDB, error) {
	// Connect to DB
	psqlconn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", host, port, user, password, dbname)
	hdb, err := sqlx.Connect("postgres", psqlconn)
	if err != nil {
		return nil, err
	}
	// Init meddler
	db.InitMeddler()
	meddler.Default = meddler.PostgreSQL

	// Run DB migrations
	migrations := &migrate.PackrMigrationSource{
		Box: packr.New("history-migrations", "./migrations"),
	}
	nMigrations, err := migrate.Exec(hdb.DB, "postgres", migrations, migrate.Up)
	if err != nil {
		return nil, err
	}
	log.Debug("HistoryDB applied ", nMigrations, " migrations for ", dbname, " database")

	return &HistoryDB{hdb}, nil
}

// AddBlock insert a block into the DB
func (hdb *HistoryDB) AddBlock(block *common.Block) error { return hdb.addBlock(hdb.db, block) }
func (hdb *HistoryDB) addBlock(d meddler.DB, block *common.Block) error {
	return meddler.Insert(d, "block", block)
}

// AddBlocks inserts blocks into the DB
func (hdb *HistoryDB) AddBlocks(blocks []common.Block) error {
	return hdb.addBlocks(hdb.db, blocks)
}

func (hdb *HistoryDB) addBlocks(d meddler.DB, blocks []common.Block) error {
	return db.BulkInsert(
		d,
		`INSERT INTO block (
			eth_block_num,
			timestamp,
			hash
		) VALUES %s;`,
		blocks[:],
	)
}

// GetBlock retrieve a block from the DB, given a block number
func (hdb *HistoryDB) GetBlock(blockNum int64) (*common.Block, error) {
	block := &common.Block{}
	err := meddler.QueryRow(
		hdb.db, block,
		"SELECT * FROM block WHERE eth_block_num = $1;", blockNum,
	)
	return block, err
}

// GetBlocks retrieve blocks from the DB, given a range of block numbers defined by from and to
func (hdb *HistoryDB) GetBlocks(from, to int64) ([]*common.Block, error) {
	var blocks []*common.Block
	err := meddler.QueryAll(
		hdb.db, &blocks,
		"SELECT * FROM block WHERE $1 <= eth_block_num AND eth_block_num < $2;",
		from, to,
	)
	return blocks, err
}

// GetLastBlock retrieve the block with the highest block number from the DB
func (hdb *HistoryDB) GetLastBlock() (*common.Block, error) {
	block := &common.Block{}
	err := meddler.QueryRow(
		hdb.db, block, "SELECT * FROM block ORDER BY eth_block_num DESC LIMIT 1;",
	)
	return block, err
}

// AddBatch insert a Batch into the DB
func (hdb *HistoryDB) AddBatch(batch *common.Batch) error { return hdb.addBatch(hdb.db, batch) }
func (hdb *HistoryDB) addBatch(d meddler.DB, batch *common.Batch) error {
	return meddler.Insert(d, "batch", batch)
}

// AddBatches insert Bids into the DB
func (hdb *HistoryDB) AddBatches(batches []common.Batch) error {
	return hdb.addBatches(hdb.db, batches)
}
func (hdb *HistoryDB) addBatches(d meddler.DB, batches []common.Batch) error {
	return db.BulkInsert(
		d,
		`INSERT INTO batch (
			batch_num,
			eth_block_num,
			forger_addr,
			fees_collected,
			state_root,
			num_accounts,
			exit_root,
			forge_l1_txs_num,
			slot_num
		) VALUES %s;`,
		batches[:],
	)
}

// GetBatches retrieve batches from the DB, given a range of batch numbers defined by from and to
func (hdb *HistoryDB) GetBatches(from, to common.BatchNum) ([]*common.Batch, error) {
	var batches []*common.Batch
	err := meddler.QueryAll(
		hdb.db, &batches,
		"SELECT * FROM batch WHERE $1 <= batch_num AND batch_num < $2;",
		from, to,
	)
	return batches, err
}

// GetLastBatchNum returns the BatchNum of the latest forged batch
func (hdb *HistoryDB) GetLastBatchNum() (common.BatchNum, error) {
	row := hdb.db.QueryRow("SELECT batch_num FROM batch ORDER BY batch_num DESC LIMIT 1;")
	var batchNum common.BatchNum
	return batchNum, row.Scan(&batchNum)
}

// GetLastL1TxsNum returns the greatest ForgeL1TxsNum in the DB.  If there's no
// batch in the DB (nil, nil) is returned.
func (hdb *HistoryDB) GetLastL1TxsNum() (*int64, error) {
	row := hdb.db.QueryRow("SELECT MAX(forge_l1_txs_num) FROM batch;")
	lastL1TxsNum := new(int64)
	return lastL1TxsNum, row.Scan(&lastL1TxsNum)
}

// Reorg deletes all the information that was added into the DB after the
// lastValidBlock.  If lastValidBlock is negative, all block information is
// deleted.
func (hdb *HistoryDB) Reorg(lastValidBlock int64) error {
	var err error
	if lastValidBlock < 0 {
		_, err = hdb.db.Exec("DELETE FROM block;")
	} else {
		_, err = hdb.db.Exec("DELETE FROM block WHERE eth_block_num > $1;", lastValidBlock)
	}
	return err
}

// SyncPoD stores all the data that can be changed / added on a block in the PoD SC
func (hdb *HistoryDB) SyncPoD(
	blockNum uint64,
	bids []common.Bid,
	coordinators []common.Coordinator,
	vars *common.AuctionVars,
) error {
	return nil
}

// AddBids insert Bids into the DB
func (hdb *HistoryDB) AddBids(bids []common.Bid) error { return hdb.addBids(hdb.db, bids) }
func (hdb *HistoryDB) addBids(d meddler.DB, bids []common.Bid) error {
	// TODO: check the coordinator info
	return db.BulkInsert(
		d,
		"INSERT INTO bid (slot_num, forger_addr, bid_value, eth_block_num) VALUES %s;",
		bids[:],
	)
}

// GetBids return the bids
func (hdb *HistoryDB) GetBids() ([]*common.Bid, error) {
	var bids []*common.Bid
	err := meddler.QueryAll(
		hdb.db, &bids,
		"SELECT * FROM bid;",
	)
	return bids, err
}

// AddCoordinators insert Coordinators into the DB
func (hdb *HistoryDB) AddCoordinators(coordinators []common.Coordinator) error {
	return hdb.addCoordinators(hdb.db, coordinators)
}
func (hdb *HistoryDB) addCoordinators(d meddler.DB, coordinators []common.Coordinator) error {
	return db.BulkInsert(
		d,
		"INSERT INTO coordinator (forger_addr, eth_block_num, withdraw_addr, url) VALUES %s;",
		coordinators[:],
	)
}

// AddExitTree insert Exit tree into the DB
func (hdb *HistoryDB) AddExitTree(exitTree []common.ExitInfo) error {
	return hdb.addExitTree(hdb.db, exitTree)
}
func (hdb *HistoryDB) addExitTree(d meddler.DB, exitTree []common.ExitInfo) error {
	return db.BulkInsert(
		d,
		"INSERT INTO exit_tree (batch_num, account_idx, merkle_proof, balance, "+
			"instant_withdrawn, delayed_withdraw_request, delayed_withdrawn) VALUES %s;",
		exitTree[:],
	)
}

// AddToken insert a token into the DB
func (hdb *HistoryDB) AddToken(token *common.Token) error {
	return meddler.Insert(hdb.db, "token", token)
}

// AddTokens insert tokens into the DB
func (hdb *HistoryDB) AddTokens(tokens []common.Token) error { return hdb.addTokens(hdb.db, tokens) }
func (hdb *HistoryDB) addTokens(d meddler.DB, tokens []common.Token) error {
	return db.BulkInsert(
		d,
		`INSERT INTO token (
			token_id,
			eth_block_num,
			eth_addr,
			name,
			symbol,
			decimals,
			usd,
			usd_update
		) VALUES %s;`,
		tokens[:],
	)
}

// UpdateTokenValue updates the USD value of a token
func (hdb *HistoryDB) UpdateTokenValue(tokenID common.TokenID, value float64) error {
	_, err := hdb.db.Exec(
		"UPDATE token SET usd = $1 WHERE token_id = $2;",
		value, tokenID,
	)
	return err
}

// GetTokens returns a list of tokens from the DB
func (hdb *HistoryDB) GetTokens() ([]*common.Token, error) {
	var tokens []*common.Token
	err := meddler.QueryAll(
		hdb.db, &tokens,
		"SELECT * FROM token ORDER BY token_id;",
	)
	return tokens, err
}

// AddAccounts insert accounts into the DB
func (hdb *HistoryDB) AddAccounts(accounts []common.Account) error {
	return hdb.addAccounts(hdb.db, accounts)
}
func (hdb *HistoryDB) addAccounts(d meddler.DB, accounts []common.Account) error {
	return db.BulkInsert(
		d,
		`INSERT INTO account (
			idx,
			token_id,
			batch_num,
			bjj,
			eth_addr
		) VALUES %s;`,
		accounts[:],
	)
}

// GetAccounts returns a list of accounts from the DB
func (hdb *HistoryDB) GetAccounts() ([]*common.Account, error) {
	var accs []*common.Account
	err := meddler.QueryAll(
		hdb.db, &accs,
		"SELECT * FROM account ORDER BY idx;",
	)
	return accs, err
}

// AddL1Txs inserts L1 txs to the DB
func (hdb *HistoryDB) AddL1Txs(l1txs []common.L1Tx) error { return hdb.addL1Txs(hdb.db, l1txs) }
func (hdb *HistoryDB) addL1Txs(d meddler.DB, l1txs []common.L1Tx) error {
	txs := []common.Tx{}
	for _, tx := range l1txs {
		txs = append(txs, *(tx.Tx()))
	}
	return hdb.addTxs(d, txs)
}

// AddL2Txs inserts L2 txs to the DB
func (hdb *HistoryDB) AddL2Txs(l2txs []common.L2Tx) error { return hdb.addL2Txs(hdb.db, l2txs) }
func (hdb *HistoryDB) addL2Txs(d meddler.DB, l2txs []common.L2Tx) error {
	txs := []common.Tx{}
	for _, tx := range l2txs {
		txs = append(txs, *(tx.Tx()))
	}
	return hdb.addTxs(d, txs)
}

// AddTxs insert L1 txs into the DB
func (hdb *HistoryDB) AddTxs(txs []common.Tx) error { return hdb.addTxs(hdb.db, txs) }
func (hdb *HistoryDB) addTxs(d meddler.DB, txs []common.Tx) error {
	return db.BulkInsert(
		d,
		`INSERT INTO tx (
			is_l1,
			id,
			type,
			position,
			from_idx,
			to_idx,
			amount,
			amount_f,
			token_id,
			amount_usd,
			batch_num,
			eth_block_num,
			to_forge_l1_txs_num,
			user_origin,
			from_eth_addr,
			from_bjj,
			load_amount,
			load_amount_f,
			load_amount_usd,
			fee,
			fee_usd,
			nonce
		) VALUES %s;`,
		txs[:],
	)
}

// SetBatchNumL1UserTxs sets the batchNum in all the L1UserTxs with toForgeL1TxsNum.
func (hdb *HistoryDB) SetBatchNumL1UserTxs(toForgeL1TxsNum, batchNum int64) error {
	return hdb.setBatchNumL1UserTxs(hdb.db, toForgeL1TxsNum, batchNum)
}
func (hdb *HistoryDB) setBatchNumL1UserTxs(d meddler.DB, toForgeL1TxsNum, batchNum int64) error {
	_, err := d.Exec("UPDATE tx SET batch_num = $1 WHERE to_forge_l1_txs_num = $2 AND is_l1 = TRUE AND user_origin = TRUE;",
		batchNum, toForgeL1TxsNum)
	return err
}

// GetTxs returns a list of txs from the DB
func (hdb *HistoryDB) GetTxs() ([]*common.Tx, error) {
	var txs []*common.Tx
	err := meddler.QueryAll(
		hdb.db, &txs,
		`SELECT * FROM tx 
		ORDER BY (batch_num, position) ASC`,
	)
	return txs, err
}

// GetHistoryTxs returns a list of txs from the DB using the HistoryTx struct
func (hdb *HistoryDB) GetHistoryTxs(
	ethAddr *ethCommon.Address, bjj *babyjub.PublicKey,
	tokenID, idx, batchNum *uint, txType *common.TxType,
	offset, limit *uint, last bool,
) ([]*HistoryTx, int, error) {
	if ethAddr != nil && bjj != nil {
		return nil, 0, errors.New("ethAddr and bjj are incompatible")
	}
	var query string
	var args []interface{}
	queryStr := `SELECT tx.*, tx.amount_f * token.usd AS current_usd,
	token.symbol, token.usd_update, block.timestamp, count(*) OVER() AS total_items FROM tx 
	INNER JOIN token ON tx.token_id = token.token_id 
	INNER JOIN block ON tx.eth_block_num = block.eth_block_num `
	// Apply filters
	nextIsAnd := false
	// ethAddr filter
	if ethAddr != nil {
		queryStr = `WITH acc AS 
		(select idx from account where eth_addr = ?) ` + queryStr
		queryStr += ", acc WHERE (tx.from_idx IN(acc.idx) OR tx.to_idx IN(acc.idx)) "
		nextIsAnd = true
		args = append(args, ethAddr)
	} else if bjj != nil { // bjj filter
		queryStr = `WITH acc AS 
		(select idx from account where bjj = ?) ` + queryStr
		queryStr += ", acc WHERE (tx.from_idx IN(acc.idx) OR tx.to_idx IN(acc.idx)) "
		nextIsAnd = true
		args = append(args, bjj)
	}
	// tokenID filter
	if tokenID != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "tx.token_id = ? "
		args = append(args, tokenID)
		nextIsAnd = true
	}
	// idx filter
	if idx != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "(tx.from_idx = ? OR tx.to_idx = ?) "
		args = append(args, idx, idx)
		nextIsAnd = true
	}
	// batchNum filter
	if batchNum != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "tx.batch_num = ? "
		args = append(args, batchNum)
		nextIsAnd = true
	}
	// txType filter
	if txType != nil {
		if nextIsAnd {
			queryStr += "AND "
		} else {
			queryStr += "WHERE "
		}
		queryStr += "tx.type = ? "
		args = append(args, txType)
		// nextIsAnd = true
	}
	// pagination
	if last {
		queryStr += "ORDER BY (batch_num, position) DESC NULLS FIRST "
	} else {
		queryStr += "ORDER BY (batch_num, position) ASC NULLS LAST "
		queryStr += fmt.Sprintf("OFFSET %d ", *offset)
	}
	queryStr += fmt.Sprintf("LIMIT %d;", *limit)
	query = hdb.db.Rebind(queryStr)
	// log.Debug(query)
	txs := []*HistoryTx{}
	if err := meddler.QueryAll(hdb.db, &txs, query, args...); err != nil {
		return nil, 0, err
	}
	if len(txs) == 0 {
		return nil, 0, sql.ErrNoRows
	} else if last {
		tmp := []*HistoryTx{}
		for i := len(txs) - 1; i >= 0; i-- {
			tmp = append(tmp, txs[i])
		}
		txs = tmp
	}
	return txs, txs[0].TotalItems, nil
}

// GetTx returns a tx from the DB
func (hdb *HistoryDB) GetTx(txID common.TxID) (*common.Tx, error) {
	tx := new(common.Tx)
	return tx, meddler.QueryRow(
		hdb.db, tx,
		"SELECT * FROM tx WHERE id = $1;",
		txID,
	)
}

// GetL1UserTxs gets L1 User Txs to be forged in a batch that will create an account
// TODO: This is currently not used.  Figure out if it should be used somewhere or removed.
func (hdb *HistoryDB) GetL1UserTxs(toForgeL1TxsNum int64) ([]*common.Tx, error) {
	var txs []*common.Tx
	err := meddler.QueryAll(
		hdb.db, &txs,
		"SELECT * FROM tx WHERE to_forge_l1_txs_num = $1 AND is_l1 = TRUE AND user_origin = TRUE;",
		toForgeL1TxsNum,
	)
	return txs, err
}

// TODO: Think about chaning all the queries that return a last value, to queries that return the next valid value.

// GetLastTxsPosition for a given to_forge_l1_txs_num
func (hdb *HistoryDB) GetLastTxsPosition(toForgeL1TxsNum int64) (int, error) {
	row := hdb.db.QueryRow("SELECT MAX(position) FROM tx WHERE to_forge_l1_txs_num = $1;", toForgeL1TxsNum)
	var lastL1TxsPosition int
	return lastL1TxsPosition, row.Scan(&lastL1TxsPosition)
}

// AddBlockSCData stores all the information of a block retrieved by the Synchronizer
func (hdb *HistoryDB) AddBlockSCData(blockData *BlockData) (err error) {
	txn, err := hdb.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = txn.Rollback()
		}
	}()

	// Add block
	err = hdb.addBlock(txn, blockData.block)
	if err != nil {
		return err
	}

	// Add l1 Txs
	if len(blockData.L1UserTxs) > 0 {
		err = hdb.addL1Txs(txn, blockData.L1UserTxs)
		if err != nil {
			return err
		}
	}

	// Add Tokens
	if len(blockData.RegisteredTokens) > 0 {
		err = hdb.addTokens(txn, blockData.RegisteredTokens)
		if err != nil {
			return err
		}
	}

	// Add Bids
	if len(blockData.Bids) > 0 {
		err = hdb.addBids(txn, blockData.Bids)
		if err != nil {
			return err
		}
	}

	// Add Coordinators
	if len(blockData.Coordinators) > 0 {
		err = hdb.addCoordinators(txn, blockData.Coordinators)
		if err != nil {
			return err
		}
	}

	// Add Batches
	for _, batch := range blockData.Batches {
		if batch.L1Batch {
			err = hdb.setBatchNumL1UserTxs(txn, batch.Batch.ForgeL1TxsNum, int64(batch.Batch.BatchNum))
			if err != nil {
				return err
			}
			if len(batch.L1CoordinatorTxs) > 0 {
				err = hdb.addL1Txs(txn, batch.L1CoordinatorTxs)
				if err != nil {
					return err
				}
			}
		}

		// Add l2 Txs
		if len(batch.L2Txs) > 0 {
			err = hdb.addL2Txs(txn, batch.L2Txs)
			if err != nil {
				return err
			}
		}

		// Add accounts
		if len(batch.CreatedAccounts) > 0 {
			err = hdb.addAccounts(txn, batch.CreatedAccounts)
			if err != nil {
				return err
			}
		}

		// Add exit tree
		if len(batch.ExitTree) > 0 {
			err = hdb.addExitTree(txn, batch.ExitTree)
			if err != nil {
				return err
			}
		}

		// Add Batch
		err = hdb.addBatch(txn, batch.Batch)
		if err != nil {
			return err
		}

		// TODO: INSERT CONTRACTS VARS
	}

	return txn.Commit()
}

// Close frees the resources used by HistoryDB
func (hdb *HistoryDB) Close() error {
	return hdb.db.Close()
}
