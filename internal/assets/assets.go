package assets

import "embed"

// StaticFS embeds the compiled frontend assets.
//
//go:embed static/*
var StaticFS embed.FS
