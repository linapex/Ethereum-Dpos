
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:31</date>
//</624342606624591872>


package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/log"
	swarm "github.com/ethereum/go-ethereum/swarm/api/client"
	colorable "github.com/mattn/go-colorable"
)

var loglevel = flag.Int("loglevel", 3, "verbosity of logs")

func init() {
	log.PrintOrigins(true)
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(*loglevel), log.StreamHandler(colorable.NewColorableStderr(), log.TerminalFormat(true))))
}

//testcliswarmup测试运行“swarm up”生成的文件
//可通过HTTP API从所有节点获取
func TestCLISwarmUp(t *testing.T) {
	testCLISwarmUp(false, t)
}
func TestCLISwarmUpRecursive(t *testing.T) {
	testCLISwarmUpRecursive(false, t)
}

//testcliswarmupencypted测试运行“swarm encrypted up”生成的文件
//可通过HTTP API从所有节点获取
func TestCLISwarmUpEncrypted(t *testing.T) {
	testCLISwarmUp(true, t)
}
func TestCLISwarmUpEncryptedRecursive(t *testing.T) {
	testCLISwarmUpRecursive(true, t)
}

func testCLISwarmUp(toEncrypt bool, t *testing.T) {
	log.Info("starting 3 node cluster")
	cluster := newTestCluster(t, 3)
	defer cluster.Shutdown()

//创建tmp文件
	tmp, err := ioutil.TempFile("", "swarm-test")
	if err != nil {
		t.Fatal(err)
	}
	defer tmp.Close()
	defer os.Remove(tmp.Name())

//将数据写入文件
	data := "notsorandomdata"
	_, err = io.WriteString(tmp, data)
	if err != nil {
		t.Fatal(err)
	}

	hashRegexp := `[a-f\d]{64}`
	flags := []string{
		"--bzzapi", cluster.Nodes[0].URL,
		"up",
		tmp.Name()}
	if toEncrypt {
		hashRegexp = `[a-f\d]{128}`
		flags = []string{
			"--bzzapi", cluster.Nodes[0].URL,
			"up",
			"--encrypt",
			tmp.Name()}
	}
//用“swarm up”上传文件，并期望得到一个哈希值
	log.Info(fmt.Sprintf("uploading file with 'swarm up'"))
	up := runSwarm(t, flags...)
	_, matches := up.ExpectRegexp(hashRegexp)
	up.ExpectExit()
	hash := matches[0]
	log.Info("file uploaded", "hash", hash)

//从每个节点的HTTP API获取文件
	for _, node := range cluster.Nodes {
		log.Info("getting file from node", "node", node.Name)

		res, err := http.Get(node.URL + "/bzz:/" + hash)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		reply, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != 200 {
			t.Fatalf("expected HTTP status 200, got %s", res.Status)
		}
		if string(reply) != data {
			t.Fatalf("expected HTTP body %q, got %q", data, reply)
		}
		log.Debug("verifying uploaded file using `swarm down`")
//试着用“蜂群下降”来获取内容
		tmpDownload, err := ioutil.TempDir("", "swarm-test")
		tmpDownload = path.Join(tmpDownload, "tmpfile.tmp")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDownload)

		bzzLocator := "bzz:/" + hash
		flags = []string{
			"--bzzapi", cluster.Nodes[0].URL,
			"down",
			bzzLocator,
			tmpDownload,
		}

		down := runSwarm(t, flags...)
		down.ExpectExit()

		fi, err := os.Stat(tmpDownload)
		if err != nil {
			t.Fatalf("could not stat path: %v", err)
		}

		switch mode := fi.Mode(); {
		case mode.IsRegular():
			downloadedBytes, err := ioutil.ReadFile(tmpDownload)
			if err != nil {
				t.Fatalf("had an error reading the downloaded file: %v", err)
			}
			if !bytes.Equal(downloadedBytes, bytes.NewBufferString(data).Bytes()) {
				t.Fatalf("retrieved data and posted data not equal!")
			}

		default:
			t.Fatalf("expected to download regular file, got %s", fi.Mode())
		}
	}

	timeout := time.Duration(2 * time.Second)
	httpClient := http.Client{
		Timeout: timeout,
	}

//
	for _, node := range cluster.Nodes {
		_, err := httpClient.Get(node.URL + "/bzz:/1023e8bae0f70be7d7b5f74343088ba408a218254391490c85ae16278e230340")
//由于NetStore在请求时有60秒的超时时间，我们正在加快超时时间。
		if err != nil && !strings.Contains(err.Error(), "Client.Timeout exceeded while awaiting headers") {
			t.Fatal(err)
		}
//由于NetStore超时需要60秒，因此此选项被禁用。
//
//t.fatalf（“预期的HTTP状态404，得到%s”，res.status）
//
	}
}

