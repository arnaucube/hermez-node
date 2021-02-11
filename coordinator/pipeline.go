package coordinator

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/hermeznetwork/hermez-node/batchbuilder"
	"github.com/hermeznetwork/hermez-node/common"
	"github.com/hermeznetwork/hermez-node/db/historydb"
	"github.com/hermeznetwork/hermez-node/db/l2db"
	"github.com/hermeznetwork/hermez-node/eth"
	"github.com/hermeznetwork/hermez-node/log"
	"github.com/hermeznetwork/hermez-node/prover"
	"github.com/hermeznetwork/hermez-node/synchronizer"
	"github.com/hermeznetwork/hermez-node/txselector"
	"github.com/hermeznetwork/tracerr"
)

type statsVars struct {
	Stats synchronizer.Stats
	Vars  synchronizer.SCVariablesPtr
}

// Pipeline manages the forging of batches with parallel server proofs
type Pipeline struct {
	num    int
	cfg    Config
	consts synchronizer.SCConsts

	// state
	batchNum                     common.BatchNum
	lastScheduledL1BatchBlockNum int64
	lastForgeL1TxsNum            int64
	started                      bool

	proversPool  *ProversPool
	provers      []prover.Client
	txManager    *TxManager
	historyDB    *historydb.HistoryDB
	l2DB         *l2db.L2DB
	txSelector   *txselector.TxSelector
	batchBuilder *batchbuilder.BatchBuilder
	purger       *Purger

	stats       synchronizer.Stats
	vars        synchronizer.SCVariables
	statsVarsCh chan statsVars

	ctx    context.Context
	wg     sync.WaitGroup
	cancel context.CancelFunc
}

// NewPipeline creates a new Pipeline
func NewPipeline(ctx context.Context,
	cfg Config,
	num int, // Pipeline sequential number
	historyDB *historydb.HistoryDB,
	l2DB *l2db.L2DB,
	txSelector *txselector.TxSelector,
	batchBuilder *batchbuilder.BatchBuilder,
	purger *Purger,
	txManager *TxManager,
	provers []prover.Client,
	scConsts *synchronizer.SCConsts,
) (*Pipeline, error) {
	proversPool := NewProversPool(len(provers))
	proversPoolSize := 0
	for _, prover := range provers {
		if err := prover.WaitReady(ctx); err != nil {
			log.Errorw("prover.WaitReady", "err", err)
		} else {
			proversPool.Add(ctx, prover)
			proversPoolSize++
		}
	}
	if proversPoolSize == 0 {
		return nil, tracerr.Wrap(fmt.Errorf("no provers in the pool"))
	}
	return &Pipeline{
		num:          num,
		cfg:          cfg,
		historyDB:    historyDB,
		l2DB:         l2DB,
		txSelector:   txSelector,
		batchBuilder: batchBuilder,
		provers:      provers,
		proversPool:  proversPool,
		purger:       purger,
		txManager:    txManager,
		consts:       *scConsts,
		statsVarsCh:  make(chan statsVars, queueLen),
	}, nil
}

// SetSyncStatsVars is a thread safe method to sets the synchronizer Stats
func (p *Pipeline) SetSyncStatsVars(ctx context.Context, stats *synchronizer.Stats, vars *synchronizer.SCVariablesPtr) {
	select {
	case p.statsVarsCh <- statsVars{Stats: *stats, Vars: *vars}:
	case <-ctx.Done():
	}
}

// reset pipeline state
func (p *Pipeline) reset(batchNum common.BatchNum,
	stats *synchronizer.Stats, vars *synchronizer.SCVariables) error {
	p.batchNum = batchNum
	p.lastForgeL1TxsNum = stats.Sync.LastForgeL1TxsNum
	p.stats = *stats
	p.vars = *vars
	p.lastScheduledL1BatchBlockNum = 0

	err := p.txSelector.Reset(p.batchNum)
	if err != nil {
		return tracerr.Wrap(err)
	}
	err = p.batchBuilder.Reset(p.batchNum, true)
	if err != nil {
		return tracerr.Wrap(err)
	}
	return nil
}

func (p *Pipeline) syncSCVars(vars synchronizer.SCVariablesPtr) {
	updateSCVars(&p.vars, vars)
}

