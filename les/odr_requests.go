
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:40</date>
//</624342644373327872>


//package light实现可按需检索的状态和链对象
//对于以太坊Light客户端。
package les

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/light"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
)

var (
	errInvalidMessageType  = errors.New("invalid message type")
	errInvalidEntryCount   = errors.New("invalid number of response entries")
	errHeaderUnavailable   = errors.New("header unavailable")
	errTxHashMismatch      = errors.New("transaction hash mismatch")
	errUncleHashMismatch   = errors.New("uncle hash mismatch")
	errReceiptHashMismatch = errors.New("receipt hash mismatch")
	errDataHashMismatch    = errors.New("data hash mismatch")
	errCHTHashMismatch     = errors.New("cht hash mismatch")
	errCHTNumberMismatch   = errors.New("cht number mismatch")
	errUselessNodes        = errors.New("useless nodes in merkle proof nodeset")
)

type LesOdrRequest interface {
	GetCost(*peer) uint64
	CanSend(*peer) bool
	Request(uint64, *peer) error
	Validate(ethdb.Database, *Msg) error
}

func LesRequest(req light.OdrRequest) LesOdrRequest {
	switch r := req.(type) {
	case *light.BlockRequest:
		return (*BlockRequest)(r)
	case *light.ReceiptsRequest:
		return (*ReceiptsRequest)(r)
	case *light.TrieRequest:
		return (*TrieRequest)(r)
	case *light.CodeRequest:
		return (*CodeRequest)(r)
	case *light.ChtRequest:
		return (*ChtRequest)(r)
	case *light.BloomRequest:
		return (*BloomRequest)(r)
	default:
		return nil
	}
}

//BlockRequest是块体的ODR请求类型
type BlockRequest light.BlockRequest

//getcost根据服务返回给定ODR请求的成本
//同行成本表（lesodrequest的实现）
func (r *BlockRequest) GetCost(peer *peer) uint64 {
	return peer.GetRequestCost(GetBlockBodiesMsg, 1)
}

//cansend告诉某个对等机是否适合服务于给定的请求
func (r *BlockRequest) CanSend(peer *peer) bool {
	return peer.HasBlock(r.Hash, r.Number)
}

//请求向LES网络发送一个ODR请求（LESODRREQUEST的实现）
func (r *BlockRequest) Request(reqID uint64, peer *peer) error {
	peer.Log().Debug("Requesting block body", "hash", r.Hash)
	return peer.RequestBodies(reqID, r.GetCost(peer), []common.Hash{r.Hash})
}

//有效处理来自LES网络的ODR请求回复消息
//如果消息是有效的答复，则返回true并将结果存储在内存中
//到请求（lesodrequest的实现）
func (r *BlockRequest) Validate(db ethdb.Database, msg *Msg) error {
	log.Debug("Validating block body", "hash", r.Hash)

//确保我们有一个正确的信息与一个单一的块体
	if msg.MsgType != MsgBlockBodies {
		return errInvalidMessageType
	}
	bodies := msg.Obj.([]*types.Body)
	if len(bodies) != 1 {
		return errInvalidEntryCount
	}
	body := bodies[0]

//检索存储的头并根据它验证块内容
	header := rawdb.ReadHeader(db, r.Hash, r.Number)
	if header == nil {
		return errHeaderUnavailable
	}
	if header.TxHash != types.DeriveSha(types.Transactions(body.Transactions)) {
		return errTxHashMismatch
	}
	if header.UncleHash != types.CalcUncleHash(body.Uncles) {
		return errUncleHashMismatch
	}
//验证通过，编码和存储RLP
	data, err := rlp.EncodeToBytes(body)
	if err != nil {
		return err
	}
	r.Rlp = data
	return nil
}

//ReceiptsRequest是按块哈希列出的块接收的ODR请求类型
type ReceiptsRequest light.ReceiptsRequest

