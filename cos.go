package txcos

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/tencentyun/cos-go-sdk-v5"
	sts "github.com/tencentyun/qcloud-cos-sts-sdk/go"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// Client COS  https://cloud.tencent.cn/document/api/436/7751
type Client struct {
	secretID     string
	secretKey    string
	appID        string
	bucketName   string
	region       string
	baseURL      *cos.BaseURL
	scenes       map[SceneType]*Scene
	contentTypes map[string]string

	client    *cos.Client
	stsClient *sts.Client
}

func New(secretID, secretKey, appID, bucket, region string) (*Client, error) {
	var bucketName = fmt.Sprintf("%s-%s", bucket, appID)
	var nClient = &Client{}
	nClient.secretID = secretID
	nClient.secretKey = secretKey
	nClient.appID = appID
	nClient.bucketName = bucketName
	nClient.region = region

	nBucketURL, err := cos.NewBucketURL(bucketName, region, true)
	if err != nil {
		return nil, err
	}
	nClient.baseURL = &cos.BaseURL{
		BucketURL:  nBucketURL,
		ServiceURL: nBucketURL,
		BatchURL:   nBucketURL,
		CIURL:      nBucketURL,
		FetchURL:   nBucketURL,
	}

	nClient.scenes = make(map[SceneType]*Scene)
	nClient.contentTypes = make(map[string]string)

	nClient.client = cos.NewClient(nClient.baseURL, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  secretID,
			SecretKey: secretKey,
		},
	})
	nClient.stsClient = sts.NewClient(secretID, secretKey, nil)
	return nClient, nil
}

func (c *Client) SecretID() string {
	return c.secretID
}

func (c *Client) SecretKey() string {
	return c.secretKey
}

func (c *Client) BaseURL() *cos.BaseURL {
	return c.baseURL
}

func (c *Client) AppID() string {
	return c.appID
}

func (c *Client) Bucket() string {
	return c.bucketName
}

func (c *Client) Region() string {
	return c.region
}

func (c *Client) ContentType(fileExt string) string {
	return c.contentTypes[fileExt]
}

// RegisterScene 注册支持的业务场景类型
func (c *Client) RegisterScene(scene *Scene) {
	if scene != nil && scene.Path != "" && len(scene.Extensions) > 0 {
		c.scenes[scene.SceneType] = scene
	}
}

// AllowContentType 设置支持上传的文件 Content-Type
func (c *Client) AllowContentType(fileExt string, contentType string) {
	if fileExt != "" && contentType != "" {
		c.contentTypes[fileExt] = contentType
	}
}

// BuildUploadFileInfo 构建待上传文件的COS路径、ContentType以及是否必须以附件方式响应
func (c *Client) BuildUploadFileInfo(sceneType SceneType, filename string, paths ...string) (filePath, contentType string, attachment bool, err error) {
	if filename == "" {
		return "", "", attachment, errors.New("文件名不能为空")
	}
	var fileExt = filepath.Ext(filename)
	if fileExt == "" {
		return "", "", attachment, errors.New("文件后缀不能为空")
	}
	fileExt = strings.TrimPrefix(fileExt, ".")

	// 获取文件场景
	scene, ok := c.scenes[sceneType]
	if !ok {
		return "", "", attachment, errors.New("文件场景不存在")
	}

	// 验证是否为“文件场景”有效的文件类型
	var supportExt = false
	for _, ext := range scene.Extensions {
		if fileExt == ext {
			supportExt = true
			break
		}
	}
	if !supportExt {
		return "", "", attachment, errors.New("不支持的文件类型")
	}

	for _, ext := range scene.Attachments {
		if fileExt == ext {
			attachment = true
			break
		}
	}

	// 获取文件的 Content-Type
	contentType = c.ContentType(fileExt)
	if contentType == "" {
		return "", "", attachment, errors.New("未知的文件类型")
	}

	// 构建待上传文件的COS路径
	var newPaths = make([]string, 0, len(paths)+3)
	newPaths = append(newPaths, "/")
	newPaths = append(newPaths, scene.Path)
	newPaths = append(newPaths, paths...)
	newPaths = append(newPaths, fmt.Sprintf("%s_%d.%s", base64.URLEncoding.EncodeToString([]byte(uuid.New().String()+filename)), time.Now().UnixNano(), fileExt))

	filePath = filepath.Join(newPaths...)

	return TrimPrefixSlash(filePath), contentType, attachment, nil
}

