
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:33</date>
//</624342613859766272>


package ens

//go:生成abigen--sol contract/ens.sol--exc contract/abstractens.sol:abstractens--pkg contract--out contract/ens.go
//go:生成abigen--sol contract/fifsregistrar.sol--exc contract/abstractens.sol:abstractens--pkg contract--out contract/fifsregistrar.go
//go:生成abigen--sol contract/publicdolver.sol--exc contract/abstractens.sol:abstractens--pkg contract--out contract/publicdolver.go

import (
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contracts/ens/contract"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	MainNetAddress = common.HexToAddress("0x314159265dD8dbb310642f98f50C066173C1259b")
	TestNetAddress = common.HexToAddress("0x112234455c3a32fd11230c42e7bccd4a84e02010")
)

//
type ENS struct {
	*contract.ENSSession
	contractBackend bind.ContractBackend
}

//newens创建了一个结构，它公开了方便的高级操作，以便与
//以太坊名称服务。
func NewENS(transactOpts *bind.TransactOpts, contractAddr common.Address, contractBackend bind.ContractBackend) (*ENS, error) {
	ens, err := contract.NewENS(contractAddr, contractBackend)
	if err != nil {
		return nil, err
	}

	return &ENS{
		&contract.ENSSession{
			Contract:     ens,
			TransactOpts: *transactOpts,
		},
		contractBackend,
	}, nil
}

//Deployeens部署ENS名称服务的一个实例，具有“先进先服务”的根注册器。
func DeployENS(transactOpts *bind.TransactOpts, contractBackend bind.ContractBackend) (common.Address, *ENS, error) {
//部署ENS注册表。
	ensAddr, _, _, err := contract.DeployENS(transactOpts, contractBackend)
	if err != nil {
		return ensAddr, nil, err
	}

	ens, err := NewENS(transactOpts, ensAddr, contractBackend)
	if err != nil {
		return ensAddr, nil, err
	}

//部署注册器。
	regAddr, _, _, err := contract.DeployFIFSRegistrar(transactOpts, contractBackend, ensAddr, [32]byte{})
	if err != nil {
		return ensAddr, nil, err
	}
//将注册器设置为ENS根目录的所有者。
	if _, err = ens.SetOwner([32]byte{}, regAddr); err != nil {
		return ensAddr, nil, err
	}

	return ensAddr, ens, nil
}

func ensParentNode(name string) (common.Hash, common.Hash) {
	parts := strings.SplitN(name, ".", 2)
	label := crypto.Keccak256Hash([]byte(parts[0]))
	if len(parts) == 1 {
		return [32]byte{}, label
	} else {
		parentNode, parentLabel := ensParentNode(parts[1])
		return crypto.Keccak256Hash(parentNode[:], parentLabel[:]), label
	}
}

func EnsNode(name string) common.Hash {
	parentNode, parentLabel := ensParentNode(name)
	return crypto.Keccak256Hash(parentNode[:], parentLabel[:])
}

func (self *ENS) getResolver(node [32]byte) (*contract.PublicResolverSession, error) {
	resolverAddr, err := self.Resolver(node)
	if err != nil {
		return nil, err
	}

	resolver, err := contract.NewPublicResolver(resolverAddr, self.contractBackend)
	if err != nil {
		return nil, err
	}

	return &contract.PublicResolverSession{
		Contract:     resolver,
		TransactOpts: self.TransactOpts,
	}, nil
}

func (self *ENS) getRegistrar(node [32]byte) (*contract.FIFSRegistrarSession, error) {
	registrarAddr, err := self.Owner(node)
	if err != nil {
		return nil, err
	}

	registrar, err := contract.NewFIFSRegistrar(registrarAddr, self.contractBackend)
	if err != nil {
		return nil, err
	}

	return &contract.FIFSRegistrarSession{
		Contract:     registrar,
		TransactOpts: self.TransactOpts,
	}, nil
}

//解析是一个非事务性调用，返回与名称关联的内容哈希。
func (self *ENS) Resolve(name string) (common.Hash, error) {
	node := EnsNode(name)

	resolver, err := self.getResolver(node)
	if err != nil {
		return common.Hash{}, err
	}

	ret, err := resolver.Content(node)
	if err != nil {
		return common.Hash{}, err
	}

	return common.BytesToHash(ret[:]), nil
}

//注册为调用者注册一个新域名，使其成为新名称的所有者。
//仅当父域的注册器实现FIFS注册器协议时才有效。
func (self *ENS) Register(name string) (*types.Transaction, error) {
	parentNode, label := ensParentNode(name)
	registrar, err := self.getRegistrar(parentNode)
	if err != nil {
		return nil, err
	}
	return registrar.Contract.Register(&self.TransactOpts, label, self.TransactOpts.From)
}

//setContentHash设置与名称关联的内容哈希。只有当呼叫方
//拥有名称，关联的冲突解决程序实现了一个“setcontent”函数。
func (self *ENS) SetContentHash(name string, hash common.Hash) (*types.Transaction, error) {
	node := EnsNode(name)

	resolver, err := self.getResolver(node)
	if err != nil {
		return nil, err
	}

	opts := self.TransactOpts
	opts.GasLimit = 200000
	return resolver.Contract.SetContent(&opts, node, hash)
}

