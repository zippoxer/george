package main

import (
	"github.com/zippoxer/george/forge"
)

type SSH struct {
	Client         *forge.Client
	Cache          *cache
	PrivateKeyPath string
}
