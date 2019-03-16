
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:47</date>
//</624342672856846336>


package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/p2p/protocols"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/swarm/log"
	"github.com/ethereum/go-ethereum/swarm/state"
)

const (
	DefaultNetworkID = 3
//protocolmaxmsgsize允许的最大消息大小
	ProtocolMaxMsgSize = 10 * 1024 * 1024
//等待超时
	bzzHandshakeTimeout = 3000 * time.Millisecond
)

//bzzspec是通用群握手的规范
var BzzSpec = &protocols.Spec{
	Name:       "bzz",
	Version:    6,
	MaxMsgSize: 10 * 1024 * 1024,
	Messages: []interface{}{
		HandshakeMsg{},
	},
}

//discovery spec是bzz discovery子协议的规范
var DiscoverySpec = &protocols.Spec{
	Name:       "hive",
	Version:    5,
	MaxMsgSize: 10 * 1024 * 1024,
	Messages: []interface{}{
		peersMsg{},
		subPeersMsg{},
	},
}

//对等池所需的addr接口
type Addr interface {
	OverlayPeer
	Over() []byte
	Under() []byte
	String() string
	Update(OverlayAddr) OverlayAddr
}

//对等接口表示实时对等连接
type Peer interface {
Addr                   //对等机的地址
Conn                   //实时连接（protocols.peer）
LastActive() time.Time //上次激活时间
}

//conn接口表示活动对等连接
type Conn interface {
ID() discover.NodeID                                                                  //唯一标识对等池节点的键
Handshake(context.Context, interface{}, func(interface{}) error) (interface{}, error) //可以发送消息
Send(context.Context, interface{}) error                                              //
Drop(error)                                                                           //
Run(func(context.Context, interface{}) error) error                                   //
	Off() OverlayAddr
}

//bzzconfig捕获配置单元使用的配置参数
type BzzConfig struct {
OverlayAddr  []byte //覆盖网络的基址
UnderlayAddr []byte //节点的参考底图地址
	HiveParams   *HiveParams
	NetworkID    uint64
	LightNode    bool
}

//bzz是swarm协议包
type Bzz struct {
	*Hive
	NetworkID    uint64
	LightNode    bool
	localAddr    *BzzAddr
	mtx          sync.Mutex
	handshakes   map[discover.NodeID]*HandshakeMsg
	streamerSpec *protocols.Spec
	streamerRun  func(*BzzPeer) error
}

//Newzz是Swarm协议的构造者
//争论
//*BZZ配置
//*覆盖驱动程序
//*对等存储
func NewBzz(config *BzzConfig, kad Overlay, store state.Store, streamerSpec *protocols.Spec, streamerRun func(*BzzPeer) error) *Bzz {
	return &Bzz{
		Hive:         NewHive(config.HiveParams, kad, store),
		NetworkID:    config.NetworkID,
		LightNode:    config.LightNode,
		localAddr:    &BzzAddr{config.OverlayAddr, config.UnderlayAddr},
		handshakes:   make(map[discover.NodeID]*HandshakeMsg),
		streamerRun:  streamerRun,
		streamerSpec: streamerSpec,
	}
}

//updateLocalAddr更新正在运行的节点的参考底图地址
func (b *Bzz) UpdateLocalAddr(byteaddr []byte) *BzzAddr {
	b.localAddr = b.localAddr.Update(&BzzAddr{
		UAddr: byteaddr,
		OAddr: b.localAddr.OAddr,
	}).(*BzzAddr)
	return b.localAddr
}

//nodeinfo返回节点的覆盖地址
func (b *Bzz) NodeInfo() interface{} {
	return b.localAddr.Address()
}

//协议返回Swarm提供的协议
//bzz实现node.service接口
//*握手/蜂窝
//＊发现
func (b *Bzz) Protocols() []p2p.Protocol {
	protocol := []p2p.Protocol{
		{
			Name:     BzzSpec.Name,
			Version:  BzzSpec.Version,
			Length:   BzzSpec.Length(),
			Run:      b.runBzz,
			NodeInfo: b.NodeInfo,
		},
		{
			Name:     DiscoverySpec.Name,
			Version:  DiscoverySpec.Version,
			Length:   DiscoverySpec.Length(),
			Run:      b.RunProtocol(DiscoverySpec, b.Hive.Run),
			NodeInfo: b.Hive.NodeInfo,
			PeerInfo: b.Hive.PeerInfo,
		},
	}
	if b.streamerSpec != nil && b.streamerRun != nil {
		protocol = append(protocol, p2p.Protocol{
			Name:    b.streamerSpec.Name,
			Version: b.streamerSpec.Version,
			Length:  b.streamerSpec.Length(),
			Run:     b.RunProtocol(b.streamerSpec, b.streamerRun),
		})
	}
	return protocol
}

