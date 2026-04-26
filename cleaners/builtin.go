// Package cleaners provides embedded builtin cleaner definitions.
// Package cleaners 提供嵌入的内置清理器定义。
package cleaners

import "embed"

// Builtin contains the shipped cleaner definitions.
// Builtin 包含随附的清理器定义。
//
//go:embed user/*.yaml user/langs/*.yaml user/k8s/*.yaml user/containers/*.yaml
//go:embed system/*.yaml system/containers/*.yaml
var Builtin embed.FS
