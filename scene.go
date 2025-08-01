package txcos

type SceneType int

type Scene struct {
	SceneType SceneType // 场景类型
	Path      string    // 存储路径
	FileExts  []string  // 支持文件类型(文件后缀，如 txt, png)
}
