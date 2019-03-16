
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:40</date>
//</624342643219894272>


//package light实现可按需检索的状态和链对象
//对于以太坊Light客户端。
package les

import (
	"container/list"
	"sync"
	"time"
)

//requestdistributor实现一种机制，将请求分发到
//合适的同行，遵守流程控制规则，并在创建过程中对其进行优先级排序。
//订购（即使需要重新发送）。
type requestDistributor struct {
	reqQueue         *list.List
	lastReqOrder     uint64
	peers            map[distPeer]struct{}
	peerLock         sync.RWMutex
	stopChn, loopChn chan struct{}
	loopNextSent     bool
	lock             sync.Mutex
}

//distpeer是请求分发服务器的LES服务器对等接口。
//waitbefore返回发送请求前所需的等待时间
//具有给定的较高估计成本或估计的剩余相对缓冲
//发送此类请求后的值（在这种情况下，可以发送请求
//立即）。这些值中至少有一个始终为零。
type distPeer interface {
	waitBefore(uint64) (time.Duration, float64)
	canQueue() bool
	queueSend(f func())
}

//Distreq是分发服务器使用的请求抽象。它是建立在
//三个回调函数：
//-getcost返回将请求发送到给定对等机的开销的估计上限
//-cansend告诉服务器对等端是否适合服务请求
//-请求准备将请求发送给给定的对等方，并返回一个函数，
//实际发送。应保留请求顺序，但回调本身不应
//在发送之前阻止，因为其他对等方可能仍然能够在
//其中一个正在阻塞。相反，返回的函数被放入对等方的发送队列中。
type distReq struct {
	getCost func(distPeer) uint64
	canSend func(distPeer) bool
	request func(distPeer) func()

	reqOrder uint64
	sentChn  chan distPeer
	element  *list.Element
}

//new request distributor创建新的请求分发服务器
func newRequestDistributor(peers *peerSet, stopChn chan struct{}) *requestDistributor {
	d := &requestDistributor{
		reqQueue: list.New(),
		loopChn:  make(chan struct{}, 2),
		stopChn:  stopChn,
		peers:    make(map[distPeer]struct{}),
	}
	if peers != nil {
		peers.notify(d)
	}
	go d.loop()
	return d
}

//registerpeer实现peersetnotify
func (d *requestDistributor) registerPeer(p *peer) {
	d.peerLock.Lock()
	d.peers[p] = struct{}{}
	d.peerLock.Unlock()
}

//UnregisterPeer实现PeerSetNotify
func (d *requestDistributor) unregisterPeer(p *peer) {
	d.peerLock.Lock()
	delete(d.peers, p)
	d.peerLock.Unlock()
}

//RegisterTestPeer添加新的测试对等
func (d *requestDistributor) registerTestPeer(p distPeer) {
	d.peerLock.Lock()
	d.peers[p] = struct{}{}
	d.peerLock.Unlock()
}

//distmaxwait是最长的等待时间，在此之后需要进一步等待。
//根据服务器的新反馈重新计算时间
const distMaxWait = time.Millisecond * 10

//主事件循环
func (d *requestDistributor) loop() {
	for {
		select {
		case <-d.stopChn:
			d.lock.Lock()
			elem := d.reqQueue.Front()
			for elem != nil {
				close(elem.Value.(*distReq).sentChn)
				elem = elem.Next()
			}
			d.lock.Unlock()
			return
		case <-d.loopChn:
			d.lock.Lock()
			d.loopNextSent = false
		loop:
			for {
				peer, req, wait := d.nextRequest()
				if req != nil && wait == 0 {
chn := req.sentChn //保存sentchn，因为remove将其设置为nil
					d.remove(req)
					send := req.request(peer)
					if send != nil {
						peer.queueSend(send)
					}
					chn <- peer
					close(chn)
				} else {
					if wait == 0 {
//没有发送请求，没有等待；下一个
//排队的请求将唤醒循环
						break loop
					}
d.loopNextSent = true //已发送“下一个”信号，在收到该信号之前不要再发送另一个信号。
					if wait > distMaxWait {
//传入的请求答复可能会缩短等待时间，如果时间太长，请定期重新计算
						wait = distMaxWait
					}
					go func() {
						time.Sleep(wait)
						d.loopChn <- struct{}{}
					}()
					break loop
				}
			}
			d.lock.Unlock()
		}
	}
}

