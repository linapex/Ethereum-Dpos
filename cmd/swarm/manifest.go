
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:31</date>
//</624342605999640576>


//
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/swarm/api"
	swarm "github.com/ethereum/go-ethereum/swarm/api/client"
	"gopkg.in/urfave/cli.v1"
)

//
//最后一个参数new entry hash必须是清单的hash
//
//
func manifestAdd(ctx *cli.Context) {
	args := ctx.Args()
	if len(args) != 3 {
		utils.Fatalf("Need exactly three arguments <MHASH> <path> <HASH>")
	}

	var (
		mhash = args[0]
		path  = args[1]
		hash  = args[2]
	)

	bzzapi := strings.TrimRight(ctx.GlobalString(SwarmApiFlag.Name), "/")
	client := swarm.NewClient(bzzapi)

	m, _, err := client.DownloadManifest(hash)
	if err != nil {
		utils.Fatalf("Error downloading manifest to add: %v", err)
	}
	l := len(m.Entries)
	if l == 0 {
		utils.Fatalf("No entries in manifest %s", hash)
	} else if l > 1 {
		utils.Fatalf("Too many entries in manifest %s", hash)
	}

	newManifest := addEntryToManifest(client, mhash, path, m.Entries[0])
	fmt.Println(newManifest)
}

//清单更新将替换给定路径上清单的现有条目。
//最后一个参数new entry hash必须是清单的hash
//只有一个条目，这些元数据将添加到原始清单中。
//成功后，此函数将打印更新清单的哈希。
func manifestUpdate(ctx *cli.Context) {
	args := ctx.Args()
	if len(args) != 3 {
		utils.Fatalf("Need exactly three arguments <MHASH> <path> <HASH>")
	}

	var (
		mhash = args[0]
		path  = args[1]
		hash  = args[2]
	)

	bzzapi := strings.TrimRight(ctx.GlobalString(SwarmApiFlag.Name), "/")
	client := swarm.NewClient(bzzapi)

	m, _, err := client.DownloadManifest(hash)
	if err != nil {
		utils.Fatalf("Error downloading manifest to update: %v", err)
	}
	l := len(m.Entries)
	if l == 0 {
		utils.Fatalf("No entries in manifest %s", hash)
	} else if l > 1 {
		utils.Fatalf("Too many entries in manifest %s", hash)
	}

	newManifest, _, defaultEntryUpdated := updateEntryInManifest(client, mhash, path, m.Entries[0], true)
	if defaultEntryUpdated {
//
//
//
		fmt.Fprintln(os.Stderr, "Manifest default entry is updated, too")
	}
	fmt.Println(newManifest)
}

//manifestremove删除给定路径上清单的现有条目。
//成功后，此函数将打印清单的哈希，但该哈希没有
//
func manifestRemove(ctx *cli.Context) {
	args := ctx.Args()
	if len(args) != 2 {
		utils.Fatalf("Need exactly two arguments <MHASH> <path>")
	}

	var (
		mhash = args[0]
		path  = args[1]
	)

	bzzapi := strings.TrimRight(ctx.GlobalString(SwarmApiFlag.Name), "/")
	client := swarm.NewClient(bzzapi)

	newManifest := removeEntryFromManifest(client, mhash, path)
	fmt.Println(newManifest)
}

func addEntryToManifest(client *swarm.Client, mhash, path string, entry api.ManifestEntry) string {
	var longestPathEntry = api.ManifestEntry{}

	mroot, isEncrypted, err := client.DownloadManifest(mhash)
	if err != nil {
		utils.Fatalf("Manifest download failed: %v", err)
	}

//看看我们的道路是否在这张清单中，或者我们需要更深入地挖掘
	for _, e := range mroot.Entries {
		if path == e.Path {
			utils.Fatalf("Path %s already present, not adding anything", path)
		} else {
			if e.ContentType == api.ManifestType {
				prfxlen := strings.HasPrefix(path, e.Path)
				if prfxlen && len(path) > len(longestPathEntry.Path) {
					longestPathEntry = e
				}
			}
		}
	}

	if longestPathEntry.Path != "" {
//
		newPath := path[len(longestPathEntry.Path):]
		newHash := addEntryToManifest(client, longestPathEntry.Hash, newPath, entry)

//替换父清单的哈希
		newMRoot := &api.Manifest{}
		for _, e := range mroot.Entries {
			if longestPathEntry.Path == e.Path {
				e.Hash = newHash
			}
			newMRoot.Entries = append(newMRoot.Entries, e)
		}
		mroot = newMRoot
	} else {
//
		entry.Path = path
		mroot.Entries = append(mroot.Entries, entry)
	}

	newManifestHash, err := client.UploadManifest(mroot, isEncrypted)
	if err != nil {
		utils.Fatalf("Manifest upload failed: %v", err)
	}
	return newManifestHash
}