//getcost根据服务返回给定ODR请求的成本
//同行成本表（lesodrequest的实现）
func (r *ReceiptsRequest) GetCost(peer *peer) uint64 {
	return peer.GetRequestCost(GetReceiptsMsg, 1)
}

//cansend告诉某个对等机是否适合服务于给定的请求
func (r *ReceiptsRequest) CanSend(peer *peer) bool {
	return peer.HasBlock(r.Hash, r.Number)
}

//请求向LES网络发送一个ODR请求（LESODRREQUEST的实现）
func (r *ReceiptsRequest) Request(reqID uint64, peer *peer) error {
	peer.Log().Debug("Requesting block receipts", "hash", r.Hash)
	return peer.RequestReceipts(reqID, r.GetCost(peer), []common.Hash{r.Hash})
}

//有效处理来自LES网络的ODR请求回复消息
//如果消息是有效的答复，则返回true并将结果存储在内存中
//到请求（lesodrequest的实现）
func (r *ReceiptsRequest) Validate(db ethdb.Database, msg *Msg) error {
	log.Debug("Validating block receipts", "hash", r.Hash)

//确保我们有一个正确的消息和一个单块收据
	if msg.MsgType != MsgReceipts {
		return errInvalidMessageType
	}
	receipts := msg.Obj.([]types.Receipts)
	if len(receipts) != 1 {
		return errInvalidEntryCount
	}
	receipt := receipts[0]

//检索存储的标题并根据其验证收据内容
	header := rawdb.ReadHeader(db, r.Hash, r.Number)
	if header == nil {
		return errHeaderUnavailable
	}
	if header.ReceiptHash != types.DeriveSha(receipt) {
		return errReceiptHashMismatch
	}
//验证通过，存储并返回
	r.Receipts = receipt
	return nil
}

type ProofReq struct {
	BHash       common.Hash
	AccKey, Key []byte
	FromLevel   uint
}

//状态/存储trie项的ODR请求类型，请参见leSodrRequest接口
type TrieRequest light.TrieRequest

//getcost根据服务返回给定ODR请求的成本
//同行成本表（lesodrequest的实现）
func (r *TrieRequest) GetCost(peer *peer) uint64 {
	switch peer.version {
	case lpv1:
		return peer.GetRequestCost(GetProofsV1Msg, 1)
	case lpv2:
		return peer.GetRequestCost(GetProofsV2Msg, 1)
	default:
		panic(nil)
	}
}

//cansend告诉某个对等机是否适合服务于给定的请求
func (r *TrieRequest) CanSend(peer *peer) bool {
	return peer.HasBlock(r.Id.BlockHash, r.Id.BlockNumber)
}

//请求向LES网络发送一个ODR请求（LESODRREQUEST的实现）
func (r *TrieRequest) Request(reqID uint64, peer *peer) error {
	peer.Log().Debug("Requesting trie proof", "root", r.Id.Root, "key", r.Key)
	req := ProofReq{
		BHash:  r.Id.BlockHash,
		AccKey: r.Id.AccKey,
		Key:    r.Key,
	}
	return peer.RequestProofs(reqID, r.GetCost(peer), []ProofReq{req})
}

//有效处理来自LES网络的ODR请求回复消息
//如果消息是有效的答复，则返回true并将结果存储在内存中
//到请求（lesodrequest的实现）
func (r *TrieRequest) Validate(db ethdb.Database, msg *Msg) error {
	log.Debug("Validating trie proof", "root", r.Id.Root, "key", r.Key)

	switch msg.MsgType {
	case MsgProofsV1:
		proofs := msg.Obj.([]light.NodeList)
		if len(proofs) != 1 {
			return errInvalidEntryCount
		}
		nodeSet := proofs[0].NodeSet()
//验证证明，如果签出则保存
		if _, _, err := trie.VerifyProof(r.Id.Root, r.Key, nodeSet); err != nil {
			return fmt.Errorf("merkle proof verification failed: %v", err)
		}
		r.Proof = nodeSet
		return nil

	case MsgProofsV2:
		proofs := msg.Obj.(light.NodeList)
//验证证明，如果签出则保存
		nodeSet := proofs.NodeSet()
		reads := &readTraceDB{db: nodeSet}
		if _, _, err := trie.VerifyProof(r.Id.Root, r.Key, reads); err != nil {
			return fmt.Errorf("merkle proof verification failed: %v", err)
		}
//检查VerifyProof是否已读取所有节点
		if len(reads.reads) != nodeSet.KeyCount() {
			return errUselessNodes
		}
		r.Proof = nodeSet
		return nil

	default:
		return errInvalidMessageType
	}
}

