package synchronizer

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"testing"

	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/hermeznetwork/hermez-node/common"
	dbUtils "github.com/hermeznetwork/hermez-node/db"
	"github.com/hermeznetwork/hermez-node/db/historydb"
	"github.com/hermeznetwork/hermez-node/db/statedb"
	"github.com/hermeznetwork/hermez-node/eth"
	"github.com/hermeznetwork/hermez-node/test"
	"github.com/hermeznetwork/hermez-node/test/til"
	"github.com/jinzhu/copier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var tokenConsts = map[common.TokenID]eth.ERC20Consts{}
var forceExits = map[int64][]common.ExitInfo{} // ForgeL1TxsNum -> []exit
var nonces = map[common.Idx]common.Nonce{}

type timer struct {
	time int64
}

func (t *timer) Time() int64 {
	currentTime := t.time
	t.time++
	return currentTime
}

// Check Sync output and HistoryDB state against expected values generated by
// til
func checkSyncBlock(t *testing.T, s *Synchronizer, blockNum int, block, syncBlock *common.BlockData) {
	// Check Blocks
	dbBlocks, err := s.historyDB.GetAllBlocks()
	require.Nil(t, err)
	dbBlocks = dbBlocks[1:] // ignore block 0, added by default in the DB
	assert.Equal(t, blockNum, len(dbBlocks))
	assert.Equal(t, int64(blockNum), dbBlocks[blockNum-1].EthBlockNum)
	assert.NotEqual(t, dbBlocks[blockNum-1].Hash, dbBlocks[blockNum-2].Hash)
	assert.Greater(t, dbBlocks[blockNum-1].Timestamp.Unix(), dbBlocks[blockNum-2].Timestamp.Unix())

	// Check Tokens
	assert.Equal(t, len(block.Rollup.AddedTokens), len(syncBlock.Rollup.AddedTokens))
	dbTokens, err := s.historyDB.GetAllTokens()
	require.Nil(t, err)
	dbTokens = dbTokens[1:] // ignore token 0, added by default in the DB
	for i, token := range block.Rollup.AddedTokens {
		dbToken := dbTokens[i]
		syncToken := syncBlock.Rollup.AddedTokens[i]

		assert.Equal(t, block.Block.EthBlockNum, syncToken.EthBlockNum)
		assert.Equal(t, token.TokenID, syncToken.TokenID)
		assert.Equal(t, token.EthAddr, syncToken.EthAddr)
		tokenConst := tokenConsts[token.TokenID]
		assert.Equal(t, tokenConst.Name, syncToken.Name)
		assert.Equal(t, tokenConst.Symbol, syncToken.Symbol)
		assert.Equal(t, tokenConst.Decimals, syncToken.Decimals)

		var tokenCpy historydb.TokenWithUSD
		//nolint:gosec
		require.Nil(t, copier.Copy(&tokenCpy, &token))      // copy common.Token to historydb.TokenWithUSD
		require.Nil(t, copier.Copy(&tokenCpy, &tokenConst)) // copy common.Token to historydb.TokenWithUSD
		tokenCpy.ItemID = dbToken.ItemID                    // we don't care about ItemID
		assert.Equal(t, tokenCpy, dbToken)
	}

	// Check L1UserTxs
	assert.Equal(t, len(block.Rollup.L1UserTxs), len(syncBlock.Rollup.L1UserTxs))
	dbL1UserTxs, err := s.historyDB.GetAllL1UserTxs()
	require.Nil(t, err)
	// Ignore BatchNum in syncBlock.L1UserTxs because this value is set by the HistoryDB
	for i := range syncBlock.Rollup.L1UserTxs {
		syncBlock.Rollup.L1UserTxs[i].BatchNum = block.Rollup.L1UserTxs[i].BatchNum
	}
	assert.Equal(t, block.Rollup.L1UserTxs, syncBlock.Rollup.L1UserTxs)
	for _, tx := range block.Rollup.L1UserTxs {
		var dbTx *common.L1Tx
		// Find tx in DB output
		for _, _dbTx := range dbL1UserTxs {
			if *tx.ToForgeL1TxsNum == *_dbTx.ToForgeL1TxsNum &&
				tx.Position == _dbTx.Position {
				dbTx = new(common.L1Tx)
				*dbTx = _dbTx
				break
			}
		}
		assert.Equal(t, &tx, dbTx) //nolint:gosec
	}

	// Check Batches
	assert.Equal(t, len(block.Rollup.Batches), len(syncBlock.Rollup.Batches))
	dbBatches, err := s.historyDB.GetAllBatches()
	require.Nil(t, err)

	dbL1CoordinatorTxs, err := s.historyDB.GetAllL1CoordinatorTxs()
	require.Nil(t, err)
	// fmt.Printf("DBG dbL1CoordinatorTxs: %+v\n", dbL1CoordinatorTxs)
	dbL2Txs, err := s.historyDB.GetAllL2Txs()
	require.Nil(t, err)
	// fmt.Printf("DBG dbL2Txs: %+v\n", dbL2Txs)
	dbExits, err := s.historyDB.GetAllExits()
	require.Nil(t, err)
	// dbL1CoordinatorTxs := []common.L1Tx{}
	for i, batch := range block.Rollup.Batches {
		var dbBatch *common.Batch
		// Find batch in DB output
		for _, _dbBatch := range dbBatches {
			if batch.Batch.BatchNum == _dbBatch.BatchNum {
				dbBatch = new(common.Batch)
				*dbBatch = _dbBatch
				break
			}
		}
		syncBatch := syncBlock.Rollup.Batches[i]

		// We don't care about TotalFeesUSD.  Use the syncBatch that
		// has a TotalFeesUSD inserted by the HistoryDB
		batch.Batch.TotalFeesUSD = syncBatch.Batch.TotalFeesUSD
		batch.CreatedAccounts = syncBatch.CreatedAccounts // til doesn't output CreatedAccounts
		batch.Batch.NumAccounts = len(batch.CreatedAccounts)

		// Test field by field to facilitate debugging of errors
		assert.Equal(t, batch.L1CoordinatorTxs, syncBatch.L1CoordinatorTxs)
		assert.Equal(t, batch.L2Txs, syncBatch.L2Txs)
		// In exit tree, we only check AccountIdx and Balance, because
		// it's what we have precomputed before.
		for j := range batch.ExitTree {
			exit := &batch.ExitTree[j]
			assert.Equal(t, exit.AccountIdx, syncBatch.ExitTree[j].AccountIdx)
			assert.Equal(t, exit.Balance, syncBatch.ExitTree[j].Balance)
			*exit = syncBatch.ExitTree[j]
		}
		// We are collecting fees after blockNum=2 in 2 idxs
		if block.Block.EthBlockNum > 2 {
			// fmt.Printf("DBG collectedFees: %+v\n", syncBatch.Batch.CollectedFees)
			assert.Equal(t, 2, len(syncBatch.Batch.CollectedFees))
		}
		batch.Batch.CollectedFees = syncBatch.Batch.CollectedFees
		assert.Equal(t, batch.Batch, syncBatch.Batch)
		assert.Equal(t, batch, syncBatch)
		assert.Equal(t, &batch.Batch, dbBatch) //nolint:gosec

		// Check L1CoordinatorTxs from DB
		for _, tx := range batch.L1CoordinatorTxs {
			var dbTx *common.L1Tx
			// Find tx in DB output
			for _, _dbTx := range dbL1CoordinatorTxs {
				if *tx.BatchNum == *_dbTx.BatchNum &&
					tx.Position == _dbTx.Position {
					dbTx = new(common.L1Tx)
					*dbTx = _dbTx
					break
				}
			}
			assert.Equal(t, &tx, dbTx) //nolint:gosec
		}

		// Check L2Txs from DB
		for _, tx := range batch.L2Txs {
			var dbTx *common.L2Tx
			// Find tx in DB output
			for _, _dbTx := range dbL2Txs {
				if tx.BatchNum == _dbTx.BatchNum &&
					tx.Position == _dbTx.Position {
					dbTx = new(common.L2Tx)
					*dbTx = _dbTx
					break
				}
			}
			assert.Equal(t, &tx, dbTx) //nolint:gosec
		}

		// Check Exits from DB
		for _, exit := range batch.ExitTree {
			var dbExit *common.ExitInfo
			// Find exit in DB output
			for _, _dbExit := range dbExits {
				if exit.BatchNum == _dbExit.BatchNum &&
					exit.AccountIdx == _dbExit.AccountIdx {
					dbExit = new(common.ExitInfo)
					*dbExit = _dbExit
					break
				}
			}
			// Compare MerkleProof in JSON because unmarshaled 0
			// big.Int leaves the internal big.Int array at nil,
			// and gives trouble when comparing big.Int with
			// internal big.Int array != nil but empty.
			mtp, err := json.Marshal(exit.MerkleProof)
			require.Nil(t, err)
			dbMtp, err := json.Marshal(dbExit.MerkleProof)
			require.Nil(t, err)
			assert.Equal(t, mtp, dbMtp)
			dbExit.MerkleProof = exit.MerkleProof
			assert.Equal(t, &exit, dbExit) //nolint:gosec
		}
	}
}

