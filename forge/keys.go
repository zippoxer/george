package forge

import (
	"context"
	"fmt"
)

type Key struct {
	Id        int    `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt Time   `json:"created_at"`
}

type Keys struct {
	serverId int
	c        *Client
}

type createKeyRequest struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type keysCreateResponse struct {
	Key Key
}

func (k *Keys) Create(name, key string) (*Key, error) {
	req := NewRequest("POST", fmt.Sprintf("/servers/%d/keys", k.serverId), createKeyRequest{
		Name: name,
		Key:  key,
	})
	var resp keysCreateResponse
	err := k.c.Do(context.Background(), req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Key, nil
}

type keysListResponse struct {
	Keys []Key
}

func (k *Keys) List() ([]Key, error) {
	req := NewRequest("GET", fmt.Sprintf("/servers/%d/keys", k.serverId), nil)
	var resp keysListResponse
	err := k.c.Do(context.Background(), req, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Keys, nil
}

type keysGetResponse struct {
	Key Key
}

func (k *Keys) Get(id int) (*Key, error) {
	req := NewRequest("GET", fmt.Sprintf("/servers/%d/keys/%d", k.serverId, id), nil)
	var resp keysGetResponse
	err := k.c.Do(context.Background(), req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Key, nil
}
