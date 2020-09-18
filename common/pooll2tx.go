package common

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"time"

	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/iden3/go-iden3-crypto/poseidon"
)

// Nonce represents the nonce value in a uint64, which has the method Bytes that returns a byte array of length 5 (40 bits).
type Nonce uint64

// Bytes returns a byte array of length 5 representing the Nonce
func (n Nonce) Bytes() ([5]byte, error) {
	if n > maxNonceValue {
		return [5]byte{}, ErrNonceOverflow
	}
	var nonceBytes [8]byte
	binary.BigEndian.PutUint64(nonceBytes[:], uint64(n))
	var b [5]byte
	copy(b[:], nonceBytes[3:])
	return b, nil
}

// BigInt returns the *big.Int representation of the Nonce value
func (n Nonce) BigInt() *big.Int {
	return big.NewInt(int64(n))
}

// NonceFromBytes returns Nonce from a [5]byte
func NonceFromBytes(b [5]byte) Nonce {
	var nonceBytes [8]byte
	copy(nonceBytes[3:], b[:])
	nonce := binary.BigEndian.Uint64(nonceBytes[:])
	return Nonce(nonce)
}

// PoolL2Tx is a struct that represents a L2Tx sent by an account to the coordinator hat is waiting to be forged
type PoolL2Tx struct {
	// Stored in DB: mandatory fileds
	TxID        TxID               `meddler:"tx_id"`
	FromIdx     Idx                `meddler:"from_idx"` // FromIdx is used by L1Tx/Deposit to indicate the Idx receiver of the L1Tx.LoadAmount (deposit)
	ToIdx       Idx                `meddler:"to_idx"`   // ToIdx is ignored in L1Tx/Deposit, but used in the L1Tx/DepositAndTransfer
	ToEthAddr   ethCommon.Address  `meddler:"to_eth_addr"`
	ToBJJ       *babyjub.PublicKey `meddler:"to_bjj"` // TODO: stop using json, use scanner/valuer
	TokenID     TokenID            `meddler:"token_id"`
	Amount      *big.Int           `meddler:"amount,bigint"` // TODO: change to float16
	AmountFloat float64            `meddler:"amount_f"`      // TODO: change to float16
	USD         float64            `meddler:"value_usd"`     // TODO: change to float16
	Fee         FeeSelector        `meddler:"fee"`
	Nonce       Nonce              `meddler:"nonce"` // effective 40 bits used
	State       PoolL2TxState      `meddler:"state"`
	Signature   *babyjub.Signature `meddler:"signature"`         // tx signature
	Timestamp   time.Time          `meddler:"timestamp,utctime"` // time when added to the tx pool
	// Stored in DB: optional fileds, may be uninitialized
	BatchNum          BatchNum           `meddler:"batch_num,zeroisnull"`   // batchNum in which this tx was forged. Presence indicates "forged" state.
	RqFromIdx         Idx                `meddler:"rq_from_idx,zeroisnull"` // FromIdx is used by L1Tx/Deposit to indicate the Idx receiver of the L1Tx.LoadAmount (deposit)
	RqToIdx           Idx                `meddler:"rq_to_idx,zeroisnull"`   // FromIdx is used by L1Tx/Deposit to indicate the Idx receiver of the L1Tx.LoadAmount (deposit)
	RqToEthAddr       ethCommon.Address  `meddler:"rq_to_eth_addr"`
	RqToBJJ           *babyjub.PublicKey `meddler:"rq_to_bjj"` // TODO: stop using json, use scanner/valuer
	RqTokenID         TokenID            `meddler:"rq_token_id,zeroisnull"`
	RqAmount          *big.Int           `meddler:"rq_amount,bigintnull"` // TODO: change to float16
	RqFee             FeeSelector        `meddler:"rq_fee,zeroisnull"`
	RqNonce           uint64             `meddler:"rq_nonce,zeroisnull"` // effective 48 bits used
	AbsoluteFee       float64            `meddler:"fee_usd,zeroisnull"`
	AbsoluteFeeUpdate time.Time          `meddler:"usd_update,utctimez"`
	Type              TxType             `meddler:"tx_type"`
	// Extra metadata, may be uninitialized
	RqTxCompressedData []byte `meddler:"-"` // 253 bits, optional for atomic txs
}

// TxCompressedData spec:
// [ 1 bits  ] toBJJSign // 1 byte
// [ 8 bits  ] userFee // 1 byte
// [ 40 bits ] nonce // 5 bytes
// [ 32 bits ] tokenID // 4 bytes
// [ 16 bits ] amountFloat16 // 2 bytes
// [ 48 bits ] toIdx // 6 bytes
// [ 48 bits ] fromIdx // 6 bytes
// [ 16 bits ] chainId // 2 bytes
// [ 32 bits ] signatureConstant // 4 bytes
// Total bits compressed data:  241 bits // 31 bytes in *big.Int representation
func (tx *PoolL2Tx) TxCompressedData() (*big.Int, error) {
	// sigconstant
	sc, ok := new(big.Int).SetString("3322668559", 10)
	if !ok {
		return nil, fmt.Errorf("error parsing SignatureConstant")
	}

	amountFloat16, err := NewFloat16(tx.Amount)
	if err != nil {
		return nil, err
	}
	var b [31]byte
	toBJJSign := byte(0)
	if babyjub.PointCoordSign(tx.ToBJJ.X) {
		toBJJSign = byte(1)
	}
	b[0] = toBJJSign
	b[1] = byte(tx.Fee)
	nonceBytes, err := tx.Nonce.Bytes()
	if err != nil {
		return nil, err
	}
	copy(b[2:7], nonceBytes[:])
	copy(b[7:11], tx.TokenID.Bytes())
	copy(b[11:13], amountFloat16.Bytes())
	copy(b[13+2:19], tx.ToIdx.Bytes())
	copy(b[19+2:25], tx.FromIdx.Bytes())
	copy(b[25:27], []byte{0, 1, 0, 0}) // TODO check js implementation (unexpected behaviour from test vector generated from js)
	copy(b[27:31], sc.Bytes())

	bi := new(big.Int).SetBytes(b[:])
	return bi, nil
}

