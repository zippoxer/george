package main

import (
	"george/forge"
)

type SSH struct {
	Client         *forge.Client
	Cache          *cache
	PrivateKeyPath string
}