// handleForgeBatch calls p.forgeBatch to forge the batch and get the zkInputs,
// and then waits for an available proof server and sends the zkInputs to it so
// that the proof computation begins.
func (p *Pipeline) handleForgeBatch(ctx context.Context, batchNum common.BatchNum) (*BatchInfo, error) {
	batchInfo, err := p.forgeBatch(batchNum)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	} else if err != nil {
		if tracerr.Unwrap(err) == errLastL1BatchNotSynced {
			log.Warnw("forgeBatch: scheduled L1Batch too early", "err", err,
				"lastForgeL1TxsNum", p.lastForgeL1TxsNum,
				"syncLastForgeL1TxsNum", p.stats.Sync.LastForgeL1TxsNum)
		} else {
			log.Errorw("forgeBatch", "err", err)
		}
		return nil, err
	}
	// 6. Wait for an available server proof (blocking call)
	serverProof, err := p.proversPool.Get(ctx)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	} else if err != nil {
		log.Errorw("proversPool.Get", "err", err)
		return nil, err
	}
	batchInfo.ServerProof = serverProof
	if err := p.sendServerProof(ctx, batchInfo); ctx.Err() != nil {
		return nil, ctx.Err()
	} else if err != nil {
		log.Errorw("sendServerProof", "err", err)
		batchInfo.ServerProof = nil
		p.proversPool.Add(ctx, serverProof)
		return nil, err
	}
	return batchInfo, nil
}

// Start the forging pipeline
func (p *Pipeline) Start(batchNum common.BatchNum,
	stats *synchronizer.Stats, vars *synchronizer.SCVariables) error {
	if p.started {
		log.Fatal("Pipeline already started")
	}
	p.started = true

	if err := p.reset(batchNum, stats, vars); err != nil {
		return tracerr.Wrap(err)
	}
	p.ctx, p.cancel = context.WithCancel(context.Background())

	queueSize := 1
	batchChSentServerProof := make(chan *BatchInfo, queueSize)

	p.wg.Add(1)
	go func() {
		waitDuration := zeroDuration
		for {
			select {
			case <-p.ctx.Done():
				log.Info("Pipeline forgeBatch loop done")
				p.wg.Done()
				return
			case statsVars := <-p.statsVarsCh:
				p.stats = statsVars.Stats
				p.syncSCVars(statsVars.Vars)
			case <-time.After(waitDuration):
				batchNum = p.batchNum + 1
				batchInfo, err := p.handleForgeBatch(p.ctx, batchNum)
				if p.ctx.Err() != nil {
					continue
				} else if err != nil {
					waitDuration = p.cfg.SyncRetryInterval
					continue
				}
				p.batchNum = batchNum
				select {
				case batchChSentServerProof <- batchInfo:
				case <-p.ctx.Done():
				}
			}
		}
	}()

	p.wg.Add(1)
	go func() {
		for {
			select {
			case <-p.ctx.Done():
				log.Info("Pipeline waitServerProofSendEth loop done")
				p.wg.Done()
				return
			case batchInfo := <-batchChSentServerProof:
				err := p.waitServerProof(p.ctx, batchInfo)
				// We are done with this serverProof, add it back to the pool
				p.proversPool.Add(p.ctx, batchInfo.ServerProof)
				batchInfo.ServerProof = nil
				if p.ctx.Err() != nil {
					continue
				} else if err != nil {
					log.Errorw("waitServerProof", "err", err)
					continue
				}
				p.txManager.AddBatch(p.ctx, batchInfo)
			}
		}
	}()
	return nil
}

// Stop the forging pipeline
func (p *Pipeline) Stop(ctx context.Context) {
	if !p.started {
		log.Fatal("Pipeline already stopped")
	}
	p.started = false
	log.Info("Stopping Pipeline...")
	p.cancel()
	p.wg.Wait()
	for _, prover := range p.provers {
		if err := prover.Cancel(ctx); ctx.Err() != nil {
			continue
		} else if err != nil {
			log.Errorw("prover.Cancel", "err", err)
		}
	}
}

// sendServerProof sends the circuit inputs to the proof server
func (p *Pipeline) sendServerProof(ctx context.Context, batchInfo *BatchInfo) error {
	p.cfg.debugBatchStore(batchInfo)

	// 7. Call the selected idle server proof with BatchBuilder output,
	// save server proof info for batchNum
	if err := batchInfo.ServerProof.CalculateProof(ctx, batchInfo.ZKInputs); err != nil {
		return tracerr.Wrap(err)
	}
	return nil
}