// TxCompressedDataV2 spec:
// [ 1 bits  ] toBJJSign // 1 byte
// [ 8 bits  ] userFee // 1 byte
// [ 40 bits ] nonce // 5 bytes
// [ 32 bits ] tokenID // 4 bytes
// [ 16 bits ] amountFloat16 // 2 bytes
// [ 48 bits ] toIdx // 6 bytes
// [ 48 bits ] fromIdx // 6 bytes
// Total bits compressed data:  193 bits // 25 bytes in *big.Int representation
func (tx *PoolL2Tx) TxCompressedDataV2() (*big.Int, error) {
	amountFloat16, err := NewFloat16(tx.Amount)
	if err != nil {
		return nil, err
	}
	var b [25]byte
	toBJJSign := byte(0)
	if babyjub.PointCoordSign(tx.ToBJJ.X) {
		toBJJSign = byte(1)
	}
	b[0] = toBJJSign
	b[1] = byte(tx.Fee)
	nonceBytes, err := tx.Nonce.Bytes()
	if err != nil {
		return nil, err
	}
	copy(b[2:7], nonceBytes[:])
	copy(b[7:11], tx.TokenID.Bytes())
	copy(b[11:13], amountFloat16.Bytes())
	copy(b[13+2:19], tx.ToIdx.Bytes())
	copy(b[19+2:25], tx.FromIdx.Bytes())

	bi := new(big.Int).SetBytes(b[:])
	return bi, nil
}

// HashToSign returns the computed Poseidon hash from the *PoolL2Tx that will be signed by the sender.
func (tx *PoolL2Tx) HashToSign() (*big.Int, error) {
	toCompressedData, err := tx.TxCompressedData()
	if err != nil {
		return nil, err
	}
	toEthAddr := EthAddrToBigInt(tx.ToEthAddr)
	toBJJAy := tx.ToBJJ.Y
	rqTxCompressedDataV2, err := tx.TxCompressedDataV2()
	if err != nil {
		return nil, err
	}

	return poseidon.Hash([]*big.Int{toCompressedData, toEthAddr, toBJJAy, rqTxCompressedDataV2, EthAddrToBigInt(tx.RqToEthAddr), tx.RqToBJJ.Y})
}

// VerifySignature returns true if the signature verification is correct for the given PublicKey
func (tx *PoolL2Tx) VerifySignature(pk *babyjub.PublicKey) bool {
	h, err := tx.HashToSign()
	if err != nil {
		return false
	}
	return pk.VerifyPoseidon(h, tx.Signature)
}

// L2Tx returns a *L2Tx from the PoolL2Tx
func (tx *PoolL2Tx) L2Tx() *L2Tx {
	return &L2Tx{
		TxID:     tx.TxID,
		BatchNum: tx.BatchNum,
		FromIdx:  tx.FromIdx,
		ToIdx:    tx.ToIdx,
		Amount:   tx.Amount,
		Fee:      tx.Fee,
		Nonce:    tx.Nonce,
		Type:     tx.Type,
	}
}

// Tx returns a *Tx from the PoolL2Tx
func (tx *PoolL2Tx) Tx() *Tx {
	return &Tx{
		TxID:    tx.TxID,
		FromIdx: tx.FromIdx,
		ToIdx:   tx.ToIdx,
		Amount:  tx.Amount,
		Nonce:   tx.Nonce,
		Fee:     tx.Fee,
		Type:    tx.Type,
	}
}

// PoolL2TxsToL2Txs returns an array of []*L2Tx from an array of []*PoolL2Tx
func PoolL2TxsToL2Txs(txs []*PoolL2Tx) []*L2Tx {
	var r []*L2Tx
	for _, tx := range txs {
		r = append(r, tx.L2Tx())
	}
	return r
}

// PoolL2TxState is a struct that represents the status of a L2 transaction
type PoolL2TxState string

const (
	// PoolL2TxStatePending represents a valid L2Tx that hasn't started the forging process
	PoolL2TxStatePending PoolL2TxState = "pend"
	// PoolL2TxStateForging represents a valid L2Tx that has started the forging process
	PoolL2TxStateForging PoolL2TxState = "fing"
	// PoolL2TxStateForged represents a L2Tx that has already been forged
	PoolL2TxStateForged PoolL2TxState = "fged"
	// PoolL2TxStateInvalid represents a L2Tx that has been invalidated
	PoolL2TxStateInvalid PoolL2TxState = "invl"
)
