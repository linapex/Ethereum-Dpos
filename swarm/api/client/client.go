
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:46</date>
//</624342668566073344>


package client

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/swarm/api"
	"github.com/ethereum/go-ethereum/swarm/storage/mru"
)

var (
DefaultGateway = "http://本地主机：8500
	DefaultClient  = NewClient(DefaultGateway)
)

var (
	ErrUnauthorized = errors.New("unauthorized")
)

func NewClient(gateway string) *Client {
	return &Client{
		Gateway: gateway,
	}
}

//客户端将与Swarm HTTP网关的交互进行包装。
type Client struct {
	Gateway string
}

//uploadraw将原始数据上载到swarm并返回结果哈希。如果加密是真的
//上载加密数据
func (c *Client) UploadRaw(r io.Reader, size int64, toEncrypt bool) (string, error) {
	if size <= 0 {
		return "", errors.New("data size must be greater than zero")
	}
	addr := ""
	if toEncrypt {
		addr = "encrypt"
	}
	req, err := http.NewRequest("POST", c.Gateway+"/bzz-raw:/"+addr, r)
	if err != nil {
		return "", err
	}
	req.ContentLength = size
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected HTTP status: %s", res.Status)
	}
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

//downloadraw从swarm下载原始数据，它返回readcloser和bool
//内容已加密
func (c *Client) DownloadRaw(hash string) (io.ReadCloser, bool, error) {
	uri := c.Gateway + "/bzz-raw:/" + hash
	res, err := http.DefaultClient.Get(uri)
	if err != nil {
		return nil, false, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, false, fmt.Errorf("unexpected HTTP status: %s", res.Status)
	}
	isEncrypted := (res.Header.Get("X-Decrypted") == "true")
	return res.Body, isEncrypted, nil
}

//文件表示群清单中的文件，用于上传和
//从Swarm下载内容
type File struct {
	io.ReadCloser
	api.ManifestEntry
}

//打开打开一个本地文件，然后可以将其传递到客户端。上载以上载
//它蜂拥而至
func Open(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &File{
		ReadCloser: f,
		ManifestEntry: api.ManifestEntry{
			ContentType: mime.TypeByExtension(filepath.Ext(path)),
			Mode:        int64(stat.Mode()),
			Size:        stat.Size(),
			ModTime:     stat.ModTime(),
		},
	}, nil
}

//上载将文件上载到Swarm，并将其添加到现有清单中
//（如果manifest参数非空）或创建包含
//文件，返回生成的清单哈希（然后该文件将
//可在bzz:/<hash>/<path>）获取
func (c *Client) Upload(file *File, manifest string, toEncrypt bool) (string, error) {
	if file.Size <= 0 {
		return "", errors.New("file size must be greater than zero")
	}
	return c.TarUpload(manifest, &FileUploader{file}, "", toEncrypt)
}

//下载从swarm manifest下载具有给定路径的文件
//给定的哈希（即它得到bzz:/<hash>/<path>）
func (c *Client) Download(hash, path string) (*File, error) {
	uri := c.Gateway + "/bzz:/" + hash + "/" + path
	res, err := http.DefaultClient.Get(uri)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("unexpected HTTP status: %s", res.Status)
	}
	return &File{
		ReadCloser: res.Body,
		ManifestEntry: api.ManifestEntry{
			ContentType: res.Header.Get("Content-Type"),
			Size:        res.ContentLength,
		},
	}, nil
}

//uploadDirectory将目录树上载到swarm并添加文件
//到现有清单（如果清单参数非空）或创建
//新清单，返回生成的清单哈希（来自
//目录将在bzz:/<hash>/path/to/file处可用，其中
//默认路径中指定的文件正在上载到清单的根目录
//（即bzz/<hash>/）
func (c *Client) UploadDirectory(dir, defaultPath, manifest string, toEncrypt bool) (string, error) {
	stat, err := os.Stat(dir)
	if err != nil {
		return "", err
	} else if !stat.IsDir() {
		return "", fmt.Errorf("not a directory: %s", dir)
	}
	if defaultPath != "" {
		if _, err := os.Stat(filepath.Join(dir, defaultPath)); err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("the default path %q was not found in the upload directory %q", defaultPath, dir)
			}
			return "", fmt.Errorf("default path: %v", err)
		}
	}
	return c.TarUpload(manifest, &DirectoryUploader{dir}, defaultPath, toEncrypt)
}