// forgeBatch forges the batchNum batch.
func (p *Pipeline) forgeBatch(batchNum common.BatchNum) (batchInfo *BatchInfo, err error) {
	// remove transactions from the pool that have been there for too long
	_, err = p.purger.InvalidateMaybe(p.l2DB, p.txSelector.LocalAccountsDB(),
		p.stats.Sync.LastBlock.Num, int64(batchNum))
	if err != nil {
		return nil, tracerr.Wrap(err)
	}
	_, err = p.purger.PurgeMaybe(p.l2DB, p.stats.Sync.LastBlock.Num, int64(batchNum))
	if err != nil {
		return nil, tracerr.Wrap(err)
	}
	// Structure to accumulate data and metadata of the batch
	batchInfo = &BatchInfo{PipelineNum: p.num, BatchNum: batchNum}
	batchInfo.Debug.StartTimestamp = time.Now()
	batchInfo.Debug.StartBlockNum = p.stats.Eth.LastBlock.Num + 1

	selectionCfg := &txselector.SelectionConfig{
		MaxL1UserTxs:      common.RollupConstMaxL1UserTx,
		TxProcessorConfig: p.cfg.TxProcessorConfig,
	}

	var poolL2Txs []common.PoolL2Tx
	var discardedL2Txs []common.PoolL2Tx
	var l1UserTxsExtra, l1CoordTxs []common.L1Tx
	var auths [][]byte
	var coordIdxs []common.Idx

	// 1. Decide if we forge L2Tx or L1+L2Tx
	if p.shouldL1L2Batch(batchInfo) {
		batchInfo.L1Batch = true
		defer func() {
			// If there's no error, update the parameters related
			// to the last L1Batch forged
			if err == nil {
				p.lastScheduledL1BatchBlockNum = p.stats.Eth.LastBlock.Num + 1
				p.lastForgeL1TxsNum++
			}
		}()
		if p.lastForgeL1TxsNum != p.stats.Sync.LastForgeL1TxsNum {
			return nil, tracerr.Wrap(errLastL1BatchNotSynced)
		}
		// 2a: L1+L2 txs
		l1UserTxs, err := p.historyDB.GetUnforgedL1UserTxs(p.lastForgeL1TxsNum + 1)
		if err != nil {
			return nil, tracerr.Wrap(err)
		}
		coordIdxs, auths, l1UserTxsExtra, l1CoordTxs, poolL2Txs, discardedL2Txs, err =
			p.txSelector.GetL1L2TxSelection(selectionCfg, l1UserTxs)
		if err != nil {
			return nil, tracerr.Wrap(err)
		}
	} else {
		// 2b: only L2 txs
		coordIdxs, auths, l1CoordTxs, poolL2Txs, discardedL2Txs, err =
			p.txSelector.GetL2TxSelection(selectionCfg)
		if err != nil {
			return nil, tracerr.Wrap(err)
		}
		l1UserTxsExtra = nil
	}

	// 3.  Save metadata from TxSelector output for BatchNum
	batchInfo.L1UserTxsExtra = l1UserTxsExtra
	batchInfo.L1CoordTxs = l1CoordTxs
	batchInfo.L1CoordinatorTxsAuths = auths
	batchInfo.CoordIdxs = coordIdxs
	batchInfo.VerifierIdx = p.cfg.VerifierIdx

	if err := p.l2DB.StartForging(common.TxIDsFromPoolL2Txs(poolL2Txs), batchInfo.BatchNum); err != nil {
		return nil, tracerr.Wrap(err)
	}
	if err := p.l2DB.UpdateTxsInfo(discardedL2Txs); err != nil {
		return nil, tracerr.Wrap(err)
	}

	// Invalidate transactions that become invalid beause of
	// the poolL2Txs selected.  Will mark as invalid the txs that have a
	// (fromIdx, nonce) which already appears in the selected txs (includes
	// all the nonces smaller than the current one)
	err = p.l2DB.InvalidateOldNonces(idxsNonceFromPoolL2Txs(poolL2Txs), batchInfo.BatchNum)
	if err != nil {
		return nil, tracerr.Wrap(err)
	}

	// 4. Call BatchBuilder with TxSelector output
	configBatch := &batchbuilder.ConfigBatch{
		TxProcessorConfig: p.cfg.TxProcessorConfig,
	}
	zkInputs, err := p.batchBuilder.BuildBatch(coordIdxs, configBatch, l1UserTxsExtra,
		l1CoordTxs, poolL2Txs)
	if err != nil {
		return nil, tracerr.Wrap(err)
	}
	l2Txs, err := common.PoolL2TxsToL2Txs(poolL2Txs) // NOTE: This is a big uggly, find a better way
	if err != nil {
		return nil, tracerr.Wrap(err)
	}
	batchInfo.L2Txs = l2Txs

	// 5. Save metadata from BatchBuilder output for BatchNum
	batchInfo.ZKInputs = zkInputs
	batchInfo.Debug.Status = StatusForged
	p.cfg.debugBatchStore(batchInfo)
	log.Infow("Pipeline: batch forged internally", "batch", batchInfo.BatchNum)

	return batchInfo, nil
}

