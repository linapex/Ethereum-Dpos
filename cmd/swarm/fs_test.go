
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:31</date>
//</624342605685067776>


//

package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/log"
	colorable "github.com/mattn/go-colorable"
)

func init() {
	log.PrintOrigins(true)
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(*loglevel), log.StreamHandler(colorable.NewColorableStderr(), log.TerminalFormat(true))))
}

type testFile struct {
	filePath string
	content  string
}

//
//
//
//
//
//
func TestCLISwarmFs(t *testing.T) {
	cluster := newTestCluster(t, 3)
	defer cluster.Shutdown()

//
	mountPoint, err := ioutil.TempDir("", "swarm-test")
	log.Debug("swarmfs cli test", "1st mount", mountPoint)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(mountPoint)

	handlingNode := cluster.Nodes[0]
	mhash := doUploadEmptyDir(t, handlingNode)
	log.Debug("swarmfs cli test: mounting first run", "ipc path", filepath.Join(handlingNode.Dir, handlingNode.IpcPath))

	mount := runSwarm(t, []string{
		"fs",
		"mount",
		"--ipcpath", filepath.Join(handlingNode.Dir, handlingNode.IpcPath),
		mhash,
		mountPoint,
	}...)
	mount.ExpectExit()

	filesToAssert := []*testFile{}

	dirPath, err := createDirInDir(mountPoint, "testSubDir")
	if err != nil {
		t.Fatal(err)
	}
	dirPath2, err := createDirInDir(dirPath, "AnotherTestSubDir")

	dummyContent := "somerandomtestcontentthatshouldbeasserted"
	dirs := []string{
		mountPoint,
		dirPath,
		dirPath2,
	}
	files := []string{"f1.tmp", "f2.tmp"}
	for _, d := range dirs {
		for _, entry := range files {
			tFile, err := createTestFileInPath(d, entry, dummyContent)
			if err != nil {
				t.Fatal(err)
			}
			filesToAssert = append(filesToAssert, tFile)
		}
	}
	if len(filesToAssert) != len(dirs)*len(files) {
		t.Fatalf("should have %d files to assert now, got %d", len(dirs)*len(files), len(filesToAssert))
	}
	hashRegexp := `[a-f\d]{64}`
	log.Debug("swarmfs cli test: unmounting first run...", "ipc path", filepath.Join(handlingNode.Dir, handlingNode.IpcPath))

	unmount := runSwarm(t, []string{
		"fs",
		"unmount",
		"--ipcpath", filepath.Join(handlingNode.Dir, handlingNode.IpcPath),
		mountPoint,
	}...)
	_, matches := unmount.ExpectRegexp(hashRegexp)
	unmount.ExpectExit()

	hash := matches[0]
	if hash == mhash {
		t.Fatal("this should not be equal")
	}
	log.Debug("swarmfs cli test: asserting no files in mount point")

//
	filesInDir, err := ioutil.ReadDir(mountPoint)
	if err != nil {
		t.Fatalf("had an error reading the directory: %v", err)
	}

	if len(filesInDir) != 0 {
		t.Fatal("there shouldn't be anything here")
	}

	secondMountPoint, err := ioutil.TempDir("", "swarm-test")
	log.Debug("swarmfs cli test", "2nd mount point at", secondMountPoint)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(secondMountPoint)

	log.Debug("swarmfs cli test: remounting at second mount point", "ipc path", filepath.Join(handlingNode.Dir, handlingNode.IpcPath))

//
	newMount := runSwarm(t, []string{
		"fs",
		"mount",
		"--ipcpath", filepath.Join(handlingNode.Dir, handlingNode.IpcPath),
hash, //最新散列
		secondMountPoint,
	}...)

	newMount.ExpectExit()
	time.Sleep(1 * time.Second)

	filesInDir, err = ioutil.ReadDir(secondMountPoint)
	if err != nil {
		t.Fatal(err)
	}

	if len(filesInDir) == 0 {
		t.Fatal("there should be something here")
	}

	log.Debug("swarmfs cli test: traversing file tree to see it matches previous mount")

	for _, file := range filesToAssert {
		file.filePath = strings.Replace(file.filePath, mountPoint, secondMountPoint, -1)
		fileBytes, err := ioutil.ReadFile(file.filePath)

		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(fileBytes, bytes.NewBufferString(file.content).Bytes()) {
			t.Fatal("this should be equal")
		}
	}

	log.Debug("swarmfs cli test: unmounting second run", "ipc path", filepath.Join(handlingNode.Dir, handlingNode.IpcPath))

	unmountSec := runSwarm(t, []string{
		"fs",
		"unmount",
		"--ipcpath", filepath.Join(handlingNode.Dir, handlingNode.IpcPath),
		secondMountPoint,
	}...)

	_, matches = unmountSec.ExpectRegexp(hashRegexp)
	unmountSec.ExpectExit()

	if matches[0] != hash {
		t.Fatal("these should be equal - no changes made")
	}
}

func doUploadEmptyDir(t *testing.T, node *testNode) string {
//创建tmp dir
	tmpDir, err := ioutil.TempDir("", "swarm-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	hashRegexp := `[a-f\d]{64}`

	flags := []string{
		"--bzzapi", node.URL,
		"--recursive",
		"up",
		tmpDir}

	log.Info("swarmfs cli test: uploading dir with 'swarm up'")
	up := runSwarm(t, flags...)
	_, matches := up.ExpectRegexp(hashRegexp)
	up.ExpectExit()
	hash := matches[0]
	log.Info("swarmfs cli test: dir uploaded", "hash", hash)
	return hash
}

func createDirInDir(createInDir string, dirToCreate string) (string, error) {
	fullpath := filepath.Join(createInDir, dirToCreate)
	err := os.MkdirAll(fullpath, 0777)
	if err != nil {
		return "", err
	}
	return fullpath, nil
}

func createTestFileInPath(dir, filename, content string) (*testFile, error) {
	tFile := &testFile{}
	filePath := filepath.Join(dir, filename)
	if file, err := os.Create(filePath); err == nil {
		tFile.content = content
		tFile.filePath = filePath

		_, err = io.WriteString(file, content)
		if err != nil {
			return nil, err
		}
		file.Close()
	}

	return tFile, nil
}

