package txcos

import "path"

// TrimPrefixSlash 去除路径开始的斜线
func TrimPrefixSlash(filePath string) string {
	filePath = path.Clean(filePath)
	if filePath != "" && filePath[0] == '/' {
		filePath = filePath[1:]
	}
	return filePath
}