//
//
//
//
//
//
func updateEntryInManifest(client *swarm.Client, mhash, path string, entry api.ManifestEntry, isRoot bool) (newManifestHash, oldHash string, defaultEntryUpdated bool) {
	var (
		newEntry         = api.ManifestEntry{}
		longestPathEntry = api.ManifestEntry{}
	)

	mroot, isEncrypted, err := client.DownloadManifest(mhash)
	if err != nil {
		utils.Fatalf("Manifest download failed: %v", err)
	}

//看看我们的道路是否在这张清单中，或者我们需要更深入地挖掘
	for _, e := range mroot.Entries {
		if path == e.Path {
			newEntry = e
//
//
			oldHash = e.Hash
		} else {
			if e.ContentType == api.ManifestType {
				prfxlen := strings.HasPrefix(path, e.Path)
				if prfxlen && len(path) > len(longestPathEntry.Path) {
					longestPathEntry = e
				}
			}
		}
	}

	if longestPathEntry.Path == "" && newEntry.Path == "" {
		utils.Fatalf("Path %s not present in the Manifest, not setting anything", path)
	}

	if longestPathEntry.Path != "" {
//
		newPath := path[len(longestPathEntry.Path):]
		var newHash string
		newHash, oldHash, _ = updateEntryInManifest(client, longestPathEntry.Hash, newPath, entry, false)

//替换父清单的哈希
		newMRoot := &api.Manifest{}
		for _, e := range mroot.Entries {
			if longestPathEntry.Path == e.Path {
				e.Hash = newHash
			}
			newMRoot.Entries = append(newMRoot.Entries, e)

		}
		mroot = newMRoot
	}

//
//检查是否应更新默认条目
	if newEntry.Path != "" || isRoot {
//替换叶清单的哈希
		newMRoot := &api.Manifest{}
		for _, e := range mroot.Entries {
			if newEntry.Path == e.Path {
				entry.Path = e.Path
				newMRoot.Entries = append(newMRoot.Entries, entry)
			} else if isRoot && e.Path == "" && e.Hash == oldHash {
				entry.Path = e.Path
				newMRoot.Entries = append(newMRoot.Entries, entry)
				defaultEntryUpdated = true
			} else {
				newMRoot.Entries = append(newMRoot.Entries, e)
			}
		}
		mroot = newMRoot
	}

	newManifestHash, err = client.UploadManifest(mroot, isEncrypted)
	if err != nil {
		utils.Fatalf("Manifest upload failed: %v", err)
	}
	return newManifestHash, oldHash, defaultEntryUpdated
}

func removeEntryFromManifest(client *swarm.Client, mhash, path string) string {
	var (
		entryToRemove    = api.ManifestEntry{}
		longestPathEntry = api.ManifestEntry{}
	)

	mroot, isEncrypted, err := client.DownloadManifest(mhash)
	if err != nil {
		utils.Fatalf("Manifest download failed: %v", err)
	}

//看看我们的道路是否在这张清单中，或者我们需要更深入地挖掘
	for _, entry := range mroot.Entries {
		if path == entry.Path {
			entryToRemove = entry
		} else {
			if entry.ContentType == api.ManifestType {
				prfxlen := strings.HasPrefix(path, entry.Path)
				if prfxlen && len(path) > len(longestPathEntry.Path) {
					longestPathEntry = entry
				}
			}
		}
	}

	if longestPathEntry.Path == "" && entryToRemove.Path == "" {
		utils.Fatalf("Path %s not present in the Manifest, not removing anything", path)
	}

	if longestPathEntry.Path != "" {
//
		newPath := path[len(longestPathEntry.Path):]
		newHash := removeEntryFromManifest(client, longestPathEntry.Hash, newPath)

//替换父清单的哈希
		newMRoot := &api.Manifest{}
		for _, entry := range mroot.Entries {
			if longestPathEntry.Path == entry.Path {
				entry.Hash = newHash
			}
			newMRoot.Entries = append(newMRoot.Entries, entry)
		}
		mroot = newMRoot
	}

	if entryToRemove.Path != "" {
//删除此清单中的条目
		newMRoot := &api.Manifest{}
		for _, entry := range mroot.Entries {
			if entryToRemove.Path != entry.Path {
				newMRoot.Entries = append(newMRoot.Entries, entry)
			}
		}
		mroot = newMRoot
	}

	newManifestHash, err := client.UploadManifest(mroot, isEncrypted)
	if err != nil {
		utils.Fatalf("Manifest upload failed: %v", err)
	}
	return newManifestHash
}

