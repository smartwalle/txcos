package txcos

// PresignedInfo 上传文件预签名信息
type PresignedInfo struct {
	UploadURL string            // 上传地址
	FilePath  string            // 文件路径
	Header    map[string]string // 上传请求头
}

// SceneType 场景类型，用于从业务上对文件的使用场景进行分类
type SceneType int

type Scene struct {
	SceneType   SceneType // 场景类型
	Path        string    // 存储路径
	Extensions  []string  // 支持上传的文件类型(文件后缀，如 txt, png)
	Attachments []string  // 需要作为附件响应的文件类型(文件后缀，如 txt)
}
