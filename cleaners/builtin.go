package cleaners

import "embed"

// Builtin contains the shipped cleaner definitions.
//
//go:embed user/*.yaml user/langs/*.yaml user/k8s/*.yaml user/containers/*.yaml
//go:embed system/*.yaml system/containers/*.yaml
var Builtin embed.FS
