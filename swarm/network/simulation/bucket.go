
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:47</date>
//</624342673108504576>


package simulation

import (
	"github.com/ethereum/go-ethereum/p2p/discover"
)

//BucketKey是模拟存储桶中的键应该使用的类型。
type BucketKey string

//nodeItem返回在servicefunc函数中为particualar节点设置的项。
func (s *Simulation) NodeItem(id discover.NodeID, key interface{}) (value interface{}, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.buckets[id]; !ok {
		return nil, false
	}
	return s.buckets[id].Load(key)
}

//setnodeitem设置与提供了nodeid的节点关联的新项。
//应使用存储桶来避免管理单独的模拟全局状态。
func (s *Simulation) SetNodeItem(id discover.NodeID, key interface{}, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.buckets[id].Store(key, value)
}

//nodes items返回在
//同样的BucketKey。
func (s *Simulation) NodesItems(key interface{}) (values map[discover.NodeID]interface{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := s.NodeIDs()
	values = make(map[discover.NodeID]interface{}, len(ids))
	for _, id := range ids {
		if _, ok := s.buckets[id]; !ok {
			continue
		}
		if v, ok := s.buckets[id].Load(key); ok {
			values[id] = v
		}
	}
	return values
}

//up nodes items从所有向上的节点返回具有相同bucketkey的项的映射。
func (s *Simulation) UpNodesItems(key interface{}) (values map[discover.NodeID]interface{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := s.UpNodeIDs()
	values = make(map[discover.NodeID]interface{})
	for _, id := range ids {
		if _, ok := s.buckets[id]; !ok {
			continue
		}
		if v, ok := s.buckets[id].Load(key); ok {
			values[id] = v
		}
	}
	return values
}