type CodeReq struct {
	BHash  common.Hash
	AccKey []byte
}

//节点数据的ODR请求类型（用于检索合同代码），请参见LESODRREQUEST接口
type CodeRequest light.CodeRequest

//getcost根据服务返回给定ODR请求的成本
//同行成本表（lesodrequest的实现）
func (r *CodeRequest) GetCost(peer *peer) uint64 {
	return peer.GetRequestCost(GetCodeMsg, 1)
}

//cansend告诉某个对等机是否适合服务于给定的请求
func (r *CodeRequest) CanSend(peer *peer) bool {
	return peer.HasBlock(r.Id.BlockHash, r.Id.BlockNumber)
}

//请求向LES网络发送一个ODR请求（LESODRREQUEST的实现）
func (r *CodeRequest) Request(reqID uint64, peer *peer) error {
	peer.Log().Debug("Requesting code data", "hash", r.Hash)
	req := CodeReq{
		BHash:  r.Id.BlockHash,
		AccKey: r.Id.AccKey,
	}
	return peer.RequestCode(reqID, r.GetCost(peer), []CodeReq{req})
}

//有效处理来自LES网络的ODR请求回复消息
//如果消息是有效的答复，则返回true并将结果存储在内存中
//到请求（lesodrequest的实现）
func (r *CodeRequest) Validate(db ethdb.Database, msg *Msg) error {
	log.Debug("Validating code data", "hash", r.Hash)

//确保我们有一个带有单个代码元素的正确消息
	if msg.MsgType != MsgCode {
		return errInvalidMessageType
	}
	reply := msg.Obj.([][]byte)
	if len(reply) != 1 {
		return errInvalidEntryCount
	}
	data := reply[0]

//验证数据并存储是否签出
	if hash := crypto.Keccak256Hash(data); r.Hash != hash {
		return errDataHashMismatch
	}
	r.Data = data
	return nil
}

const (
//helper trie类型常量
htCanonical = iota //规范哈希trie
htBloomBits        //布卢姆斯特里

//适用于所有helper trie请求
	auxRoot = 1
//适用于htcanonical
	auxHeader = 2
)

type HelperTrieReq struct {
	Type              uint
	TrieIdx           uint64
	Key               []byte
	FromLevel, AuxReq uint
}

type HelperTrieResps struct { //描述所有响应，而不仅仅是单个响应
	Proofs  light.NodeList
	AuxData [][]byte
}

//遗产LES / 1
type ChtReq struct {
	ChtNum, BlockNum uint64
	FromLevel        uint
}

//遗产LES / 1
type ChtResp struct {
	Header *types.Header
	Proof  []rlp.RawValue
}

//用于通过规范哈希检索请求头的ODR请求类型，请参见leSodrRequest接口
type ChtRequest light.ChtRequest

//getcost根据服务返回给定ODR请求的成本
//同行成本表（lesodrequest的实现）
func (r *ChtRequest) GetCost(peer *peer) uint64 {
	switch peer.version {
	case lpv1:
		return peer.GetRequestCost(GetHeaderProofsMsg, 1)
	case lpv2:
		return peer.GetRequestCost(GetHelperTrieProofsMsg, 1)
	default:
		panic(nil)
	}
}

