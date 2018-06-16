package forge

import (
	"context"
	"fmt"
)

type Site struct {
	Id                 int         `json:"id"`
	Name               string      `json:"name"`
	Directory          string      `json:"directory"`
	Wildcards          bool        `json:"wildcards"`
	Status             string      `json:"status"`
	Repository         string      `json:"repository"`
	RepositoryProvider string      `json:"repository_provider"`
	RepositoryBranch   string      `json:"repository_branch"`
	RepositoryStatus   string      `json:"repository_status"`
	QuickDeploy        bool        `json:"quick_deploy"`
	ProjectType        string      `json:"project_type"`
	App                interface{} `json:"app"`
	AppStatus          interface{} `json:"app_status"`
	HipchatRoom        interface{} `json:"hipchat_room"`
	SlackChannel       interface{} `json:"slack_channel"`
	CreatedAt          string      `json:"created_at"`
}

type Sites struct {
	serverId int
	c        *Client
}

type sitesListResponse struct {
	Sites []Site
}

func (s *Sites) List() ([]Site, error) {
	req := NewRequest("GET", fmt.Sprintf("/servers/%d/sites", s.serverId), nil)
	var resp sitesListResponse
	err := s.c.Do(context.Background(), req, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Sites, nil
}
