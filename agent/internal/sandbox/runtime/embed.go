package runtime

import "embed"

// FS contains files installed into every sandbox workspace.
//
//go:embed files/*
var FS embed.FS