//API返回BZZ提供的API
//*蜂箱
//bzz实现node.service接口
func (b *Bzz) APIs() []rpc.API {
	return []rpc.API{{
		Namespace: "hive",
		Version:   "3.0",
		Service:   b.Hive,
	}}
}

//runprotocol是swarm子协议的包装器
//返回可分配给p2p.protocol run字段的p2p协议运行函数。
//争论：
//*P2P协议规范
//*以bzzpeer为参数运行函数
//此运行函数用于在协议会话期间阻塞
//返回时，会话终止，对等端断开连接。
//协议等待BZZ握手被协商
//bzzpeer上的覆盖地址是通过远程握手设置的。
func (b *Bzz) RunProtocol(spec *protocols.Spec, run func(*BzzPeer) error) func(*p2p.Peer, p2p.MsgReadWriter) error {
	return func(p *p2p.Peer, rw p2p.MsgReadWriter) error {
//等待BZZ协议执行握手
		handshake, _ := b.GetHandshake(p.ID())
		defer b.removeHandshake(p.ID())
		select {
		case <-handshake.done:
		case <-time.After(bzzHandshakeTimeout):
			return fmt.Errorf("%08x: %s protocol timeout waiting for handshake on %08x", b.BaseAddr()[:4], spec.Name, p.ID().Bytes()[:4])
		}
		if handshake.err != nil {
			return fmt.Errorf("%08x: %s protocol closed: %v", b.BaseAddr()[:4], spec.Name, handshake.err)
		}
//握手成功，因此构造bzzpeer并运行协议
		peer := &BzzPeer{
			Peer:       protocols.NewPeer(p, rw, spec),
			localAddr:  b.localAddr,
			BzzAddr:    handshake.peerAddr,
			lastActive: time.Now(),
			LightNode:  handshake.LightNode,
		}

		log.Debug("peer created", "addr", handshake.peerAddr.String())

		return run(peer)
	}
}

//performhandshake实现BZZ握手的协商
//在群子协议中共享
func (b *Bzz) performHandshake(p *protocols.Peer, handshake *HandshakeMsg) error {
	ctx, cancel := context.WithTimeout(context.Background(), bzzHandshakeTimeout)
	defer func() {
		close(handshake.done)
		cancel()
	}()
	rsh, err := p.Handshake(ctx, handshake, b.checkHandshake)
	if err != nil {
		handshake.err = err
		return err
	}
	handshake.peerAddr = rsh.(*HandshakeMsg).Addr
	handshake.LightNode = rsh.(*HandshakeMsg).LightNode
	return nil
}

//run bzz是bzz基本协议的p2p协议运行函数
//与BZZ握手谈判
func (b *Bzz) runBzz(p *p2p.Peer, rw p2p.MsgReadWriter) error {
	handshake, _ := b.GetHandshake(p.ID())
	if !<-handshake.init {
		return fmt.Errorf("%08x: bzz already started on peer %08x", b.localAddr.Over()[:4], ToOverlayAddr(p.ID().Bytes())[:4])
	}
	close(handshake.init)
	defer b.removeHandshake(p.ID())
	peer := protocols.NewPeer(p, rw, BzzSpec)
	err := b.performHandshake(peer, handshake)
	if err != nil {
		log.Warn(fmt.Sprintf("%08x: handshake failed with remote peer %08x: %v", b.localAddr.Over()[:4], ToOverlayAddr(p.ID().Bytes())[:4], err))

		return err
	}
//如果我们再握手就失败了
	msg, err := rw.ReadMsg()
	if err != nil {
		return err
	}
	msg.Discard()
	return errors.New("received multiple handshakes")
}

//bzz peer是协议的bzz协议视图。peer（本身是p2p.peer的扩展）
//实现对等接口和所有接口对等实现：addr、overlaypeer
type BzzPeer struct {
*protocols.Peer           //表示联机对等机的连接
localAddr       *BzzAddr  //本地对等地址
*BzzAddr                  //远程地址->实现addr interface=protocols.peer
lastActive      time.Time //当互斥锁释放时，时间会更新。
	LightNode       bool
}

func NewBzzTestPeer(p *protocols.Peer, addr *BzzAddr) *BzzPeer {
	return &BzzPeer{
		Peer:      p,
		localAddr: addr,
		BzzAddr:   NewAddrFromNodeID(p.ID()),
	}
}

//off返回脱机持久性的覆盖对等记录
func (p *BzzPeer) Off() OverlayAddr {
	return p.BzzAddr
}

//LastActive返回上次激活对等机的时间
func (p *BzzPeer) LastActive() time.Time {
	return p.lastActive
}

/*
 握手

*版本：协议的8字节整数版本
*networkid:8字节整数网络标识符
*地址：节点公布的地址，包括底层和覆盖连接。
**/