func TestSync(t *testing.T) {
	//
	// Setup
	//

	ctx := context.Background()
	// Int State DB
	dir, err := ioutil.TempDir("", "tmpdb")
	require.Nil(t, err)
	defer assert.Nil(t, os.RemoveAll(dir))

	stateDB, err := statedb.NewStateDB(dir, statedb.TypeSynchronizer, 32)
	assert.Nil(t, err)

	// Init History DB
	pass := os.Getenv("POSTGRES_PASS")
	db, err := dbUtils.InitSQLDB(5432, "localhost", "hermez", pass, "hermez")
	require.Nil(t, err)
	historyDB := historydb.NewHistoryDB(db)
	// Clear DB
	test.WipeDB(historyDB.DB())

	// Init eth client
	var timer timer
	clientSetup := test.NewClientSetupExample()
	bootCoordAddr := clientSetup.AuctionVariables.BootCoordinator
	client := test.NewClient(true, &timer, &ethCommon.Address{}, clientSetup)

	// Create Synchronizer
	s, err := NewSynchronizer(client, historyDB, stateDB, Config{
		StartBlockNum: ConfigStartBlockNum{
			Rollup:   1,
			Auction:  1,
			WDelayer: 1,
		},
	})
	require.Nil(t, err)

	//
	// First Sync from an initial state
	//

	// Test Sync for rollup genesis block
	syncBlock, discards, err := s.Sync2(ctx, nil)
	require.Nil(t, err)
	require.Nil(t, discards)
	require.NotNil(t, syncBlock)
	assert.Equal(t, int64(1), syncBlock.Block.EthBlockNum)
	dbBlocks, err := s.historyDB.GetAllBlocks()
	require.Nil(t, err)
	assert.Equal(t, 2, len(dbBlocks))
	assert.Equal(t, int64(1), dbBlocks[1].EthBlockNum)

	// Sync again and expect no new blocks
	syncBlock, discards, err = s.Sync2(ctx, nil)
	require.Nil(t, err)
	require.Nil(t, discards)
	require.Nil(t, syncBlock)

	//
	// Generate blockchain and smart contract data, and fill the test smart contracts
	//

	// Generate blockchain data with til
	set1 := `
		Type: Blockchain

		AddToken(1)
		AddToken(2)
		AddToken(3)

		CreateAccountDeposit(1) C: 2000 // Idx=256+2=258
		CreateAccountDeposit(2) A: 2000 // Idx=256+3=259
		CreateAccountDeposit(1) D: 500  // Idx=256+4=260
		CreateAccountDeposit(2) B: 500  // Idx=256+5=261
		CreateAccountDeposit(2) C: 500  // Idx=256+6=262

		CreateAccountCoordinator(1) A // Idx=256+0=256
		CreateAccountCoordinator(1) B // Idx=256+1=257

		> batchL1 // forge L1UserTxs{nil}, freeze defined L1UserTxs{5}
		> batchL1 // forge defined L1UserTxs{5}, freeze L1UserTxs{nil}
		> block // blockNum=2

		CreateAccountDepositTransfer(1) E-A: 1000, 200 // Idx=256+7=263
		ForceExit(1) A: 100
		ForceTransfer(1) A-D: 100

		Transfer(1) C-A: 100 (200)
		Exit(1) D: 30 (200)

		> batchL1 // forge L1UserTxs{nil}, freeze defined L1UserTxs{2}
		> batchL1 // forge L1UserTxs{2}, freeze defined L1UserTxs{nil}
		> block // blockNum=3

	`
	tc := til.NewContext(common.RollupConstMaxL1UserTx)
	blocks, err := tc.GenerateBlocks(set1)
	require.Nil(t, err)
	// Sanity check
	require.Equal(t, 2, len(blocks))
	// blocks 0 (blockNum=2)
	i := 0
	require.Equal(t, 2, int(blocks[i].Block.EthBlockNum))
	require.Equal(t, 3, len(blocks[i].Rollup.AddedTokens))
	require.Equal(t, 5, len(blocks[i].Rollup.L1UserTxs))
	require.Equal(t, 2, len(blocks[i].Rollup.Batches))
	require.Equal(t, 2, len(blocks[i].Rollup.Batches[0].L1CoordinatorTxs))
	// blocks 1 (blockNum=3)
	i = 1
	require.Equal(t, 3, int(blocks[i].Block.EthBlockNum))
	require.Equal(t, 3, len(blocks[i].Rollup.L1UserTxs))
	require.Equal(t, 2, len(blocks[i].Rollup.Batches))
	require.Equal(t, 2, len(blocks[i].Rollup.Batches[0].L2Txs))

	// Generate extra required data
	for _, block := range blocks {
		for _, token := range block.Rollup.AddedTokens {
			consts := eth.ERC20Consts{
				Name:     fmt.Sprintf("Token %d", token.TokenID),
				Symbol:   fmt.Sprintf("TK%d", token.TokenID),
				Decimals: 18,
			}
			tokenConsts[token.TokenID] = consts
			client.CtlAddERC20(token.EthAddr, consts)
		}
	}

	// Add block data to the smart contracts
	for _, block := range blocks {
		for _, token := range block.Rollup.AddedTokens {
			_, err := client.RollupAddTokenSimple(token.EthAddr, clientSetup.RollupVariables.FeeAddToken)
			require.Nil(t, err)
		}
		for _, tx := range block.Rollup.L1UserTxs {
			client.CtlSetAddr(tx.FromEthAddr)
			_, err := client.RollupL1UserTxERC20ETH(tx.FromBJJ, int64(tx.FromIdx), tx.LoadAmount, tx.Amount,
				uint32(tx.TokenID), int64(tx.ToIdx))
			require.Nil(t, err)
		}
		client.CtlSetAddr(bootCoordAddr)
		feeIdxCoordinator := []common.Idx{}
		if block.Block.EthBlockNum > 2 {
			// After blockNum=2 we have some accounts, use them as
			// coordinator owned to receive fees.
			feeIdxCoordinator = []common.Idx{common.Idx(256), common.Idx(259)}
		}
		for _, batch := range block.Rollup.Batches {
			_, err := client.RollupForgeBatch(&eth.RollupForgeBatchArgs{
				NewLastIdx:            batch.Batch.LastIdx,
				NewStRoot:             batch.Batch.StateRoot,
				NewExitRoot:           batch.Batch.ExitRoot,
				L1CoordinatorTxs:      batch.L1CoordinatorTxs,
				L1CoordinatorTxsAuths: [][]byte{}, // Intentionally empty
				L2TxsData:             batch.L2Txs,
				FeeIdxCoordinator:     feeIdxCoordinator,
				// Circuit selector
				VerifierIdx: 0, // Intentionally empty
				L1Batch:     batch.L1Batch,
				ProofA:      [2]*big.Int{},    // Intentionally empty
				ProofB:      [2][2]*big.Int{}, // Intentionally empty
				ProofC:      [2]*big.Int{},    // Intentionally empty
			})
			require.Nil(t, err)
		}
		// Mine block and sync
		client.CtlMineBlock()
	}

	// Fill extra fields not generated by til in til block
	openToForge := int64(0)
	toForgeL1TxsNum := int64(0)
	l1UserTxsLen := map[int64]int{} // ForgeL1TxsNum -> len(L1UserTxs)
	for i := range blocks {
		block := &blocks[i]
		// Count number of L1UserTxs in each queue, to figure out later
		// position of L1CoordinatorTxs and L2Txs
		for j := range block.Rollup.L1UserTxs {
			tx := &block.Rollup.L1UserTxs[j]
			l1UserTxsLen[*tx.ToForgeL1TxsNum]++
			if tx.Type == common.TxTypeForceExit {
				forceExits[*tx.ToForgeL1TxsNum] = append(forceExits[*tx.ToForgeL1TxsNum],
					common.ExitInfo{
						AccountIdx: tx.FromIdx,
						Balance:    tx.Amount,
					})
			}
		}
		for j := range block.Rollup.Batches {
			batch := &block.Rollup.Batches[j]
			if batch.L1Batch {
				// Set BatchNum for forged L1UserTxs to til blocks
				bn := batch.Batch.BatchNum
				for k := range blocks {
					block := &blocks[k]
					for l := range block.Rollup.L1UserTxs {
						tx := &block.Rollup.L1UserTxs[l]
						if *tx.ToForgeL1TxsNum == openToForge {
							tx.BatchNum = &bn
						}
					}
				}
				openToForge++
			}

			batch.Batch.EthBlockNum = block.Block.EthBlockNum
			batch.Batch.ForgerAddr = bootCoordAddr // til doesn't fill the batch forger addr
			if batch.L1Batch {
				toForgeL1TxsNumCpy := toForgeL1TxsNum
				batch.Batch.ForgeL1TxsNum = &toForgeL1TxsNumCpy // til doesn't fill the ForgeL1TxsNum
				toForgeL1TxsNum++
			}

			batchNum := batch.Batch.BatchNum
			for k := range batch.L1CoordinatorTxs {
				tx := &batch.L1CoordinatorTxs[k]
				tx.BatchNum = &batchNum
				tx.EthBlockNum = batch.Batch.EthBlockNum
			}
		}
	}

	// Fill expected positions in L1CoordinatorTxs and L2Txs
	for i := range blocks {
		block := &blocks[i]
		for j := range block.Rollup.Batches {
			batch := &block.Rollup.Batches[j]
			position := 0
			if batch.L1Batch {
				position = l1UserTxsLen[*batch.Batch.ForgeL1TxsNum]
			}
			for k := range batch.L1CoordinatorTxs {
				tx := &batch.L1CoordinatorTxs[k]
				tx.Position = position
				position++
				nTx, err := common.NewL1Tx(tx)
				require.Nil(t, err)
				*tx = *nTx
			}
			for k := range batch.L2Txs {
				tx := &batch.L2Txs[k]
				tx.Position = position
				position++
				nonces[tx.FromIdx]++
				tx.Nonce = nonces[tx.FromIdx]
				nTx, err := common.NewL2Tx(tx)
				require.Nil(t, err)
				*tx = *nTx
			}
		}
	}

	// Fill ExitTree (only AccountIdx and Balance)
	for i := range blocks {
		block := &blocks[i]
		for j := range block.Rollup.Batches {
			batch := &block.Rollup.Batches[j]
			if batch.L1Batch {
				for forgeL1TxsNum, exits := range forceExits {
					if forgeL1TxsNum == *batch.Batch.ForgeL1TxsNum {
						batch.ExitTree = append(batch.ExitTree, exits...)
					}
				}
			}
			for k := range batch.L2Txs {
				tx := &batch.L2Txs[k]
				if tx.Type == common.TxTypeExit {
					batch.ExitTree = append(batch.ExitTree, common.ExitInfo{
						AccountIdx: tx.FromIdx,
						Balance:    tx.Amount,
					})
				}
			}
		}
	}

	//
	// Sync to synchronize the current state from the test smart contracts,
	// and check the outcome
	//

	// Block 2

	syncBlock, discards, err = s.Sync2(ctx, nil)
	require.Nil(t, err)
	require.Nil(t, discards)
	require.NotNil(t, syncBlock)
	assert.Equal(t, int64(2), syncBlock.Block.EthBlockNum)

	checkSyncBlock(t, s, 2, &blocks[0], syncBlock)

	// Block 3

	syncBlock, discards, err = s.Sync2(ctx, nil)
	require.Nil(t, err)
	require.Nil(t, discards)
	require.NotNil(t, syncBlock)
	assert.Equal(t, int64(3), syncBlock.Block.EthBlockNum)

	checkSyncBlock(t, s, 3, &blocks[1], syncBlock)

	// TODO: Reorg will be properly tested once we have the mock ethClient implemented
	/*
		// Force a Reorg
		lastSavedBlock, err := historyDB.GetLastBlock()
		require.Nil(t, err)

		lastSavedBlock.EthBlockNum++
		err = historyDB.AddBlock(lastSavedBlock)
		require.Nil(t, err)

		lastSavedBlock.EthBlockNum++
		err = historyDB.AddBlock(lastSavedBlock)
		require.Nil(t, err)

		log.Debugf("Wait for the blockchain to generate some blocks...")
		time.Sleep(40 * time.Second)


		err = s.Sync()
		require.Nil(t, err)
	*/
}