//cansend告诉某个对等机是否适合服务于给定的请求
func (r *ChtRequest) CanSend(peer *peer) bool {
	peer.lock.RLock()
	defer peer.lock.RUnlock()

	return peer.headInfo.Number >= light.HelperTrieConfirmations && r.ChtNum <= (peer.headInfo.Number-light.HelperTrieConfirmations)/light.CHTFrequencyClient
}

//请求向LES网络发送一个ODR请求（LESODRREQUEST的实现）
func (r *ChtRequest) Request(reqID uint64, peer *peer) error {
	peer.Log().Debug("Requesting CHT", "cht", r.ChtNum, "block", r.BlockNum)
	var encNum [8]byte
	binary.BigEndian.PutUint64(encNum[:], r.BlockNum)
	req := HelperTrieReq{
		Type:    htCanonical,
		TrieIdx: r.ChtNum,
		Key:     encNum[:],
		AuxReq:  auxHeader,
	}
	return peer.RequestHelperTrieProofs(reqID, r.GetCost(peer), []HelperTrieReq{req})
}

//有效处理来自LES网络的ODR请求回复消息
//如果消息是有效的答复，则返回true并将结果存储在内存中
//到请求（lesodrequest的实现）
func (r *ChtRequest) Validate(db ethdb.Database, msg *Msg) error {
	log.Debug("Validating CHT", "cht", r.ChtNum, "block", r.BlockNum)

	switch msg.MsgType {
case MsgHeaderProofs: //LES/1向后兼容性
		proofs := msg.Obj.([]ChtResp)
		if len(proofs) != 1 {
			return errInvalidEntryCount
		}
		proof := proofs[0]

//验证CHT
		var encNumber [8]byte
		binary.BigEndian.PutUint64(encNumber[:], r.BlockNum)

		value, _, err := trie.VerifyProof(r.ChtRoot, encNumber[:], light.NodeList(proof.Proof).NodeSet())
		if err != nil {
			return err
		}
		var node light.ChtNode
		if err := rlp.DecodeBytes(value, &node); err != nil {
			return err
		}
		if node.Hash != proof.Header.Hash() {
			return errCHTHashMismatch
		}
//验证通过，存储并返回
		r.Header = proof.Header
		r.Proof = light.NodeList(proof.Proof).NodeSet()
		r.Td = node.Td
	case MsgHelperTrieProofs:
		resp := msg.Obj.(HelperTrieResps)
		if len(resp.AuxData) != 1 {
			return errInvalidEntryCount
		}
		nodeSet := resp.Proofs.NodeSet()
		headerEnc := resp.AuxData[0]
		if len(headerEnc) == 0 {
			return errHeaderUnavailable
		}
		header := new(types.Header)
		if err := rlp.DecodeBytes(headerEnc, header); err != nil {
			return errHeaderUnavailable
		}

//验证CHT
		var encNumber [8]byte
		binary.BigEndian.PutUint64(encNumber[:], r.BlockNum)

		reads := &readTraceDB{db: nodeSet}
		value, _, err := trie.VerifyProof(r.ChtRoot, encNumber[:], reads)
		if err != nil {
			return fmt.Errorf("merkle proof verification failed: %v", err)
		}
		if len(reads.reads) != nodeSet.KeyCount() {
			return errUselessNodes
		}

		var node light.ChtNode
		if err := rlp.DecodeBytes(value, &node); err != nil {
			return err
		}
		if node.Hash != header.Hash() {
			return errCHTHashMismatch
		}
		if r.BlockNum != header.Number.Uint64() {
			return errCHTNumberMismatch
		}
//验证通过，存储并返回
		r.Header = header
		r.Proof = nodeSet
		r.Td = node.Td
	default:
		return errInvalidMessageType
	}
	return nil
}

type BloomReq struct {
	BloomTrieNum, BitIdx, SectionIdx, FromLevel uint64
}

