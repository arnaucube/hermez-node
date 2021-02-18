package debugapi

import (
	"crypto/ecdsa"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"sync"
	"testing"

	"github.com/dghubble/sling"
	ethCommon "github.com/ethereum/go-ethereum/common"
	ethCrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/hermeznetwork/hermez-node/common"
	dbUtils "github.com/hermeznetwork/hermez-node/db"
	"github.com/hermeznetwork/hermez-node/db/historydb"
	"github.com/hermeznetwork/hermez-node/db/statedb"
	"github.com/hermeznetwork/hermez-node/test"
	"github.com/hermeznetwork/hermez-node/test/til"
	"github.com/hermeznetwork/hermez-node/txprocessor"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

func newAccount(t *testing.T, i int) *common.Account {
	var sk babyjub.PrivateKey
	copy(sk[:], []byte(strconv.Itoa(i))) // only for testing
	pk := sk.Public()

	var key ecdsa.PrivateKey
	key.D = big.NewInt(int64(i + 1)) // only for testing
	key.PublicKey.X, key.PublicKey.Y = ethCrypto.S256().ScalarBaseMult(key.D.Bytes())
	key.Curve = ethCrypto.S256()
	address := ethCrypto.PubkeyToAddress(key.PublicKey)

	return &common.Account{
		Idx:     common.Idx(256 + i),
		TokenID: common.TokenID(i),
		Nonce:   common.Nonce(i),
		Balance: big.NewInt(1000),
		BJJ:     pk.Compress(),
		EthAddr: address,
	}
}

func TestDebugAPI(t *testing.T) {
	dir, err := ioutil.TempDir("", "tmpdb")
	require.Nil(t, err)

	sdb, err := statedb.NewStateDB(statedb.Config{Path: dir, Keep: 128, Type: statedb.TypeSynchronizer, NLevels: 32})
	require.Nil(t, err)
	err = sdb.MakeCheckpoint() // Make a checkpoint to increment the batchNum
	require.Nil(t, err)

	addr := "localhost:12345"
	// We won't test the sync/stats endpoint, so we can se the Syncrhonizer to nil
	debugAPI := NewDebugAPI(addr, nil, sdb, nil)

	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err := debugAPI.Run(ctx)
		require.Nil(t, err)
		wg.Done()
	}()

	var accounts []common.Account
	for i := 0; i < 16; i++ {
		account := newAccount(t, i)
		accounts = append(accounts, *account)
		_, err = sdb.CreateAccount(account.Idx, account)
		require.Nil(t, err)
	}
	// Make a checkpoint (batchNum 2) to make the accounts available in Last
	err = sdb.MakeCheckpoint()
	require.Nil(t, err)

	url := fmt.Sprintf("http://%v/debug/", addr)

	var batchNum common.BatchNum
	req, err := sling.New().Get(url).Path("sdb/batchnum").ReceiveSuccess(&batchNum)
	require.Equal(t, http.StatusOK, req.StatusCode)
	require.Nil(t, err)
	assert.Equal(t, common.BatchNum(2), batchNum)

	var mtroot *big.Int
	req, err = sling.New().Get(url).Path("sdb/mtroot").ReceiveSuccess(&mtroot)
	require.Equal(t, http.StatusOK, req.StatusCode)
	require.Nil(t, err)
	// Testing against a hardcoded value obtained by running the test and
	// printing the value previously.
	assert.Equal(t, "21765339739823365993496282904432398015268846626944509989242908567129545640185",
		mtroot.String())

	var accountAPI common.Account
	req, err = sling.New().Get(url).
		Path(fmt.Sprintf("sdb/accounts/%v", accounts[0].Idx)).
		ReceiveSuccess(&accountAPI)
	require.Equal(t, http.StatusOK, req.StatusCode)
	require.Nil(t, err)
	assert.Equal(t, accounts[0], accountAPI)

	var accountsAPI []common.Account
	req, err = sling.New().Get(url).Path("sdb/accounts").ReceiveSuccess(&accountsAPI)
	require.Equal(t, http.StatusOK, req.StatusCode)
	require.Nil(t, err)
	assert.Equal(t, accounts, accountsAPI)

	cancel()
	wg.Wait()
}

