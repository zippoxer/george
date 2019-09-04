package forge

import (
	"context"
	"fmt"
	"log"

	"github.com/joho/godotenv"
)

type DotEnv map[string]string

func (e DotEnv) Get(keys ...string) string {
	for _, key := range keys {
		if s, _ := e[key]; s != "" {
			return s
		}
	}
	return ""
}

type Env struct {
	serverId int
	siteId   int
	c        *Client
}

func (e *Env) Get() (DotEnv, error) {
	req := &Request{
		Method: "GET",
		Path:   fmt.Sprintf("/servers/%d/sites/%d/env", e.serverId, e.siteId),
	}
	var env []byte
	err := e.c.Do(context.Background(), req, &env)
	if err != nil {
		return nil, err
	}
	m, err := godotenv.Unmarshal(string(env))
	if err != nil {
		log.Fatal(err)
	}
	return DotEnv(m), nil
}
