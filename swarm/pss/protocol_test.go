
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:48</date>
//</624342678171029504>

//
//
//
//
//
//
//
//
//
//
//
//
//
//
//

package pss

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	"github.com/ethereum/go-ethereum/swarm/log"
)

type protoCtrl struct {
	C        chan bool
	protocol *Protocol
	run      func(*p2p.Peer, p2p.MsgReadWriter) error
}

//
func TestProtocol(t *testing.T) {
	t.Run("32", testProtocol)
	t.Run("8", testProtocol)
	t.Run("0", testProtocol)
}

func testProtocol(t *testing.T) {

//
	var addrsize int64
	paramstring := strings.Split(t.Name(), "/")
	addrsize, _ = strconv.ParseInt(paramstring[1], 10, 0)
	log.Info("protocol test", "addrsize", addrsize)

	topic := PingTopic.String()

	clients, err := setupNetwork(2, false)
	if err != nil {
		t.Fatal(err)
	}
	var loaddrhex string
	err = clients[0].Call(&loaddrhex, "pss_baseAddr")
	if err != nil {
		t.Fatalf("rpc get node 1 baseaddr fail: %v", err)
	}
	loaddrhex = loaddrhex[:2+(addrsize*2)]
	var roaddrhex string
	err = clients[1].Call(&roaddrhex, "pss_baseAddr")
	if err != nil {
		t.Fatalf("rpc get node 2 baseaddr fail: %v", err)
	}
	roaddrhex = roaddrhex[:2+(addrsize*2)]
	lnodeinfo := &p2p.NodeInfo{}
	err = clients[0].Call(&lnodeinfo, "admin_nodeInfo")
	if err != nil {
		t.Fatalf("rpc nodeinfo node 11 fail: %v", err)
	}

	var lpubkey string
	err = clients[0].Call(&lpubkey, "pss_getPublicKey")
	if err != nil {
		t.Fatalf("rpc get node 1 pubkey fail: %v", err)
	}
	var rpubkey string
	err = clients[1].Call(&rpubkey, "pss_getPublicKey")
	if err != nil {
		t.Fatalf("rpc get node 2 pubkey fail: %v", err)
	}

time.Sleep(time.Millisecond * 1000) //

	lmsgC := make(chan APIMsg)
	lctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	lsub, err := clients[0].Subscribe(lctx, "pss", lmsgC, "receive", topic)
	defer lsub.Unsubscribe()
	rmsgC := make(chan APIMsg)
	rctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	rsub, err := clients[1].Subscribe(rctx, "pss", rmsgC, "receive", topic)
	defer rsub.Unsubscribe()

//
	err = clients[0].Call(nil, "pss_setPeerPublicKey", rpubkey, topic, roaddrhex)
	if err != nil {
		t.Fatal(err)
	}
	err = clients[1].Call(nil, "pss_setPeerPublicKey", lpubkey, topic, loaddrhex)
	if err != nil {
		t.Fatal(err)
	}

//
nid, _ := discover.HexID("0x00") //
	p := p2p.NewPeer(nid, fmt.Sprintf("%x", common.FromHex(loaddrhex)), []p2p.Cap{})
	_, err = pssprotocols[lnodeinfo.ID].protocol.AddPeer(p, PingTopic, true, rpubkey)
	if err != nil {
		t.Fatal(err)
	}

//
	pssprotocols[lnodeinfo.ID].C <- false
	select {
	case <-lmsgC:
		log.Debug("lnode ok")
	case cerr := <-lctx.Done():
		t.Fatalf("test message timed out: %v", cerr)
	}
	select {
	case <-rmsgC:
		log.Debug("rnode ok")
	case cerr := <-lctx.Done():
		t.Fatalf("test message timed out: %v", cerr)
	}

//
	pssprotocols[lnodeinfo.ID].C <- false
	select {
	case <-lmsgC:
		log.Debug("lnode ok")
	case cerr := <-lctx.Done():
		t.Fatalf("test message timed out: %v", cerr)
	}
	select {
	case <-rmsgC:
		log.Debug("rnode ok")
	case cerr := <-lctx.Done():
		t.Fatalf("test message timed out: %v", cerr)
	}
	rw := pssprotocols[lnodeinfo.ID].protocol.pubKeyRWPool[rpubkey]
	pssprotocols[lnodeinfo.ID].protocol.RemovePeer(true, rpubkey)
	if err := rw.WriteMsg(p2p.Msg{
		Size:    3,
		Payload: bytes.NewReader([]byte("foo")),
	}); err == nil {
		t.Fatalf("expected error on write")
	}
}