func (c *Client) GetUploadCredentialPolicyStatement(resources, contentTypes []string) (statements []sts.CredentialPolicyStatement, err error) {
	if len(resources) < 1 {
		return nil, errors.New("资源路径不能为空")
	}
	if len(contentTypes) < 1 {
		return nil, errors.New("ContentType 不能为空")
	}
	var base = fmt.Sprintf("qcs::cos:%s:uid/%s:%s", c.region, c.appID, c.bucketName)
	var resourceList = make([]string, 0, len(resources))
	for _, resource := range resources {
		resourceList = append(resourceList, filepath.Join(base, resource))
	}
	// https://cloud.tencent.cn/document/product/598/69901
	statements = []sts.CredentialPolicyStatement{
		{
			Action: []string{
				//简单上传操作
				"name/cos:PutObject",
				//表单上传对象
				"name/cos:PostObject",
				//分块上传：初始化分块操作
				"name/cos:InitiateMultipartUpload",
				//分块上传：List 进行中的分块上传
				"name/cos:ListMultipartUploads",
				//分块上传：List 已上传分块操作
				"name/cos:ListParts",
				//分块上传：上传分块操作
				"name/cos:UploadPart",
				//分块上传：完成所有分块上传操作
				"name/cos:CompleteMultipartUpload",
				//取消分块上传操作
				"name/cos:AbortMultipartUpload",
			},
			Effect:   "allow",
			Resource: resourceList,
			Condition: map[string]map[string]interface{}{
				"string_equal_ignore_case": {
					"cos:content-type": contentTypes,
				},
			},
		},
	}
	return statements, nil
}

func (c *Client) GetViewCredentialPolicyStatement(resources []string) (statements []sts.CredentialPolicyStatement, err error) {
	if len(resources) < 1 {
		return nil, errors.New("资源路径不能为空")
	}
	var base = fmt.Sprintf("qcs::cos:%s:uid/%s:%s", c.region, c.appID, c.bucketName)

	var resourceList = make([]string, 0, len(resources))
	for _, resource := range resources {
		resourceList = append(resourceList, filepath.Join(base, resource))
	}
	// https://cloud.tencent.cn/document/product/598/69901
	statements = []sts.CredentialPolicyStatement{
		{
			Action: []string{
				//下载操作
				"name/cos:GetObject",
			},
			Effect:   "allow",
			Resource: resourceList,
		},
	}
	return statements, nil
}

// GetTmpUploadCredentials 获取临时上传凭证
func (c *Client) GetTmpUploadCredentials(resources, contentTypes []string, expired time.Duration) (credentials *sts.Credentials, err error) {
	credentialPolicyStatementList, err := c.GetUploadCredentialPolicyStatement(resources, contentTypes)
	if err != nil {
		return nil, err
	}
	credentialOpts := &sts.CredentialOptions{
		DurationSeconds: int64(expired.Seconds()),
		Region:          c.region,
		Policy: &sts.CredentialPolicy{
			Statement: credentialPolicyStatementList,
		},
	}
	credential, err := c.stsClient.GetCredential(credentialOpts)
	if err != nil {
		return nil, err
	}
	if credential == nil || credential.Credentials == nil {
		return nil, errors.New("获取COS临时密钥异常")
	}
	return credential.Credentials, nil
}

// GetTmpViewCredentials 获取临时访问凭证
func (c *Client) GetTmpViewCredentials(resources []string, expired time.Duration) (credentials *sts.Credentials, err error) {
	credentialPolicyStatementList, err := c.GetViewCredentialPolicyStatement(resources)
	if err != nil {
		return nil, err
	}
	credentialOpts := &sts.CredentialOptions{
		DurationSeconds: int64(expired.Seconds()),
		Region:          c.region,
		Policy: &sts.CredentialPolicy{
			Statement: credentialPolicyStatementList,
		},
	}
	credential, err := c.stsClient.GetCredential(credentialOpts)
	if err != nil {
		return nil, err
	}
	if credential == nil || credential.Credentials == nil {
		return nil, errors.New("获取COS临时密钥异常")
	}
	return credential.Credentials, nil
}