//用于通过规范哈希检索请求头的ODR请求类型，请参见leSodrRequest接口
type BloomRequest light.BloomRequest

//getcost根据服务返回给定ODR请求的成本
//同行成本表（lesodrequest的实现）
func (r *BloomRequest) GetCost(peer *peer) uint64 {
	return peer.GetRequestCost(GetHelperTrieProofsMsg, len(r.SectionIdxList))
}

//cansend告诉某个对等机是否适合服务于给定的请求
func (r *BloomRequest) CanSend(peer *peer) bool {
	peer.lock.RLock()
	defer peer.lock.RUnlock()

	if peer.version < lpv2 {
		return false
	}
	return peer.headInfo.Number >= light.HelperTrieConfirmations && r.BloomTrieNum <= (peer.headInfo.Number-light.HelperTrieConfirmations)/light.BloomTrieFrequency
}

//请求向LES网络发送一个ODR请求（LESODRREQUEST的实现）
func (r *BloomRequest) Request(reqID uint64, peer *peer) error {
	peer.Log().Debug("Requesting BloomBits", "bloomTrie", r.BloomTrieNum, "bitIdx", r.BitIdx, "sections", r.SectionIdxList)
	reqs := make([]HelperTrieReq, len(r.SectionIdxList))

	var encNumber [10]byte
	binary.BigEndian.PutUint16(encNumber[:2], uint16(r.BitIdx))

	for i, sectionIdx := range r.SectionIdxList {
		binary.BigEndian.PutUint64(encNumber[2:], sectionIdx)
		reqs[i] = HelperTrieReq{
			Type:    htBloomBits,
			TrieIdx: r.BloomTrieNum,
			Key:     common.CopyBytes(encNumber[:]),
		}
	}
	return peer.RequestHelperTrieProofs(reqID, r.GetCost(peer), reqs)
}

//有效处理来自LES网络的ODR请求回复消息
//如果消息是有效的答复，则返回true并将结果存储在内存中
//到请求（lesodrequest的实现）
func (r *BloomRequest) Validate(db ethdb.Database, msg *Msg) error {
	log.Debug("Validating BloomBits", "bloomTrie", r.BloomTrieNum, "bitIdx", r.BitIdx, "sections", r.SectionIdxList)

//确保我们有一个正确的消息和一个证明元素
	if msg.MsgType != MsgHelperTrieProofs {
		return errInvalidMessageType
	}
	resps := msg.Obj.(HelperTrieResps)
	proofs := resps.Proofs
	nodeSet := proofs.NodeSet()
	reads := &readTraceDB{db: nodeSet}

	r.BloomBits = make([][]byte, len(r.SectionIdxList))

//核实证据
	var encNumber [10]byte
	binary.BigEndian.PutUint16(encNumber[:2], uint16(r.BitIdx))

	for i, idx := range r.SectionIdxList {
		binary.BigEndian.PutUint64(encNumber[2:], idx)
		value, _, err := trie.VerifyProof(r.BloomTrieRoot, encNumber[:], reads)
		if err != nil {
			return err
		}
		r.BloomBits[i] = value
	}

	if len(reads.reads) != nodeSet.KeyCount() {
		return errUselessNodes
	}
	r.Proofs = nodeSet
	return nil
}

//readtracedb存储数据库读取的键。我们使用这个来检查接收到的节点
//集合只包含使证明通过所需的trie节点。
type readTraceDB struct {
	db    trie.DatabaseReader
	reads map[string]struct{}
}

//get返回存储节点
func (db *readTraceDB) Get(k []byte) ([]byte, error) {
	if db.reads == nil {
		db.reads = make(map[string]struct{})
	}
	db.reads[string(k)] = struct{}{}
	return db.db.Get(k)
}

//如果节点集包含给定的键，则返回true
func (db *readTraceDB) Has(key []byte) (bool, error) {
	_, err := db.Get(key)
	return err == nil, nil
}

