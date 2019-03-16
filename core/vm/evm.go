
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:35</date>
//</624342621531148288>


package vm

import (
	"math/big"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)

//create使用EmptyCodeHash来确保已不允许部署
//已部署的合同地址（在帐户提取之后相关）。
var emptyCodeHash = crypto.Keccak256Hash(nil)

type (
//cantransferfunc是传递保护函数的签名
	CanTransferFunc func(StateDB, common.Address, *big.Int) bool
//transferFunc是传递函数的签名
	TransferFunc func(StateDB, common.Address, common.Address, *big.Int)
//gethashfunc返回区块链中的第n个区块哈希
//并由blockhash evm op代码使用。
	GetHashFunc func(uint64) common.Hash
)

//Run运行给定的合同，并负责运行回退到字节码解释器的预编译。
func run(evm *EVM, contract *Contract, input []byte) ([]byte, error) {
	if contract.CodeAddr != nil {
		precompiles := PrecompiledContractsHomestead
		if evm.ChainConfig().IsByzantium(evm.BlockNumber) {
			precompiles = PrecompiledContractsByzantium
		}
		if p := precompiles[*contract.CodeAddr]; p != nil {
			return RunPrecompiledContract(p, input, contract)
		}
	}
	for _, interpreter := range evm.interpreters {
		if interpreter.CanRun(contract.Code) {
			if evm.interpreter != interpreter {
//确保解释器指针设置回原处
//返回到当前值。
				defer func(i Interpreter) {
					evm.interpreter = i
				}(evm.interpreter)
				evm.interpreter = interpreter
			}
			return interpreter.Run(contract, input)
		}
	}
	return nil, ErrNoCompatibleInterpreter
}

//上下文为EVM提供辅助信息。一旦提供
//它不应该被修改。
type Context struct {
//CanTransfer返回帐户是否包含
//足够的乙醚转移价值
	CanTransfer CanTransferFunc
//将乙醚从一个帐户转移到另一个帐户
	Transfer TransferFunc
//GetHash返回与n对应的哈希
	GetHash GetHashFunc

//消息信息
Origin   common.Address //提供源站信息
GasPrice *big.Int       //为Gasprice提供信息

//阻止信息
Coinbase    common.Address //为CoinBase提供信息
GasLimit    uint64         //提供气体限制信息
BlockNumber *big.Int       //提供数字信息
Time        *big.Int       //提供时间信息
Difficulty  *big.Int       //为困难提供信息
}

//EVM是以太坊虚拟机基础对象，它提供
//在给定状态下运行合同所需的工具
//提供的上下文。应该注意的是，任何错误
//通过任何调用生成的应被视为
//恢复状态并消耗所有气体操作，不检查
//应执行特定错误。翻译使
//确保生成的任何错误都被视为错误代码。
//
//EVM不应该被重用，也不是线程安全的。
type EVM struct {
//上下文提供辅助区块链相关信息
	Context
//statedb提供对底层状态的访问
	StateDB StateDB
//深度是当前调用堆栈
	depth int

//chainconfig包含有关当前链的信息
	chainConfig *params.ChainConfig
//链规则包含当前纪元的链规则
	chainRules params.Rules
//用于初始化的虚拟机配置选项
//EVM。
	vmConfig Config
//全局（到此上下文）以太坊虚拟机
//在整个Tx执行过程中使用。
	interpreters []Interpreter
	interpreter  Interpreter
//abort用于中止EVM调用操作
//注：必须按原子顺序设置
	abort int32
//callgastemp保留当前呼叫可用的气体。这是需要的，因为
//根据63/64规则和更高版本，可用气体在GasCall*中计算。
//在opcall*中应用。
	callGasTemp uint64
}

//new evm返回新的evm。返回的EVM不是线程安全的，应该
//只能使用一次。
func NewEVM(ctx Context, statedb StateDB, chainConfig *params.ChainConfig, vmConfig Config) *EVM {
	evm := &EVM{
		Context:      ctx,
		StateDB:      statedb,
		vmConfig:     vmConfig,
		chainConfig:  chainConfig,
		chainRules:   chainConfig.Rules(ctx.BlockNumber),
		interpreters: make([]Interpreter, 1),
	}

	evm.interpreters[0] = NewEVMInterpreter(evm, vmConfig)
	evm.interpreter = evm.interpreters[0]

	return evm
}