//下载目录下载群清单中包含的文件
//到本地目录的给定路径（现有文件将被覆盖）
func (c *Client) DownloadDirectory(hash, path, destDir, credentials string) error {
	stat, err := os.Stat(destDir)
	if err != nil {
		return err
	} else if !stat.IsDir() {
		return fmt.Errorf("not a directory: %s", destDir)
	}

	uri := c.Gateway + "/bzz:/" + hash + "/" + path
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return err
	}
	if credentials != "" {
		req.SetBasicAuth("", credentials)
	}
	req.Header.Set("Accept", "application/x-tar")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return ErrUnauthorized
	default:
		return fmt.Errorf("unexpected HTTP status: %s", res.Status)
	}
	tr := tar.NewReader(res.Body)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
//忽略默认路径文件
		if hdr.Name == "" {
			continue
		}

		dstPath := filepath.Join(destDir, filepath.Clean(strings.TrimPrefix(hdr.Name, path)))
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return err
		}
		var mode os.FileMode = 0644
		if hdr.Mode > 0 {
			mode = os.FileMode(hdr.Mode)
		}
		dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
		if err != nil {
			return err
		}
		n, err := io.Copy(dst, tr)
		dst.Close()
		if err != nil {
			return err
		} else if n != hdr.Size {
			return fmt.Errorf("expected %s to be %d bytes but got %d", hdr.Name, hdr.Size, n)
		}
	}
}

//下载文件将单个文件下载到目标目录中
//如果清单项未指定文件名-它将回退
//以文件名的形式传递到文件的哈希
func (c *Client) DownloadFile(hash, path, dest, credentials string) error {
	hasDestinationFilename := false
	if stat, err := os.Stat(dest); err == nil {
		hasDestinationFilename = !stat.IsDir()
	} else {
		if os.IsNotExist(err) {
//不存在-应创建
			hasDestinationFilename = true
		} else {
			return fmt.Errorf("could not stat path: %v", err)
		}
	}

	manifestList, err := c.List(hash, path, credentials)
	if err != nil {
		return err
	}

	switch len(manifestList.Entries) {
	case 0:
		return fmt.Errorf("could not find path requested at manifest address. make sure the path you've specified is correct")
	case 1:
//持续
	default:
		return fmt.Errorf("got too many matches for this path")
	}

	uri := c.Gateway + "/bzz:/" + hash + "/" + path
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return err
	}
	if credentials != "" {
		req.SetBasicAuth("", credentials)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return ErrUnauthorized
	default:
		return fmt.Errorf("unexpected HTTP status: expected 200 OK, got %d", res.StatusCode)
	}
	filename := ""
	if hasDestinationFilename {
		filename = dest
	} else {
//尝试断言
re := regexp.MustCompile("[^/]+$") //最后一个斜线后的所有内容

		if results := re.FindAllString(path, -1); len(results) > 0 {
			filename = results[len(results)-1]
		} else {
			if entry := manifestList.Entries[0]; entry.Path != "" && entry.Path != "/" {
				filename = entry.Path
			} else {
//如果命令行中没有任何内容，则假定hash为名称
				filename = hash
			}
		}
		filename = filepath.Join(dest, filename)
	}
	filePath, err := filepath.Abs(filename)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0777); err != nil {
		return err
	}

	dst, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, res.Body)
	return err
}

//上载清单将给定清单上载到Swarm
func (c *Client) UploadManifest(m *api.Manifest, toEncrypt bool) (string, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return c.UploadRaw(bytes.NewReader(data), int64(len(data)), toEncrypt)
}

