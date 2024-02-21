package site

import "embed"

//go:embed static/*
var Static embed.FS

//go:embed templates/*.html
var Templates embed.FS