func TestDebugAPITokenBalances(t *testing.T) {
	dir, err := ioutil.TempDir("", "tmpdb")
	require.Nil(t, err)

	sdb, err := statedb.NewStateDB(statedb.Config{Path: dir, Keep: 128, Type: statedb.TypeSynchronizer, NLevels: 32})
	require.Nil(t, err)

	// Init History DB
	pass := os.Getenv("POSTGRES_PASS")
	db, err := dbUtils.InitSQLDB(5432, "localhost", "hermez", pass, "hermez")
	require.NoError(t, err)
	historyDB := historydb.NewHistoryDB(db, nil)
	// Clear DB
	test.WipeDB(historyDB.DB())

	set := `
		Type: Blockchain

		AddToken(1)
		AddToken(2)
		AddToken(3)

		CreateAccountDeposit(1) A: 1000
		CreateAccountDeposit(2) A: 2000
		CreateAccountDeposit(1) B: 100
		CreateAccountDeposit(2) B: 200
		CreateAccountDeposit(2) C: 400

		> batchL1 // forge L1UserTxs{nil}, freeze defined L1UserTxs{5}
		> batchL1 // forge defined L1UserTxs{5}, freeze L1UserTxs{nil}
		> block // blockNum=2
	`

	chainID := uint16(0)
	tc := til.NewContext(chainID, common.RollupConstMaxL1UserTx)
	tilCfgExtra := til.ConfigExtra{
		BootCoordAddr: ethCommon.HexToAddress("0xE39fEc6224708f0772D2A74fd3f9055A90E0A9f2"),
		CoordUser:     "A",
	}
	blocks, err := tc.GenerateBlocks(set)
	require.NoError(t, err)

	err = tc.FillBlocksExtra(blocks, &tilCfgExtra)
	require.NoError(t, err)
	tc.FillBlocksL1UserTxsBatchNum(blocks)
	err = tc.FillBlocksForgedL1UserTxs(blocks)
	require.NoError(t, err)

	tpc := txprocessor.Config{
		NLevels:  32,
		MaxTx:    100,
		ChainID:  chainID,
		MaxFeeTx: common.RollupConstMaxFeeIdxCoordinator,
		MaxL1Tx:  common.RollupConstMaxL1Tx,
	}
	tp := txprocessor.NewTxProcessor(sdb, tpc)

	for _, block := range blocks {
		require.NoError(t, historyDB.AddBlockSCData(&block))
		for _, batch := range block.Rollup.Batches {
			_, err := tp.ProcessTxs(batch.Batch.FeeIdxsCoordinator,
				batch.L1UserTxs, batch.L1CoordinatorTxs, []common.PoolL2Tx{})
			require.NoError(t, err)
		}
	}

	addr := "localhost:12345"
	// We won't test the sync/stats endpoint, so we can se the Syncrhonizer to nil
	debugAPI := NewDebugAPI(addr, historyDB, sdb, nil)

	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err := debugAPI.Run(ctx)
		require.Nil(t, err)
		wg.Done()
	}()

	var accounts []common.Account
	for i := 0; i < 16; i++ {
		account := newAccount(t, i)
		accounts = append(accounts, *account)
		_, err = sdb.CreateAccount(account.Idx, account)
		require.Nil(t, err)
	}
	// Make a checkpoint (batchNum 2) to make the accounts available in Last
	err = sdb.MakeCheckpoint()
	require.Nil(t, err)

	url := fmt.Sprintf("http://%v/debug/", addr)

	var batchNum common.BatchNum
	req, err := sling.New().Get(url).Path("sdb/batchnum").ReceiveSuccess(&batchNum)
	require.Equal(t, http.StatusOK, req.StatusCode)
	require.Nil(t, err)
	assert.Equal(t, common.BatchNum(2), batchNum)

	var mtroot *big.Int
	req, err = sling.New().Get(url).Path("sdb/mtroot").ReceiveSuccess(&mtroot)
	require.Equal(t, http.StatusOK, req.StatusCode)
	require.Nil(t, err)
	// Testing against a hardcoded value obtained by running the test and
	// printing the value previously.
	assert.Equal(t, "21765339739823365993496282904432398015268846626944509989242908567129545640185",
		mtroot.String())

	var accountAPI common.Account
	req, err = sling.New().Get(url).
		Path(fmt.Sprintf("sdb/accounts/%v", accounts[0].Idx)).
		ReceiveSuccess(&accountAPI)
	require.Equal(t, http.StatusOK, req.StatusCode)
	require.Nil(t, err)
	assert.Equal(t, accounts[0], accountAPI)

	var accountsAPI []common.Account
	req, err = sling.New().Get(url).Path("sdb/accounts").ReceiveSuccess(&accountsAPI)
	require.Equal(t, http.StatusOK, req.StatusCode)
	require.Nil(t, err)
	assert.Equal(t, accounts, accountsAPI)

	cancel()
	wg.Wait()
}
