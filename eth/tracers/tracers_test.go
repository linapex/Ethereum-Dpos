
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:39</date>
//</624342638492913664>


package tracers

import (
	"encoding/json"
	"io/ioutil"
	"math/big"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/tests"
)

//要生成新的calltracer测试，请将下面的maketest方法复制粘贴到
//一个geth控制台，用要导出的事务散列调用它。

/*
//maketest通过运行预状态重新组装和
//调用trace run，将收集到的所有信息组装到一个测试用例中。
var maketest=功能（tx，倒带）
  //从块、事务和预存数据生成Genesis块
  var block=eth.getblock（eth.getTransaction（tx）.blockHash）；
  var genesis=eth.getblock（block.parenthash）；

  删除genesis.gasused；
  删除genesis.logsbloom；
  删除genesis.parenthash；
  删除genesis.receiptsroot；
  删除Genesis.sha3uncles；
  删除genesis.size；
  删除genesis.transactions；
  删除genesis.transactionsroot；
  删除genesis.uncles；

  genesis.gaslimit=genesis.gaslimit.toString（）；
  genesis.number=genesis.number.toString（）；
  genesis.timestamp=genesis.timestamp.toString（）；

  genesis.alloc=debug.traceTransaction（tx，tracer:“prestatedtracer”，rewind:rewind）；
  for（genesis.alloc中的var键）
    genesis.alloc[key].nonce=genesis.alloc[key].nonce.toString（）；
  }
  genesis.config=admin.nodeinfo.protocols.eth.config；

  //生成调用跟踪并生成测试输入
  var result=debug.traceTransaction（tx，tracer:“calltracer”，rewind:rewind）；
  删除result.time；

  console.log（json.stringify（
    创世纪：创世纪，
    语境：{
      编号：block.number.toString（），
      困难：障碍。困难，
      timestamp:block.timestamp.toString（），
      gaslimit:block.gaslimit.toString（），
      矿工：block.miner，
    }
    输入：eth.getrawtransaction（tx）
    结果：
  }，NULL，2）；
}
**/


//CallTrace是CallTracer运行的结果。
type callTrace struct {
	Type    string          `json:"type"`
	From    common.Address  `json:"from"`
	To      common.Address  `json:"to"`
	Input   hexutil.Bytes   `json:"input"`
	Output  hexutil.Bytes   `json:"output"`
	Gas     *hexutil.Uint64 `json:"gas,omitempty"`
	GasUsed *hexutil.Uint64 `json:"gasUsed,omitempty"`
	Value   *hexutil.Big    `json:"value,omitempty"`
	Error   string          `json:"error,omitempty"`
	Calls   []callTrace     `json:"calls,omitempty"`
}

type callContext struct {
	Number     math.HexOrDecimal64   `json:"number"`
	Difficulty *math.HexOrDecimal256 `json:"difficulty"`
	Time       math.HexOrDecimal64   `json:"timestamp"`
	GasLimit   math.HexOrDecimal64   `json:"gasLimit"`
	Miner      common.Address        `json:"miner"`
}

//CallTracerTest定义一个测试来检查调用跟踪程序。
type callTracerTest struct {
	Genesis *core.Genesis `json:"genesis"`
	Context *callContext  `json:"context"`
	Input   string        `json:"input"`
	Result  *callTrace    `json:"result"`
}

//迭代跟踪测试工具中的所有输入输出数据集，并
//对它们运行javascript跟踪程序。
func TestCallTracer(t *testing.T) {
	files, err := ioutil.ReadDir("testdata")
	if err != nil {
		t.Fatalf("failed to retrieve tracer test suite: %v", err)
	}
	for _, file := range files {
		if !strings.HasPrefix(file.Name(), "call_tracer_") {
			continue
		}
file := file //捕获范围变量
		t.Run(camel(strings.TrimSuffix(strings.TrimPrefix(file.Name(), "call_tracer_"), ".json")), func(t *testing.T) {
			t.Parallel()

//找到呼叫追踪测试，从磁盘读取
			blob, err := ioutil.ReadFile(filepath.Join("testdata", file.Name()))
			if err != nil {
				t.Fatalf("failed to read testcase: %v", err)
			}
			test := new(callTracerTest)
			if err := json.Unmarshal(blob, test); err != nil {
				t.Fatalf("failed to parse testcase: %v", err)
			}
//使用给定的预状态配置区块链
			tx := new(types.Transaction)
			if err := rlp.DecodeBytes(common.FromHex(test.Input), tx); err != nil {
				t.Fatalf("failed to parse testcase input: %v", err)
			}
			signer := types.MakeSigner(test.Genesis.Config, new(big.Int).SetUint64(uint64(test.Context.Number)))
			origin, _ := signer.Sender(tx)

			context := vm.Context{
				CanTransfer: core.CanTransfer,
				Transfer:    core.Transfer,
				Origin:      origin,
				Coinbase:    test.Context.Miner,
				BlockNumber: new(big.Int).SetUint64(uint64(test.Context.Number)),
				Time:        new(big.Int).SetUint64(uint64(test.Context.Time)),
				Difficulty:  (*big.Int)(test.Context.Difficulty),
				GasLimit:    uint64(test.Context.GasLimit),
				GasPrice:    tx.GasPrice(),
			}
			statedb := tests.MakePreState(ethdb.NewMemDatabase(), test.Genesis.Alloc)

//创建跟踪程序、EVM环境并运行它
			tracer, err := New("callTracer")
			if err != nil {
				t.Fatalf("failed to create call tracer: %v", err)
			}
			evm := vm.NewEVM(context, statedb, test.Genesis.Config, vm.Config{Debug: true, Tracer: tracer})

			msg, err := tx.AsMessage(signer)
			if err != nil {
				t.Fatalf("failed to prepare transaction for tracing: %v", err)
			}
			st := core.NewStateTransition(evm, msg, new(core.GasPool).AddGas(tx.Gas()))
			if _, _, _, err = st.TransitionDb(); err != nil {
				t.Fatalf("failed to execute transaction: %v", err)
			}
//检索跟踪结果并与标准具进行比较
			res, err := tracer.GetResult()
			if err != nil {
				t.Fatalf("failed to retrieve trace result: %v", err)
			}
			ret := new(callTrace)
			if err := json.Unmarshal(res, ret); err != nil {
				t.Fatalf("failed to unmarshal trace result: %v", err)
			}
			if !reflect.DeepEqual(ret, test.Result) {
				t.Fatalf("trace mismatch: have %+v, want %+v", ret, test.Result)
			}
		})
	}
}