//取消取消任何正在运行的EVM操作。这可以同时调用，并且
//多次打电话是安全的。
func (evm *EVM) Cancel() {
	atomic.StoreInt32(&evm.abort, 1)
}

//解释器返回当前解释器
func (evm *EVM) Interpreter() Interpreter {
	return evm.interpreter
}

//调用执行与给定输入为的addr关联的协定
//参数。它还处理任何必要的价值转移，并采取
//创建帐户和在
//执行错误或值传输失败。
func (evm *EVM) Call(caller ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	if evm.vmConfig.NoRecursion && evm.depth > 0 {
		return nil, gas, nil
	}

//如果我们试图执行超过调用深度限制，则失败
	if evm.depth > int(params.CallCreateDepth) {
		return nil, gas, ErrDepth
	}
//如果我们试图转移的余额超过可用余额，则失败
	if !evm.Context.CanTransfer(evm.StateDB, caller.Address(), value) {
		return nil, gas, ErrInsufficientBalance
	}

	var (
		to       = AccountRef(addr)
		snapshot = evm.StateDB.Snapshot()
	)
	if !evm.StateDB.Exist(addr) {
		precompiles := PrecompiledContractsHomestead
		if evm.ChainConfig().IsByzantium(evm.BlockNumber) {
			precompiles = PrecompiledContractsByzantium
		}
		if precompiles[addr] == nil && evm.ChainConfig().IsEIP158(evm.BlockNumber) && value.Sign() == 0 {
//调用一个不存在的帐户，不要做任何事情，只需ping跟踪程序
			if evm.vmConfig.Debug && evm.depth == 0 {
				evm.vmConfig.Tracer.CaptureStart(caller.Address(), addr, false, input, gas, value)
				evm.vmConfig.Tracer.CaptureEnd(ret, 0, 0, nil)
			}
			return nil, gas, nil
		}
		evm.StateDB.CreateAccount(addr)
	}
	evm.Transfer(evm.StateDB, caller.Address(), to.Address(), value)

//初始化新合同并设置EVM要使用的代码。
//契约只是这个执行上下文的作用域环境。
	contract := NewContract(caller, to, value, gas)
	contract.SetCallCode(&addr, evm.StateDB.GetCodeHash(addr), evm.StateDB.GetCode(addr))

	start := time.Now()

//在调试模式下捕获跟踪程序开始/结束事件
	if evm.vmConfig.Debug && evm.depth == 0 {
		evm.vmConfig.Tracer.CaptureStart(caller.Address(), addr, false, input, gas, value)

defer func() { //参数的延迟评估
			evm.vmConfig.Tracer.CaptureEnd(ret, gas-contract.Gas, time.Since(start), err)
		}()
	}
	ret, err = run(evm, contract, input)

//当EVM返回错误或设置创建代码时
//在上面，我们返回快照并消耗所有剩余的气体。另外
//当我们在宅基地时，这也算是代码存储气体错误。
	if err != nil {
		evm.StateDB.RevertToSnapshot(snapshot)
		if err != errExecutionReverted {
			contract.UseGas(contract.Gas)
		}
	}
	return ret, contract.Gas, err
}

//callcode使用给定的输入执行与addr关联的协定
//作为参数。它还处理任何必要的价值转移，并采取
//创建帐户和在
//执行错误或值传输失败。
//
//callcode与call的区别在于它执行给定的地址'
//以调用方为上下文的代码。
func (evm *EVM) CallCode(caller ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	if evm.vmConfig.NoRecursion && evm.depth > 0 {
		return nil, gas, nil
	}

//如果我们试图执行超过调用深度限制，则失败
	if evm.depth > int(params.CallCreateDepth) {
		return nil, gas, ErrDepth
	}
//如果我们试图转移的余额超过可用余额，则失败
	if !evm.CanTransfer(evm.StateDB, caller.Address(), value) {
		return nil, gas, ErrInsufficientBalance
	}

	var (
		snapshot = evm.StateDB.Snapshot()
		to       = AccountRef(caller.Address())
	)
//初始化新合同并设置
//EVM。合同是此执行上下文的作用域环境
//只有。
	contract := NewContract(caller, to, value, gas)
	contract.SetCallCode(&addr, evm.StateDB.GetCodeHash(addr), evm.StateDB.GetCode(addr))

	ret, err = run(evm, contract, input)
	if err != nil {
		evm.StateDB.RevertToSnapshot(snapshot)
		if err != errExecutionReverted {
			contract.UseGas(contract.Gas)
		}
	}
	return ret, contract.Gas, err
}