//下载清单下载群清单
func (c *Client) DownloadManifest(hash string) (*api.Manifest, bool, error) {
	res, isEncrypted, err := c.DownloadRaw(hash)
	if err != nil {
		return nil, isEncrypted, err
	}
	defer res.Close()
	var manifest api.Manifest
	if err := json.NewDecoder(res).Decode(&manifest); err != nil {
		return nil, isEncrypted, err
	}
	return &manifest, isEncrypted, nil
}

//列出具有给定前缀、分组的群清单中的列表文件
//使用“/”作为分隔符的常见前缀。
//
//例如，如果清单表示以下目录结构：
//
//文件1.TXT
//文件2.TXT
//DRI1/FIL3.TXT
//dir1/dir2/file4.txt文件
//
//然后：
//
//-前缀“”将返回[dir1/，file1.txt，file2.txt]
//-前缀“file”将返回[file1.txt，file2.txt]
//-前缀“dir1/”将返回[dir1/dir2/，dir1/file3.txt]
//
//其中以“/”结尾的条目是常见的前缀。
func (c *Client) List(hash, prefix, credentials string) (*api.ManifestList, error) {
	req, err := http.NewRequest(http.MethodGet, c.Gateway+"/bzz-list:/"+hash+"/"+prefix, nil)
	if err != nil {
		return nil, err
	}
	if credentials != "" {
		req.SetBasicAuth("", credentials)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return nil, ErrUnauthorized
	default:
		return nil, fmt.Errorf("unexpected HTTP status: %s", res.Status)
	}
	var list api.ManifestList
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		return nil, err
	}
	return &list, nil
}

//上载程序使用提供的上载将文件上载到Swarm fn
type Uploader interface {
	Upload(UploadFn) error
}

type UploaderFunc func(UploadFn) error

func (u UploaderFunc) Upload(upload UploadFn) error {
	return u(upload)
}

//DirectoryUploader上载目录中的所有文件，可以选择上载
//默认路径的文件
type DirectoryUploader struct {
	Dir string
}

//上载执行目录和默认路径的上载
func (d *DirectoryUploader) Upload(upload UploadFn) error {
	return filepath.Walk(d.Dir, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if f.IsDir() {
			return nil
		}
		file, err := Open(path)
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(d.Dir, path)
		if err != nil {
			return err
		}
		file.Path = filepath.ToSlash(relPath)
		return upload(file)
	})
}

//文件上载程序上载单个文件
type FileUploader struct {
	File *File
}

//上载执行文件上载
func (f *FileUploader) Upload(upload UploadFn) error {
	return upload(f.File)
}

//uploadfn是传递给上载程序以执行上载的函数类型。
//对于单个文件（例如，目录上载程序将调用
//目录树中每个文件的uploadfn）
type UploadFn func(file *File) error