// waitServerProof gets the generated zkProof & sends it to the SmartContract
func (p *Pipeline) waitServerProof(ctx context.Context, batchInfo *BatchInfo) error {
	proof, pubInputs, err := batchInfo.ServerProof.GetProof(ctx) // blocking call, until not resolved don't continue. Returns when the proof server has calculated the proof
	if err != nil {
		return tracerr.Wrap(err)
	}
	batchInfo.Proof = proof
	batchInfo.PublicInputs = pubInputs
	batchInfo.ForgeBatchArgs = prepareForgeBatchArgs(batchInfo)
	batchInfo.Debug.Status = StatusProof
	p.cfg.debugBatchStore(batchInfo)
	log.Infow("Pipeline: batch proof calculated", "batch", batchInfo.BatchNum)
	return nil
}

func (p *Pipeline) shouldL1L2Batch(batchInfo *BatchInfo) bool {
	// Take the lastL1BatchBlockNum as the biggest between the last
	// scheduled one, and the synchronized one.
	lastL1BatchBlockNum := p.lastScheduledL1BatchBlockNum
	if p.stats.Sync.LastL1BatchBlock > lastL1BatchBlockNum {
		lastL1BatchBlockNum = p.stats.Sync.LastL1BatchBlock
	}
	// Set Debug information
	batchInfo.Debug.LastScheduledL1BatchBlockNum = p.lastScheduledL1BatchBlockNum
	batchInfo.Debug.LastL1BatchBlock = p.stats.Sync.LastL1BatchBlock
	batchInfo.Debug.LastL1BatchBlockDelta = p.stats.Eth.LastBlock.Num + 1 - lastL1BatchBlockNum
	batchInfo.Debug.L1BatchBlockScheduleDeadline =
		int64(float64(p.vars.Rollup.ForgeL1L2BatchTimeout-1) * p.cfg.L1BatchTimeoutPerc)
	// Return true if we have passed the l1BatchTimeoutPerc portion of the
	// range before the l1batch timeout.
	return p.stats.Eth.LastBlock.Num+1-lastL1BatchBlockNum >=
		int64(float64(p.vars.Rollup.ForgeL1L2BatchTimeout-1)*p.cfg.L1BatchTimeoutPerc)
}

func prepareForgeBatchArgs(batchInfo *BatchInfo) *eth.RollupForgeBatchArgs {
	proof := batchInfo.Proof
	zki := batchInfo.ZKInputs
	return &eth.RollupForgeBatchArgs{
		NewLastIdx:            int64(zki.Metadata.NewLastIdxRaw),
		NewStRoot:             zki.Metadata.NewStateRootRaw.BigInt(),
		NewExitRoot:           zki.Metadata.NewExitRootRaw.BigInt(),
		L1UserTxs:             batchInfo.L1UserTxsExtra,
		L1CoordinatorTxs:      batchInfo.L1CoordTxs,
		L1CoordinatorTxsAuths: batchInfo.L1CoordinatorTxsAuths,
		L2TxsData:             batchInfo.L2Txs,
		FeeIdxCoordinator:     batchInfo.CoordIdxs,
		// Circuit selector
		VerifierIdx: batchInfo.VerifierIdx,
		L1Batch:     batchInfo.L1Batch,
		ProofA:      [2]*big.Int{proof.PiA[0], proof.PiA[1]},
		// Implementation of the verifier need a swap on the proofB vector
		ProofB: [2][2]*big.Int{
			{proof.PiB[0][1], proof.PiB[0][0]},
			{proof.PiB[1][1], proof.PiB[1][0]},
		},
		ProofC: [2]*big.Int{proof.PiC[0], proof.PiC[1]},
	}
}