//DelegateCall执行与给定输入的addr关联的协定
//作为参数。如果发生执行错误，它将反转状态。
//
//delegateCall与callcode的区别在于它执行给定的地址'
//以调用者为上下文的代码，调用者被设置为调用者的调用者。
func (evm *EVM) DelegateCall(caller ContractRef, addr common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error) {
	if evm.vmConfig.NoRecursion && evm.depth > 0 {
		return nil, gas, nil
	}
//如果我们试图执行超过调用深度限制，则失败
	if evm.depth > int(params.CallCreateDepth) {
		return nil, gas, ErrDepth
	}

	var (
		snapshot = evm.StateDB.Snapshot()
		to       = AccountRef(caller.Address())
	)

//初始化新合同并使委托值初始化
	contract := NewContract(caller, to, nil, gas).AsDelegate()
	contract.SetCallCode(&addr, evm.StateDB.GetCodeHash(addr), evm.StateDB.GetCode(addr))

	ret, err = run(evm, contract, input)
	if err != nil {
		evm.StateDB.RevertToSnapshot(snapshot)
		if err != errExecutionReverted {
			contract.UseGas(contract.Gas)
		}
	}
	return ret, contract.Gas, err
}

//staticCall使用给定的输入执行与addr关联的协定
//作为参数，同时不允许在调用期间对状态进行任何修改。
//试图执行此类修改的操作码将导致异常
//而不是执行修改。
func (evm *EVM) StaticCall(caller ContractRef, addr common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error) {
	if evm.vmConfig.NoRecursion && evm.depth > 0 {
		return nil, gas, nil
	}
//如果我们试图执行超过调用深度限制，则失败
	if evm.depth > int(params.CallCreateDepth) {
		return nil, gas, ErrDepth
	}
//确保仅当我们不在readonly中时才设置readonly
//这还确保不会为删除只读标志
//儿童电话。
	if !evm.interpreter.IsReadOnly() {
		evm.interpreter.SetReadOnly(true)
		defer func() { evm.interpreter.SetReadOnly(false) }()
	}

	var (
		to       = AccountRef(addr)
		snapshot = evm.StateDB.Snapshot()
	)
//初始化新合同并设置
//EVM。合同是此执行上下文的作用域环境
//只有。
	contract := NewContract(caller, to, new(big.Int), gas)
	contract.SetCallCode(&addr, evm.StateDB.GetCodeHash(addr), evm.StateDB.GetCode(addr))

//当EVM返回错误或设置创建代码时
//在上面，我们返回快照并消耗所有剩余的气体。另外
//当我们在宅基地时，这也算是代码存储气体错误。
	ret, err = run(evm, contract, input)
	if err != nil {
		evm.StateDB.RevertToSnapshot(snapshot)
		if err != errExecutionReverted {
			contract.UseGas(contract.Gas)
		}
	}
	return ret, contract.Gas, err
}

