package main

import (
	"bytes"
	_ "embed"

	"github.com/z5labs/bedrock/example/custom_framework/echo/service"
	"github.com/z5labs/bedrock/example/custom_framework/framework"
)

//go:embed config.yaml
var cfgSrc []byte

func main() {
	framework.Rest(bytes.NewReader(cfgSrc), service.Init)
}
