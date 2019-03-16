
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:49</date>
//</624342682021400576>

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
//
//

package db

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/swarm/storage/mock/test"
)

//
//
func TestDBStore(t *testing.T) {
	dir, err := ioutil.TempDir("", "mock_"+t.Name())
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	store, err := NewGlobalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	test.MockStore(t, store, 100)
}

//
//
func TestImportExport(t *testing.T) {
	dir1, err := ioutil.TempDir("", "mock_"+t.Name()+"_exporter")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir1)

	store1, err := NewGlobalStore(dir1)
	if err != nil {
		t.Fatal(err)
	}
	defer store1.Close()

	dir2, err := ioutil.TempDir("", "mock_"+t.Name()+"_importer")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir2)

	store2, err := NewGlobalStore(dir2)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()

	test.ImportExport(t, store1, store2, 100)
}