//创建使用代码作为部署代码创建新合同。
func (evm *EVM) create(caller ContractRef, code []byte, gas uint64, value *big.Int, address common.Address) ([]byte, common.Address, uint64, error) {
//深度检查执行。如果我们试图执行上面的
//极限。
	if evm.depth > int(params.CallCreateDepth) {
		return nil, common.Address{}, gas, ErrDepth
	}
	if !evm.CanTransfer(evm.StateDB, caller.Address(), value) {
		return nil, common.Address{}, gas, ErrInsufficientBalance
	}
	nonce := evm.StateDB.GetNonce(caller.Address())
	evm.StateDB.SetNonce(caller.Address(), nonce+1)

//确保指定地址没有现有合同
	contractHash := evm.StateDB.GetCodeHash(address)
	if evm.StateDB.GetNonce(address) != 0 || (contractHash != (common.Hash{}) && contractHash != emptyCodeHash) {
		return nil, common.Address{}, 0, ErrContractAddressCollision
	}
//在该州创建新帐户
	snapshot := evm.StateDB.Snapshot()
	evm.StateDB.CreateAccount(address)
	if evm.ChainConfig().IsEIP158(evm.BlockNumber) {
		evm.StateDB.SetNonce(address, 1)
	}
	evm.Transfer(evm.StateDB, caller.Address(), address, value)

//初始化新合同并设置
//EVM。合同是此执行上下文的作用域环境
//只有。
	contract := NewContract(caller, AccountRef(address), value, gas)
	contract.SetCallCode(&address, crypto.Keccak256Hash(code), code)

	if evm.vmConfig.NoRecursion && evm.depth > 0 {
		return nil, address, gas, nil
	}

	if evm.vmConfig.Debug && evm.depth == 0 {
		evm.vmConfig.Tracer.CaptureStart(caller.Address(), address, true, code, gas, value)
	}
	start := time.Now()

	ret, err := run(evm, contract, nil)

//检查是否超过了最大代码大小
	maxCodeSizeExceeded := evm.ChainConfig().IsEIP158(evm.BlockNumber) && len(ret) > params.MaxCodeSize
//如果合同创建成功运行且未返回错误
//计算存储代码所需的气体。如果代码不能
//因气量不足而储存，设置错误并处理
//通过下面的错误检查条件。
	if err == nil && !maxCodeSizeExceeded {
		createDataGas := uint64(len(ret)) * params.CreateDataGas
		if contract.UseGas(createDataGas) {
			evm.StateDB.SetCode(address, ret)
		} else {
			err = ErrCodeStoreOutOfGas
		}
	}

//当EVM返回错误或设置创建代码时
//在上面，我们返回快照并消耗所有剩余的气体。另外
//当我们在宅基地时，这也算是代码存储气体错误。
	if maxCodeSizeExceeded || (err != nil && (evm.ChainConfig().IsHomestead(evm.BlockNumber) || err != ErrCodeStoreOutOfGas)) {
		evm.StateDB.RevertToSnapshot(snapshot)
		if err != errExecutionReverted {
			contract.UseGas(contract.Gas)
		}
	}
//如果合同代码大小超过最大值而错误仍然为空，则分配错误。
	if maxCodeSizeExceeded && err == nil {
		err = errMaxCodeSizeExceeded
	}
	if evm.vmConfig.Debug && evm.depth == 0 {
		evm.vmConfig.Tracer.CaptureEnd(ret, gas-contract.Gas, time.Since(start), err)
	}
	return ret, address, contract.Gas, err

}

//创建使用代码作为部署代码创建新合同。
func (evm *EVM) Create(caller ContractRef, code []byte, gas uint64, value *big.Int) (ret []byte, contractAddr common.Address, leftOverGas uint64, err error) {
	contractAddr = crypto.CreateAddress(caller.Address(), evm.StateDB.GetNonce(caller.Address()))
	return evm.create(caller, code, gas, value, contractAddr)
}

//Create2使用代码作为部署代码创建新合同。
//
//create2与create的区别是create2使用sha3（0xff++msg.sender++salt++sha3（init_code））[12:]
//而不是通常的发送者和nonce散列作为合同初始化的地址。
func (evm *EVM) Create2(caller ContractRef, code []byte, gas uint64, endowment *big.Int, salt *big.Int) (ret []byte, contractAddr common.Address, leftOverGas uint64, err error) {
	contractAddr = crypto.CreateAddress2(caller.Address(), common.BigToHash(salt), code)
	return evm.create(caller, code, gas, endowment, contractAddr)
}

//chainconfig返回环境的链配置
func (evm *EVM) ChainConfig() *params.ChainConfig { return evm.chainConfig }

