
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:46</date>
//</624342666921906176>


package core

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func hexAddr(a string) common.Address { return common.BytesToAddress(common.FromHex(a)) }
func mixAddr(a string) (*common.MixedcaseAddress, error) {
	return common.NewMixedcaseAddressFromString(a)
}
func toHexBig(h string) hexutil.Big {
	b := big.NewInt(0).SetBytes(common.FromHex(h))
	return hexutil.Big(*b)
}
func toHexUint(h string) hexutil.Uint64 {
	b := big.NewInt(0).SetBytes(common.FromHex(h))
	return hexutil.Uint64(b.Uint64())
}
func dummyTxArgs(t txtestcase) *SendTxArgs {
	to, _ := mixAddr(t.to)
	from, _ := mixAddr(t.from)
	n := toHexUint(t.n)
	gas := toHexUint(t.g)
	gasPrice := toHexBig(t.gp)
	value := toHexBig(t.value)
	var (
		data, input *hexutil.Bytes
	)
	if t.d != "" {
		a := hexutil.Bytes(common.FromHex(t.d))
		data = &a
	}
	if t.i != "" {
		a := hexutil.Bytes(common.FromHex(t.i))
		input = &a

	}
	return &SendTxArgs{
		From:     *from,
		To:       to,
		Value:    value,
		Nonce:    n,
		GasPrice: gasPrice,
		Gas:      gas,
		Data:     data,
		Input:    input,
	}
}

type txtestcase struct {
	from, to, n, g, gp, value, d, i string
	expectErr                       bool
	numMessages                     int
}

func TestValidator(t *testing.T) {
	var (
//使用空数据库，对ABI特定的东西还有其他测试
		db, _ = NewEmptyAbiDB()
		v     = NewValidator(db)
	)
	testcases := []txtestcase{
//校验和无效
		{from: "000000000000000000000000000000000000dead", to: "000000000000000000000000000000000000dead",
			n: "0x01", g: "0x20", gp: "0x40", value: "0x01", numMessages: 1},
//有效的0x0000000000000000000000000000000标题
		{from: "000000000000000000000000000000000000dead", to: "0x000000000000000000000000000000000000dEaD",
			n: "0x01", g: "0x20", gp: "0x40", value: "0x01", numMessages: 0},
//输入和数据冲突
		{from: "000000000000000000000000000000000000dead", to: "0x000000000000000000000000000000000000dEaD",
			n: "0x01", g: "0x20", gp: "0x40", value: "0x01", d: "0x01", i: "0x02", expectErr: true},
//无法分析数据
		{from: "000000000000000000000000000000000000dead", to: "0x000000000000000000000000000000000000dEaD",
			n: "0x01", g: "0x20", gp: "0x40", value: "0x01", d: "0x0102", numMessages: 1},
//无法分析（输入时）数据
		{from: "000000000000000000000000000000000000dead", to: "0x000000000000000000000000000000000000dEaD",
			n: "0x01", g: "0x20", gp: "0x40", value: "0x01", i: "0x0102", numMessages: 1},
//发送到0
		{from: "000000000000000000000000000000000000dead", to: "0x0000000000000000000000000000000000000000",
			n: "0x01", g: "0x20", gp: "0x40", value: "0x01", numMessages: 1},
//创建空合同（无值）
		{from: "000000000000000000000000000000000000dead", to: "",
			n: "0x01", g: "0x20", gp: "0x40", value: "0x00", numMessages: 1},
//创建空合同（带值）
		{from: "000000000000000000000000000000000000dead", to: "",
			n: "0x01", g: "0x20", gp: "0x40", value: "0x01", expectErr: true},
//用于创建的小负载
		{from: "000000000000000000000000000000000000dead", to: "",
			n: "0x01", g: "0x20", gp: "0x40", value: "0x01", d: "0x01", numMessages: 1},
	}
	for i, test := range testcases {
		msgs, err := v.ValidateTransaction(dummyTxArgs(test), nil)
		if err == nil && test.expectErr {
			t.Errorf("Test %d, expected error", i)
			for _, msg := range msgs.Messages {
				fmt.Printf("* %s: %s\n", msg.Typ, msg.Message)
			}
		}
		if err != nil && !test.expectErr {
			t.Errorf("Test %d, unexpected error: %v", i, err)
		}
		if err == nil {
			got := len(msgs.Messages)
			if got != test.numMessages {
				for _, msg := range msgs.Messages {
					fmt.Printf("* %s: %s\n", msg.Typ, msg.Message)
				}
				t.Errorf("Test %d, expected %d messages, got %d", i, test.numMessages, got)
			} else {
//调试打印输出，稍后删除
				for _, msg := range msgs.Messages {
					fmt.Printf("* [%d] %s: %s\n", i, msg.Typ, msg.Message)
				}
				fmt.Println()
			}
		}
	}
}