// GetTmpUploadPresignedInfo 使用"临时凭证"获取上传文件预签名URL
func (c *Client) GetTmpUploadPresignedInfo(ctx context.Context, sceneType SceneType, dispositionType DispositionType, filename string, expired time.Duration, paths ...string) (presignedInfo *PresignedInfo, err error) {
	// 构建待上传文件的COS路径及ContentType
	uploadFilePath, contentType, attachment, err := c.BuildUploadFileInfo(sceneType, filename, paths...)
	if err != nil {
		return nil, err
	}

	// 获取临时上传密钥
	credentials, err := c.GetTmpUploadCredentials([]string{uploadFilePath}, []string{contentType}, expired)
	if err != nil {
		return nil, err
	}

	var opts = &cos.PresignedURLOptions{
		Query:      &url.Values{},
		Header:     &http.Header{},
		SignMerged: true,
	}
	if attachment || dispositionType == DispositionTypeAttachment {
		opts.Header.Add("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	}
	opts.Header.Add("Content-Type", contentType)
	opts.Query.Add("x-cos-security-token", credentials.SessionToken)

	var secretID = credentials.TmpSecretID
	var secretKey = credentials.TmpSecretKey

	var cosClient = cos.NewClient(c.baseURL, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  secretID,
			SecretKey: secretKey,
		},
	})

	// 获取预签名 URL
	presignedURL, err := cosClient.Object.GetPresignedURL(ctx, http.MethodPut, uploadFilePath, secretID, secretKey, expired, opts)
	if err != nil {
		return nil, err
	}

	presignedInfo = &PresignedInfo{}
	presignedInfo.UploadURL = presignedURL.String()
	presignedInfo.FilePath = uploadFilePath
	presignedInfo.Header = make(map[string]string)
	for key := range *opts.Header {
		presignedInfo.Header[key] = opts.Header.Get(key)
	}
	return presignedInfo, nil
}

// GetTmpViewPresignedURL 使用"临时凭证"获取访问文件预签名URL
func (c *Client) GetTmpViewPresignedURL(ctx context.Context, filePath string, param *url.Values, expired time.Duration) (string, error) {
	if filePath == "" {
		return "", errors.New("路径不能为空")
	}
	filePath = TrimPrefixSlash(filePath)

	if param == nil {
		param = &url.Values{}
	}

	// 获取临时访问密钥
	credentials, err := c.GetTmpViewCredentials([]string{filePath}, expired)
	if err != nil {
		return "", err
	}

	var opts = &cos.PresignedURLOptions{
		Query:      param,
		Header:     &http.Header{},
		SignMerged: true,
	}
	opts.Query.Set("x-cos-security-token", credentials.SessionToken)

	var secretID = credentials.TmpSecretID
	var secretKey = credentials.TmpSecretKey

	var cosClient = cos.NewClient(c.baseURL, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  secretID,
			SecretKey: secretKey,
		},
	})

	// 获取预签名 URL
	presignedURL, err := cosClient.Object.GetPresignedURL(ctx, http.MethodGet, filePath, secretID, secretKey, expired, opts)
	if err != nil {
		return "", err
	}

	return presignedURL.String(), nil
}

