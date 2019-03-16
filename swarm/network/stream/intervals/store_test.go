
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:48</date>
//</624342675541200896>

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

package intervals

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/swarm/state"
)

var ErrNotFound = errors.New("not found")

//
func TestInmemoryStore(t *testing.T) {
	testStore(t, state.NewInmemoryStore())
}

//
func testStore(t *testing.T, s state.Store) {
	key1 := "key1"
	i1 := NewIntervals(0)
	i1.Add(10, 20)
	if err := s.Put(key1, i1); err != nil {
		t.Fatal(err)
	}
	i := &Intervals{}
	err := s.Get(key1, i)
	if err != nil {
		t.Fatal(err)
	}
	if i.String() != i1.String() {
		t.Errorf("expected interval %s, got %s", i1, i)
	}

	key2 := "key2"
	i2 := NewIntervals(0)
	i2.Add(10, 20)
	if err := s.Put(key2, i2); err != nil {
		t.Fatal(err)
	}
	err = s.Get(key2, i)
	if err != nil {
		t.Fatal(err)
	}
	if i.String() != i2.String() {
		t.Errorf("expected interval %s, got %s", i2, i)
	}

	if err := s.Delete(key1); err != nil {
		t.Fatal(err)
	}
	if err := s.Get(key1, i); err != state.ErrNotFound {
		t.Errorf("expected error %v, got %s", state.ErrNotFound, err)
	}
	if err := s.Get(key2, i); err != nil {
		t.Errorf("expected error %v, got %s", nil, err)
	}

	if err := s.Delete(key2); err != nil {
		t.Fatal(err)
	}
	if err := s.Get(key2, i); err != state.ErrNotFound {
		t.Errorf("expected error %v, got %s", state.ErrNotFound, err)
	}
}