func testCLISwarmUpRecursive(toEncrypt bool, t *testing.T) {
	fmt.Println("starting 3 node cluster")
	cluster := newTestCluster(t, 3)
	defer cluster.Shutdown()

	tmpUploadDir, err := ioutil.TempDir("", "swarm-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpUploadDir)
//
	data := "notsorandomdata"
	for _, path := range []string{"tmp1", "tmp2"} {
		if err := ioutil.WriteFile(filepath.Join(tmpUploadDir, path), bytes.NewBufferString(data).Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
	}

	hashRegexp := `[a-f\d]{64}`
	flags := []string{
		"--bzzapi", cluster.Nodes[0].URL,
		"--recursive",
		"up",
		tmpUploadDir}
	if toEncrypt {
		hashRegexp = `[a-f\d]{128}`
		flags = []string{
			"--bzzapi", cluster.Nodes[0].URL,
			"--recursive",
			"up",
			"--encrypt",
			tmpUploadDir}
	}
//用“swarm up”上传文件，并期望得到一个哈希值
	log.Info(fmt.Sprintf("uploading file with 'swarm up'"))
	up := runSwarm(t, flags...)
	_, matches := up.ExpectRegexp(hashRegexp)
	up.ExpectExit()
	hash := matches[0]
	log.Info("dir uploaded", "hash", hash)

//从每个节点的HTTP API获取文件
	for _, node := range cluster.Nodes {
		log.Info("getting file from node", "node", node.Name)
//试着用“蜂群下降”来获取内容
		tmpDownload, err := ioutil.TempDir("", "swarm-test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDownload)
		bzzLocator := "bzz:/" + hash
		flagss := []string{}
		flagss = []string{
			"--bzzapi", cluster.Nodes[0].URL,
			"down",
			"--recursive",
			bzzLocator,
			tmpDownload,
		}

		fmt.Println("downloading from swarm with recursive")
		down := runSwarm(t, flagss...)
		down.ExpectExit()

		files, err := ioutil.ReadDir(tmpDownload)
		for _, v := range files {
			fi, err := os.Stat(path.Join(tmpDownload, v.Name()))
			if err != nil {
				t.Fatalf("got an error: %v", err)
			}

			switch mode := fi.Mode(); {
			case mode.IsRegular():
				if file, err := swarm.Open(path.Join(tmpDownload, v.Name())); err != nil {
					t.Fatalf("encountered an error opening the file returned from the CLI: %v", err)
				} else {
					ff := make([]byte, len(data))
					io.ReadFull(file, ff)
					buf := bytes.NewBufferString(data)

					if !bytes.Equal(ff, buf.Bytes()) {
						t.Fatalf("retrieved data and posted data not equal!")
					}
				}
			default:
				t.Fatalf("this shouldnt happen")
			}
		}
		if err != nil {
			t.Fatalf("could not list files at: %v", files)
		}
	}
}

//
//默认路径和加密。
func TestCLISwarmUpDefaultPath(t *testing.T) {
	testCLISwarmUpDefaultPath(false, false, t)
	testCLISwarmUpDefaultPath(false, true, t)
	testCLISwarmUpDefaultPath(true, false, t)
	testCLISwarmUpDefaultPath(true, true, t)
}

func testCLISwarmUpDefaultPath(toEncrypt bool, absDefaultPath bool, t *testing.T) {
	cluster := newTestCluster(t, 1)
	defer cluster.Shutdown()

	tmp, err := ioutil.TempDir("", "swarm-defaultpath-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	err = ioutil.WriteFile(filepath.Join(tmp, "index.html"), []byte("<h1>Test</h1>"), 0666)
	if err != nil {
		t.Fatal(err)
	}
	err = ioutil.WriteFile(filepath.Join(tmp, "robots.txt"), []byte("Disallow: /"), 0666)
	if err != nil {
		t.Fatal(err)
	}

	defaultPath := "index.html"
	if absDefaultPath {
		defaultPath = filepath.Join(tmp, defaultPath)
	}

	args := []string{
		"--bzzapi",
		cluster.Nodes[0].URL,
		"--recursive",
		"--defaultpath",
		defaultPath,
		"up",
		tmp,
	}
	if toEncrypt {
		args = append(args, "--encrypt")
	}

	up := runSwarm(t, args...)
	hashRegexp := `[a-f\d]{64,128}`
	_, matches := up.ExpectRegexp(hashRegexp)
	up.ExpectExit()
	hash := matches[0]

	client := swarm.NewClient(cluster.Nodes[0].URL)

	m, isEncrypted, err := client.DownloadManifest(hash)
	if err != nil {
		t.Fatal(err)
	}

	if toEncrypt != isEncrypted {
		t.Error("downloaded manifest is not encrypted")
	}

	var found bool
	var entriesCount int
	for _, e := range m.Entries {
		entriesCount++
		if e.Path == "" {
			found = true
		}
	}

	if !found {
		t.Error("manifest default entry was not found")
	}

	if entriesCount != 3 {
		t.Errorf("manifest contains %v entries, expected %v", entriesCount, 3)
	}
}

