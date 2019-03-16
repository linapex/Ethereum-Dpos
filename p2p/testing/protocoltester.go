
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:45</date>
//</624342662048124928>


/*
p2p/测试包提供了一个单元测试方案来检查
协议消息与一个透视节点和多个虚拟对等点交换
透视测试节点运行一个节点。服务，虚拟对等运行一个模拟节点
可用于发送和接收消息的
**/


package testing

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/simulations"
	"github.com/ethereum/go-ethereum/p2p/simulations/adapters"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
)

//ProtocolTester是用于单元测试协议的测试环境
//消息交换。它使用P2P/仿真框架
type ProtocolTester struct {
	*ProtocolSession
	network *simulations.Network
}

//NewProtocolTester构造了一个新的ProtocolTester
//它将透视节点ID、虚拟对等数和
//P2P服务器在对等连接上调用的协议运行函数
func NewProtocolTester(t *testing.T, id discover.NodeID, n int, run func(*p2p.Peer, p2p.MsgReadWriter) error) *ProtocolTester {
	services := adapters.Services{
		"test": func(ctx *adapters.ServiceContext) (node.Service, error) {
			return &testNode{run}, nil
		},
		"mock": func(ctx *adapters.ServiceContext) (node.Service, error) {
			return newMockNode(), nil
		},
	}
	adapter := adapters.NewSimAdapter(services)
	net := simulations.NewNetwork(adapter, &simulations.NetworkConfig{})
	if _, err := net.NewNodeWithConfig(&adapters.NodeConfig{
		ID:              id,
		EnableMsgEvents: true,
		Services:        []string{"test"},
	}); err != nil {
		panic(err.Error())
	}
	if err := net.Start(id); err != nil {
		panic(err.Error())
	}

	node := net.GetNode(id).Node.(*adapters.SimNode)
	peers := make([]*adapters.NodeConfig, n)
	peerIDs := make([]discover.NodeID, n)
	for i := 0; i < n; i++ {
		peers[i] = adapters.RandomNodeConfig()
		peers[i].Services = []string{"mock"}
		peerIDs[i] = peers[i].ID
	}
	events := make(chan *p2p.PeerEvent, 1000)
	node.SubscribeEvents(events)
	ps := &ProtocolSession{
		Server:  node.Server(),
		IDs:     peerIDs,
		adapter: adapter,
		events:  events,
	}
	self := &ProtocolTester{
		ProtocolSession: ps,
		network:         net,
	}

	self.Connect(id, peers...)

	return self
}

//停止停止P2P服务器
func (t *ProtocolTester) Stop() error {
	t.Server.Stop()
	return nil
}

//Connect打开远程对等节点并使用
//P2P/模拟与内存网络适配器的网络连接
func (t *ProtocolTester) Connect(selfID discover.NodeID, peers ...*adapters.NodeConfig) {
	for _, peer := range peers {
		log.Trace(fmt.Sprintf("start node %v", peer.ID))
		if _, err := t.network.NewNodeWithConfig(peer); err != nil {
			panic(fmt.Sprintf("error starting peer %v: %v", peer.ID, err))
		}
		if err := t.network.Start(peer.ID); err != nil {
			panic(fmt.Sprintf("error starting peer %v: %v", peer.ID, err))
		}
		log.Trace(fmt.Sprintf("connect to %v", peer.ID))
		if err := t.network.Connect(selfID, peer.ID); err != nil {
			panic(fmt.Sprintf("error connecting to peer %v: %v", peer.ID, err))
		}
	}

}

//testnode包装协议运行函数并实现node.service
//界面
type testNode struct {
	run func(*p2p.Peer, p2p.MsgReadWriter) error
}

func (t *testNode) Protocols() []p2p.Protocol {
	return []p2p.Protocol{{
		Length: 100,
		Run:    t.run,
	}}
}

func (t *testNode) APIs() []rpc.API {
	return nil
}

func (t *testNode) Start(server *p2p.Server) error {
	return nil
}

func (t *testNode) Stop() error {
	return nil
}

//mocknode是一个没有实际运行协议的testnode
//公开通道，以便测试可以手动触发并预期
//信息
type mockNode struct {
	testNode

	trigger  chan *Trigger
	expect   chan []Expect
	err      chan error
	stop     chan struct{}
	stopOnce sync.Once
}

func newMockNode() *mockNode {
	mock := &mockNode{
		trigger: make(chan *Trigger),
		expect:  make(chan []Expect),
		err:     make(chan error),
		stop:    make(chan struct{}),
	}
	mock.testNode.run = mock.Run
	return mock
}

//运行是一个协议运行函数，它只循环等待测试
//指示它触发或期望来自对等端的消息
func (m *mockNode) Run(peer *p2p.Peer, rw p2p.MsgReadWriter) error {
	for {
		select {
		case trig := <-m.trigger:
			wmsg := Wrap(trig.Msg)
			m.err <- p2p.Send(rw, trig.Code, wmsg)
		case exps := <-m.expect:
			m.err <- expectMsgs(rw, exps)
		case <-m.stop:
			return nil
		}
	}
}

func (m *mockNode) Trigger(trig *Trigger) error {
	m.trigger <- trig
	return <-m.err
}

func (m *mockNode) Expect(exp ...Expect) error {
	m.expect <- exp
	return <-m.err
}

func (m *mockNode) Stop() error {
	m.stopOnce.Do(func() { close(m.stop) })
	return nil
}

func expectMsgs(rw p2p.MsgReadWriter, exps []Expect) error {
	matched := make([]bool, len(exps))
	for {
		msg, err := rw.ReadMsg()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		actualContent, err := ioutil.ReadAll(msg.Payload)
		if err != nil {
			return err
		}
		var found bool
		for i, exp := range exps {
			if exp.Code == msg.Code && bytes.Equal(actualContent, mustEncodeMsg(Wrap(exp.Msg))) {
				if matched[i] {
					return fmt.Errorf("message #%d received two times", i)
				}
				matched[i] = true
				found = true
				break
			}
		}
		if !found {
			expected := make([]string, 0)
			for i, exp := range exps {
				if matched[i] {
					continue
				}
				expected = append(expected, fmt.Sprintf("code %d payload %x", exp.Code, mustEncodeMsg(Wrap(exp.Msg))))
			}
			return fmt.Errorf("unexpected message code %d payload %x, expected %s", msg.Code, actualContent, strings.Join(expected, " or "))
		}
		done := true
		for _, m := range matched {
			if !m {
				done = false
				break
			}
		}
		if done {
			return nil
		}
	}
	for i, m := range matched {
		if !m {
			return fmt.Errorf("expected message #%d not received", i)
		}
	}
	return nil
}

//mustencodemsg使用rlp对消息进行编码。
//一旦出错，它就会惊慌失措。
func mustEncodeMsg(msg interface{}) []byte {
	contentEnc, err := rlp.EncodeToBytes(msg)
	if err != nil {
		panic("content encode error: " + err.Error())
	}
	return contentEnc
}

type WrappedMsg struct {
	Context []byte
	Size    uint32
	Payload []byte
}

func Wrap(msg interface{}) interface{} {
	data, _ := rlp.EncodeToBytes(msg)
	return &WrappedMsg{
		Size:    uint32(len(data)),
		Payload: data,
	}
}

