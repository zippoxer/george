package forge

import (
	"context"
	"fmt"
)

type Env struct {
	serverId int
	siteId   int
	c        *Client
}

func (e *Env) Get() (string, error) {
	req := &Request{
		Method: "GET",
		Path:   fmt.Sprintf("/servers/%d/sites/%d/env", e.serverId, e.siteId),
	}
	var env []byte
	err := e.c.Do(context.Background(), req, &env)
	if err != nil {
		return "", err
	}
	return string(env), nil
}
