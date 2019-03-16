
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:39</date>
//</624342640220966912>


package build

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/Azure/azure-storage-blob-go/2018-03-28/azblob"
)

//azureBlobstoreConfig是一个身份验证和配置结构，其中包含
//Azure SDK与中的speicifc容器交互所需的数据
//博客商店。
type AzureBlobstoreConfig struct {
Account   string //帐户名称以授权API请求
Token     string //上述帐户的访问令牌
Container string //要将文件上载到的Blob容器
}

//AzureBlobstoreUpload uploads a local file to the Azure Blob Storage. 注意，这个
//方法假定最大文件大小为64MB（Azure限制）。较大的文件将
//需要实现多API调用方法。
//
//请参阅：https://msdn.microsoft.com/en-us/library/azure/dd179451.aspx anchor_3
func AzureBlobstoreUpload(path string, name string, config AzureBlobstoreConfig) error {
	if *DryRunFlag {
		fmt.Printf("would upload %q to %s/%s/%s\n", path, config.Account, config.Container, name)
		return nil
	}
//针对Azure云创建经过身份验证的客户端
	credential := azblob.NewSharedKeyCredential(config.Account, config.Token)
	pipeline := azblob.NewPipeline(credential, azblob.PipelineOptions{})

u, _ := url.Parse(fmt.Sprintf("https://%s.blob.core.windows.net“，配置帐户）
	service := azblob.NewServiceURL(*u, pipeline)

	container := service.NewContainerURL(config.Container)
	blockblob := container.NewBlockBlobURL(name)

//将要上载的文件传输到指定的blobstore容器中
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()

	_, err = blockblob.Upload(context.Background(), in, azblob.BlobHTTPHeaders{}, azblob.Metadata{}, azblob.BlobAccessConditions{})
	return err
}

//azureBlobstoreList列出了一个Azure Blobstore中包含的所有文件。
func AzureBlobstoreList(config AzureBlobstoreConfig) ([]azblob.BlobItem, error) {
	credential := azblob.NewSharedKeyCredential(config.Account, config.Token)
	pipeline := azblob.NewPipeline(credential, azblob.PipelineOptions{})

u, _ := url.Parse(fmt.Sprintf("https://%s.blob.core.windows.net“，配置帐户）
	service := azblob.NewServiceURL(*u, pipeline)

//列出容器中的所有blob并将其返回
	container := service.NewContainerURL(config.Container)

	res, err := container.ListBlobsFlatSegment(context.Background(), azblob.Marker{}, azblob.ListBlobsSegmentOptions{
MaxResults: 1024 * 1024 * 1024, //是的，把它们都拿出来
	})
	if err != nil {
		return nil, err
	}
	return res.Segment.BlobItems, nil
}

//azureblobstorelete迭代要删除的文件列表并删除它们
//从墓碑上。
func AzureBlobstoreDelete(config AzureBlobstoreConfig, blobs []azblob.BlobItem) error {
	if *DryRunFlag {
		for _, blob := range blobs {
			fmt.Printf("would delete %s (%s) from %s/%s\n", blob.Name, blob.Properties.LastModified, config.Account, config.Container)
		}
		return nil
	}
//针对Azure云创建经过身份验证的客户端
	credential := azblob.NewSharedKeyCredential(config.Account, config.Token)
	pipeline := azblob.NewPipeline(credential, azblob.PipelineOptions{})

u, _ := url.Parse(fmt.Sprintf("https://%s.blob.core.windows.net“，配置帐户）
	service := azblob.NewServiceURL(*u, pipeline)

	container := service.NewContainerURL(config.Container)

//迭代这些blob并删除它们
	for _, blob := range blobs {
		blockblob := container.NewBlockBlobURL(blob.Name)
		if _, err := blockblob.Delete(context.Background(), azblob.DeleteSnapshotsOptionInclude, azblob.BlobAccessConditions{}); err != nil {
			return err
		}
	}
	return nil
}