// GetUploadPresignedInfo 获取上传文件预签名URL
func (c *Client) GetUploadPresignedInfo(ctx context.Context, sceneType SceneType, dispositionType DispositionType, filename string, expired time.Duration, paths ...string) (presignedInfo *PresignedInfo, err error) {
	// 构建待上传文件的COS路径及ContentType
	uploadFilePath, contentType, attachment, err := c.BuildUploadFileInfo(sceneType, filename, paths...)
	if err != nil {
		return nil, err
	}

	var opts = &cos.PresignedURLOptions{
		Query:      &url.Values{},
		Header:     &http.Header{},
		SignMerged: true,
	}
	if attachment || dispositionType == DispositionTypeAttachment {
		opts.Header.Add("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	}
	opts.Header.Add("Content-Type", contentType)

	// 获取预签名 URL
	presignedURL, err := c.client.Object.GetPresignedURL(ctx, http.MethodPut, uploadFilePath, c.secretID, c.secretKey, expired, opts)
	if err != nil {
		return nil, err
	}

	presignedInfo = &PresignedInfo{}
	presignedInfo.UploadURL = presignedURL.String()
	presignedInfo.FilePath = uploadFilePath
	presignedInfo.Header = make(map[string]string)
	for key := range *opts.Header {
		presignedInfo.Header[key] = opts.Header.Get(key)
	}
	return presignedInfo, nil
}

// GetViewPresignedURL 获取访问文件预签名URL
func (c *Client) GetViewPresignedURL(ctx context.Context, filePath string, param *url.Values, expired time.Duration) (string, error) {
	if filePath == "" {
		return "", errors.New("路径不能为空")
	}
	filePath = TrimPrefixSlash(filePath)

	if param == nil {
		param = &url.Values{}
	}

	var opts = &cos.PresignedURLOptions{
		Query:      param,
		Header:     &http.Header{},
		SignMerged: true,
	}

	// 获取预签名 URL
	presignedURL, err := c.client.Object.GetPresignedURL(ctx, http.MethodGet, filePath, c.secretID, c.secretKey, expired, opts)
	if err != nil {
		return "", err
	}

	return presignedURL.String(), nil
}

// GetPreviewFileURL 获取文件预览URL
func (c *Client) GetPreviewFileURL(ctx context.Context, filePath string, expired time.Duration) (string, error) {
	var param = &url.Values{}
	param.Add("ci-process", "doc-preview")
	param.Add("dstType", "html")
	param.Add("copyable", "0")
	param.Add("htmlwaterword", "")
	param.Add("htmlfillstyle", "cmdiYSgxOTIsMTkyLDE5MiwwLjYp")
	param.Add("htmlfront", "Ym9sZCAyMHB4IFNlcmlm")
	param.Add("htmlrotate", "325")
	param.Add("htmlhorizontal", "100")
	param.Add("htmlvertical", "100")

	fileURL, err := c.GetViewPresignedURL(ctx, filePath, param, expired)
	if err != nil {
		return "", err
	}
	return fileURL, nil
}

// GetFileURL 获取文件访问URL
func (c *Client) GetFileURL(ctx context.Context, filePath string, expired time.Duration) (string, error) {
	fileURL, err := c.GetViewPresignedURL(ctx, filePath, &url.Values{}, expired)
	if err != nil {
		return "", err
	}
	return fileURL, nil
}

// PutFromObject 上传对象
//
// sceneType 场景类型
//
// dispositionType 响应文件的方式
//
// filename 下载保存文件名
//
// reader 待上传对象
func (c *Client) PutFromObject(ctx context.Context, sceneType SceneType, dispositionType DispositionType, filename string, object io.Reader, paths ...string) (string, error) {
	// 构建待上传文件的COS路径及ContentType
	uploadFilePath, contentType, attachment, err := c.BuildUploadFileInfo(sceneType, filename, paths...)
	if err != nil {
		return "", err
	}

	var opts = &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{},
	}

	if attachment || dispositionType == DispositionTypeAttachment {
		opts.ObjectPutHeaderOptions.ContentDisposition = fmt.Sprintf("attachment; filename=%s", filename)
		opts.ObjectPutHeaderOptions.ContentType = contentType
	}

	if _, err = c.client.Object.Put(ctx, uploadFilePath, object, opts); err != nil {
		return "", err
	}
	return uploadFilePath, nil
}

// PutFromFile 上传文件
//
// sceneType 场景类型
//
// dispositionType 响应文件的方式
//
// filePath 待上传本地文件
func (c *Client) PutFromFile(ctx context.Context, sceneType SceneType, dispositionType DispositionType, filePath string, paths ...string) (string, error) {
	var filename = filepath.Base(filePath)
	// 构建待上传文件的COS路径及ContentType
	uploadFilePath, contentType, attachment, err := c.BuildUploadFileInfo(sceneType, filename, paths...)
	if err != nil {
		return "", err
	}

	var opts = &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{},
	}

	if attachment || dispositionType == DispositionTypeAttachment {
		opts.ObjectPutHeaderOptions.ContentDisposition = fmt.Sprintf("attachment; filename=%s", filename)
		opts.ObjectPutHeaderOptions.ContentType = contentType
	}

	if _, err = c.client.Object.PutFromFile(ctx, uploadFilePath, filePath, opts); err != nil {
		return "", err
	}
	return uploadFilePath, nil
}
