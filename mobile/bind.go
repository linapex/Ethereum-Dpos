
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:43</date>
//</624342653282029568>


//包含绑定包中的所有包装。

package geth

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

//签名者是一个接口，当合同要求
//提交前签署事务的方法。
type Signer interface {
	Sign(*Address, *Transaction) (tx *Transaction, _ error)
}

type signer struct {
	sign bind.SignerFn
}

func (s *signer) Sign(addr *Address, unsignedTx *Transaction) (signedTx *Transaction, _ error) {
	sig, err := s.sign(types.HomesteadSigner{}, addr.address, unsignedTx.tx)
	if err != nil {
		return nil, err
	}
	return &Transaction{sig}, nil
}

//Callopts是对合同调用请求进行微调的选项集合。
type CallOpts struct {
	opts bind.CallOpts
}

//newcallopts为合同调用创建新的选项集。
func NewCallOpts() *CallOpts {
	return new(CallOpts)
}

func (opts *CallOpts) IsPending() bool    { return opts.opts.Pending }
/*c（opts*callopts）getgaslimit（）int64返回0/*todo（karalabe）*/

//没有身份保护，getContext无法可靠实现（https://github.com/golang/go/issues/16876）
即使是这样，将GO上下文的细微部分解压缩到Java也是很困难的。
//func（opts*callopts）getContext（）*context返回&context opts.opts.context

func（opts*callopts）setpending（挂起bool）opts.opts.pending=挂起
func（opts*callopts）setgaslimit（limit int64）/*todo（karalabe）*/ }

func (opts *CallOpts) SetContext(context *Context) { opts.opts.Context = context.context }

//TransactioOpts是创建
//有效的以太坊事务。
type TransactOpts struct {
	opts bind.TransactOpts
}

func (opts *TransactOpts) GetFrom() *Address    { return &Address{opts.opts.From} }
func (opts *TransactOpts) GetNonce() int64      { return opts.opts.Nonce.Int64() }
func (opts *TransactOpts) GetValue() *BigInt    { return &BigInt{opts.opts.Value} }
func (opts *TransactOpts) GetGasPrice() *BigInt { return &BigInt{opts.opts.GasPrice} }
func (opts *TransactOpts) GetGasLimit() int64   { return int64(opts.opts.GasLimit) }

//没有身份保护，getsigner无法可靠实现（https://github.com/golang/go/issues/16876）
//func（opts*transactioopts）getsigner（）签名者返回&签名者opts.opts.signer

//没有身份保护，getContext无法可靠实现（https://github.com/golang/go/issues/16876）
//即使这样，将GO上下文的细微部分解压缩到Java也是很困难的。
//func（opts*transactionopts）getContext（）*context返回&context opts.opts.context

func (opts *TransactOpts) SetFrom(from *Address) { opts.opts.From = from.address }
func (opts *TransactOpts) SetNonce(nonce int64)  { opts.opts.Nonce = big.NewInt(nonce) }
func (opts *TransactOpts) SetSigner(s Signer) {
	opts.opts.Signer = func(signer types.Signer, addr common.Address, tx *types.Transaction) (*types.Transaction, error) {
		sig, err := s.Sign(&Address{addr}, &Transaction{tx})
		if err != nil {
			return nil, err
		}
		return sig.tx, nil
	}
}
func (opts *TransactOpts) SetValue(value *BigInt)      { opts.opts.Value = value.bigint }
func (opts *TransactOpts) SetGasPrice(price *BigInt)   { opts.opts.GasPrice = price.bigint }
func (opts *TransactOpts) SetGasLimit(limit int64)     { opts.opts.GasLimit = uint64(limit) }
func (opts *TransactOpts) SetContext(context *Context) { opts.opts.Context = context.context }

//BoundContract是反映在
//以太坊网络。它包含由
//要操作的更高级别合同绑定。
type BoundContract struct {
	contract *bind.BoundContract
	address  common.Address
	deployer *types.Transaction
}

//DeployContract将合同部署到以太坊区块链上，并绑定
//带有包装的部署地址。
func DeployContract(opts *TransactOpts, abiJSON string, bytecode []byte, client *EthereumClient, args *Interfaces) (contract *BoundContract, _ error) {
//将合同部署到网络
	parsed, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return nil, err
	}
	addr, tx, bound, err := bind.DeployContract(&opts.opts, parsed, common.CopyBytes(bytecode), client.client, args.objects...)
	if err != nil {
		return nil, err
	}
	return &BoundContract{
		contract: bound,
		address:  addr,
		deployer: tx,
	}, nil
}

//bindcontact创建一个低级合同接口，通过该接口调用和
//交易可以通过。
func BindContract(address *Address, abiJSON string, client *EthereumClient) (contract *BoundContract, _ error) {
	parsed, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return nil, err
	}
	return &BoundContract{
		contract: bind.NewBoundContract(address.address, parsed, client.client, client.client, client.client),
		address:  address.address,
	}, nil
}

func (c *BoundContract) GetAddress() *Address { return &Address{c.address} }
func (c *BoundContract) GetDeployer() *Transaction {
	if c.deployer == nil {
		return nil
	}
	return &Transaction{c.deployer}
}

//调用调用（常量）contract方法，参数作为输入值，并且
//将输出设置为结果。
func (c *BoundContract) Call(opts *CallOpts, out *Interfaces, method string, args *Interfaces) error {
	if len(out.objects) == 1 {
		result := out.objects[0]
		if err := c.contract.Call(&opts.opts, result, method, args.objects...); err != nil {
			return err
		}
		out.objects[0] = result
	} else {
		results := make([]interface{}, len(out.objects))
		copy(results, out.objects)
		if err := c.contract.Call(&opts.opts, &results, method, args.objects...); err != nil {
			return err
		}
		copy(out.objects, results)
	}
	return nil
}

//Transact使用参数作为输入值调用（付费）Contract方法。
func (c *BoundContract) Transact(opts *TransactOpts, method string, args *Interfaces) (tx *Transaction, _ error) {
	rawTx, err := c.contract.Transact(&opts.opts, method, args.objects...)
	if err != nil {
		return nil, err
	}
	return &Transaction{rawTx}, nil
}

//转账启动普通交易以将资金转移到合同，调用
//它的默认方法（如果有）。
func (c *BoundContract) Transfer(opts *TransactOpts) (tx *Transaction, _ error) {
	rawTx, err := c.contract.Transfer(&opts.opts)
	if err != nil {
		return nil, err
	}
	return &Transaction{rawTx}, nil
}

