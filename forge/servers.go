package forge

import (
	"context"
)

type Server struct {
	Id               int           `json:"id"`
	CredentialId     int           `json:"credential_id"`
	Name             string        `json:"name"`
	Size             string        `json:"size"`
	Region           string        `json:"region"`
	PhpVersion       string        `json:"php_version"`
	IPAddress        string        `json:"ip_address"`
	PrivateIPAddress string        `json:"private_ip_address"`
	BlackfireStatus  interface{}   `json:"blackfire_status"`
	PapertrailStatus interface{}   `json:"papertrail_status"`
	Revoked          bool          `json:"revoked"`
	CreatedAt        string        `json:"created_at"`
	IsReady          bool          `json:"is_ready"`
	Network          []interface{} `json:"network"`
}

type Servers struct {
	c *Client
}

type serversListResponse struct {
	Servers []Server
}

func (s *Servers) List() ([]Server, error) {
	req := &Request{
		Method: "GET",
		Path:   "/servers",
	}
	var resp serversListResponse
	err := s.c.Do(context.Background(), req, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Servers, nil
}