//SelectePeerItem表示要按WeightedRandomSelect为请求选择的对等机
type selectPeerItem struct {
	peer   distPeer
	req    *distReq
	weight int64
}

//权重实现WRSitem接口
func (sp selectPeerItem) Weight() int64 {
	return sp.weight
}

//NextRequest返回来自任何对等机的下一个可能的请求，以及
//关联的对等机和必要的等待时间
func (d *requestDistributor) nextRequest() (distPeer, *distReq, time.Duration) {
	checkedPeers := make(map[distPeer]struct{})
	elem := d.reqQueue.Front()
	var (
		bestPeer distPeer
		bestReq  *distReq
		bestWait time.Duration
		sel      *weightedRandomSelect
	)

	d.peerLock.RLock()
	defer d.peerLock.RUnlock()

	for (len(d.peers) > 0 || elem == d.reqQueue.Front()) && elem != nil {
		req := elem.Value.(*distReq)
		canSend := false
		for peer := range d.peers {
			if _, ok := checkedPeers[peer]; !ok && peer.canQueue() && req.canSend(peer) {
				canSend = true
				cost := req.getCost(peer)
				wait, bufRemain := peer.waitBefore(cost)
				if wait == 0 {
					if sel == nil {
						sel = newWeightedRandomSelect()
					}
					sel.update(selectPeerItem{peer: peer, req: req, weight: int64(bufRemain*1000000) + 1})
				} else {
					if bestReq == nil || wait < bestWait {
						bestPeer = peer
						bestReq = req
						bestWait = wait
					}
				}
				checkedPeers[peer] = struct{}{}
			}
		}
		next := elem.Next()
		if !canSend && elem == d.reqQueue.Front() {
			close(req.sentChn)
			d.remove(req)
		}
		elem = next
	}

	if sel != nil {
		c := sel.choose().(selectPeerItem)
		return c.peer, c.req, 0
	}
	return bestPeer, bestReq, bestWait
}

//队列向分发队列添加请求，返回一个通道，其中
//发送请求后即发送接收对等（返回请求回调）。
//如果请求被取消或在没有合适的对等方的情况下超时，则通道为
//关闭而不向其发送任何对等引用。
func (d *requestDistributor) queue(r *distReq) chan distPeer {
	d.lock.Lock()
	defer d.lock.Unlock()

	if r.reqOrder == 0 {
		d.lastReqOrder++
		r.reqOrder = d.lastReqOrder
	}

	back := d.reqQueue.Back()
	if back == nil || r.reqOrder > back.Value.(*distReq).reqOrder {
		r.element = d.reqQueue.PushBack(r)
	} else {
		before := d.reqQueue.Front()
		for before.Value.(*distReq).reqOrder < r.reqOrder {
			before = before.Next()
		}
		r.element = d.reqQueue.InsertBefore(r, before)
	}

	if !d.loopNextSent {
		d.loopNextSent = true
		d.loopChn <- struct{}{}
	}

	r.sentChn = make(chan distPeer, 1)
	return r.sentChn
}

//如果尚未发送请求，则取消将其从队列中删除（返回
//如果已发送，则为false）。保证回调函数
//取消返回后将不调用。
func (d *requestDistributor) cancel(r *distReq) bool {
	d.lock.Lock()
	defer d.lock.Unlock()

	if r.sentChn == nil {
		return false
	}

	close(r.sentChn)
	d.remove(r)
	return true
}

//删除从队列中删除请求
func (d *requestDistributor) remove(r *distReq) {
	r.sentChn = nil
	if r.element != nil {
		d.reqQueue.Remove(r.element)
		r.element = nil
	}
}