type HandshakeMsg struct {
	Version   uint64
	NetworkID uint64
	Addr      *BzzAddr
	LightNode bool

//PeerAddr是对等握手中接收到的地址
	peerAddr *BzzAddr

	init chan bool
	done chan struct{}
	err  error
}

//字符串漂亮地打印了握手
func (bh *HandshakeMsg) String() string {
	return fmt.Sprintf("Handshake: Version: %v, NetworkID: %v, Addr: %v, LightNode: %v, peerAddr: %v", bh.Version, bh.NetworkID, bh.Addr, bh.LightNode, bh.peerAddr)
}

//执行启动握手并验证远程握手消息
func (b *Bzz) checkHandshake(hs interface{}) error {
	rhs := hs.(*HandshakeMsg)
	if rhs.NetworkID != b.NetworkID {
		return fmt.Errorf("network id mismatch %d (!= %d)", rhs.NetworkID, b.NetworkID)
	}
	if rhs.Version != uint64(BzzSpec.Version) {
		return fmt.Errorf("version mismatch %d (!= %d)", rhs.Version, BzzSpec.Version)
	}
	return nil
}

//removehandshake删除具有peerID的对等方的握手
//来自BZZ握手商店
func (b *Bzz) removeHandshake(peerID discover.NodeID) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	delete(b.handshakes, peerID)
}

//gethandshake返回peerid远程对等机发送的bzz handshake
func (b *Bzz) GetHandshake(peerID discover.NodeID) (*HandshakeMsg, bool) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	handshake, found := b.handshakes[peerID]
	if !found {
		handshake = &HandshakeMsg{
			Version:   uint64(BzzSpec.Version),
			NetworkID: b.NetworkID,
			Addr:      b.localAddr,
			LightNode: b.LightNode,
			init:      make(chan bool, 1),
			done:      make(chan struct{}),
		}
//首次为远程对等机创建handhsake时
//它是用init初始化的
		handshake.init <- true
		b.handshakes[peerID] = handshake
	}

	return handshake, found
}

//bzzaddr实现peeraddr接口
type BzzAddr struct {
	OAddr []byte
	UAddr []byte
}

//地址实现覆盖中要使用的覆盖对等接口
func (a *BzzAddr) Address() []byte {
	return a.OAddr
}

//over返回覆盖地址
func (a *BzzAddr) Over() []byte {
	return a.OAddr
}

//在下面返回参考底图地址
func (a *BzzAddr) Under() []byte {
	return a.UAddr
}

//id返回参考底图enode地址中的nodeid
func (a *BzzAddr) ID() discover.NodeID {
	return discover.MustParseNode(string(a.UAddr)).ID
}

//更新更新更新对等记录的底层地址
func (a *BzzAddr) Update(na OverlayAddr) OverlayAddr {
	return &BzzAddr{a.OAddr, na.(Addr).Under()}
}

//字符串漂亮地打印地址
func (a *BzzAddr) String() string {
	return fmt.Sprintf("%x <%s>", a.OAddr, a.UAddr)
}

//randomaddr是从公钥生成地址的实用方法
func RandomAddr() *BzzAddr {
	key, err := crypto.GenerateKey()
	if err != nil {
		panic("unable to generate key")
	}
	pubkey := crypto.FromECDSAPub(&key.PublicKey)
	var id discover.NodeID
	copy(id[:], pubkey[1:])
	return NewAddrFromNodeID(id)
}

//newnodeidfromaddr将底层地址转换为adapters.nodeid
func NewNodeIDFromAddr(addr Addr) discover.NodeID {
	log.Info(fmt.Sprintf("uaddr=%s", string(addr.Under())))
	node := discover.MustParseNode(string(addr.Under()))
	return node.ID
}

//newAddrFromNodeID从discover.nodeID构造BzzAddr
//覆盖地址是作为nodeid的散列派生的。
func NewAddrFromNodeID(id discover.NodeID) *BzzAddr {
	return &BzzAddr{
		OAddr: ToOverlayAddr(id.Bytes()),
		UAddr: []byte(discover.NewNode(id, net.IP{127, 0, 0, 1}, 30303, 30303).String()),
	}
}

//newaddrFromNodeAndPort从discover.nodeid和端口uint16构造bzzaddr
//覆盖地址是作为nodeid的散列派生的。
func NewAddrFromNodeIDAndPort(id discover.NodeID, host net.IP, port uint16) *BzzAddr {
	return &BzzAddr{
		OAddr: ToOverlayAddr(id.Bytes()),
		UAddr: []byte(discover.NewNode(id, host, port, port).String()),
	}
}

//ToOverlayAddr从字节片创建覆盖地址
func ToOverlayAddr(id []byte) []byte {
	return crypto.Keccak256(id)
}