//tar upload使用给定的上传器将文件作为tar流上传到swarm，
//返回结果清单哈希
func (c *Client) TarUpload(hash string, uploader Uploader, defaultPath string, toEncrypt bool) (string, error) {
	reqR, reqW := io.Pipe()
	defer reqR.Close()
	addr := hash

//如果已经存在哈希（清单），那么该清单将确定上载是否
//是否加密。如果没有清单，则toEncrypt参数决定
//是否加密。
	if hash == "" && toEncrypt {
//这是加密上载端点的内置地址
		addr = "encrypt"
	}
	req, err := http.NewRequest("POST", c.Gateway+"/bzz:/"+addr, reqR)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-tar")
	if defaultPath != "" {
		q := req.URL.Query()
		q.Set("defaultpath", defaultPath)
		req.URL.RawQuery = q.Encode()
	}

//使用“expect:100 continue”，以便在以下情况下不发送请求正文：
//服务器拒绝请求
	req.Header.Set("Expect", "100-continue")

	tw := tar.NewWriter(reqW)

//定义将文件添加到tar流的uploadfn
	uploadFn := func(file *File) error {
		hdr := &tar.Header{
			Name:    file.Path,
			Mode:    file.Mode,
			Size:    file.Size,
			ModTime: file.ModTime,
			Xattrs: map[string]string{
				"user.swarm.content-type": file.ContentType,
			},
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err = io.Copy(tw, file)
		return err
	}

//在Goroutine中运行上载，以便我们可以发送请求头和
//在发送tar流之前，等待“100 continue”响应
	go func() {
		err := uploader.Upload(uploadFn)
		if err == nil {
			err = tw.Close()
		}
		reqW.CloseWithError(err)
	}()

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected HTTP status: %s", res.Status)
	}
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

//multipartupload使用给定的上载程序将文件作为
//多部分表单，返回结果清单哈希
func (c *Client) MultipartUpload(hash string, uploader Uploader) (string, error) {
	reqR, reqW := io.Pipe()
	defer reqR.Close()
	req, err := http.NewRequest("POST", c.Gateway+"/bzz:/"+hash, reqR)
	if err != nil {
		return "", err
	}

//使用“expect:100 continue”，以便在以下情况下不发送请求正文：
//服务器拒绝请求
	req.Header.Set("Expect", "100-continue")

	mw := multipart.NewWriter(reqW)
	req.Header.Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%q", mw.Boundary()))

//定义将文件添加到多部分表单的uploadfn
	uploadFn := func(file *File) error {
		hdr := make(textproto.MIMEHeader)
		hdr.Set("Content-Disposition", fmt.Sprintf("form-data; name=%q", file.Path))
		hdr.Set("Content-Type", file.ContentType)
		hdr.Set("Content-Length", strconv.FormatInt(file.Size, 10))
		w, err := mw.CreatePart(hdr)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, file)
		return err
	}

//在Goroutine中运行上载，以便我们可以发送请求头和
//在发送多部分表单之前，请等待“100继续”响应
	go func() {
		err := uploader.Upload(uploadFn)
		if err == nil {
			err = mw.Close()
		}
		reqW.CloseWithError(err)
	}()

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected HTTP status: %s", res.Status)
	}
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

//createResource创建具有给定名称和频率的可变资源，并使用提供的
//数据。数据是否解释为多哈希，取决于多哈希参数。
//starttime=0表示“现在”
//返回生成的可变资源清单地址，可用于包含在ENS解析程序（setcontent）中。
//或引用将来的更新（client.updateResource）
func (c *Client) CreateResource(request *mru.Request) (string, error) {
	responseStream, err := c.updateResource(request)
	if err != nil {
		return "", err
	}
	defer responseStream.Close()

	body, err := ioutil.ReadAll(responseStream)
	if err != nil {
		return "", err
	}

	var manifestAddress string
	if err = json.Unmarshal(body, &manifestAddress); err != nil {
		return "", err
	}
	return manifestAddress, nil
}

//UpdateResource允许您设置内容的新版本
func (c *Client) UpdateResource(request *mru.Request) error {
	_, err := c.updateResource(request)
	return err
}

func (c *Client) updateResource(request *mru.Request) (io.ReadCloser, error) {
	body, err := request.MarshalJSON()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.Gateway+"/bzz-resource:/", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	return res.Body, nil

}

//GetResource返回包含资源原始内容的字节流
//ManifestAddressOrDomain是您在CreateResource或其解析程序的ENS域中获得的地址。
//指向那个地址
func (c *Client) GetResource(manifestAddressOrDomain string) (io.ReadCloser, error) {

	res, err := http.Get(c.Gateway + "/bzz-resource:/" + manifestAddressOrDomain)
	if err != nil {
		return nil, err
	}
	return res.Body, nil

}

//GetResourceMetadata返回一个描述可变资源的结构
//ManifestAddressOrDomain是您在CreateResource或其解析程序的ENS域中获得的地址。
//指向那个地址
func (c *Client) GetResourceMetadata(manifestAddressOrDomain string) (*mru.Request, error) {

	responseStream, err := c.GetResource(manifestAddressOrDomain + "/meta")
	if err != nil {
		return nil, err
	}
	defer responseStream.Close()

	body, err := ioutil.ReadAll(responseStream)
	if err != nil {
		return nil, err
	}

	var metadata mru.Request
	if err := metadata.UnmarshalJSON(body); err != nil {
		return nil, err
	}
	return &metadata, nil
}

