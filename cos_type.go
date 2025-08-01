package txcos

type PresignedInfo struct {
	UploadURL string            // 上传地址
	FilePath  string            // 文件路径
	Header    map[string]string // 上传请求头
}
