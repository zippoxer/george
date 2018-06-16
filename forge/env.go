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

type envGetResponse struct {
	Content string `json:"content"`
}

func (e *Env) Get() (string, error) {
	req := &Request{
		Method: "GET",
		Path:   fmt.Sprintf("/servers/%d/sites/%d/env", e.serverId, e.siteId),
	}
	var resp envGetResponse
	err := e.c.Do(context.Background(), req, &resp)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}
