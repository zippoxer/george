package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/zippoxer/george/forge"
)

type cache struct {
	client    *forge.Client
	servers   []forge.Server
	serversMu sync.Mutex
	sites     map[int][]forge.Site // Map of server id to it's sites.
	sitesMu   sync.Mutex
}

func newCache(client *forge.Client) *cache {
	return &cache{
		client: client,
		sites:  make(map[int][]forge.Site),
	}
}

func (c *cache) Servers() ([]forge.Server, error) {
	c.serversMu.Lock()
	ok := c.servers != nil
	c.serversMu.Unlock()
	if ok {
		return c.servers, nil
	}
	servers, err := c.client.Servers().List()
	if err != nil {
		return nil, err
	}
	c.serversMu.Lock()
	c.servers = servers
	c.serversMu.Unlock()
	return servers, nil
}

func (c *cache) Server(id int) (*forge.Server, error) {
	servers, err := c.Servers()
	if err != nil {
		return nil, err
	}
	c.serversMu.Lock()
	defer c.serversMu.Unlock()
	for _, server := range servers {
		if server.Id == id {
			return &server, nil
		}
	}
	return nil, fmt.Errorf("Server not found.")
}

func (c *cache) Sites(serverId int) ([]forge.Site, error) {
	c.sitesMu.Lock()
	sites, ok := c.sites[serverId]
	c.sitesMu.Unlock()
	if ok {
		return sites, nil
	}
	sites, err := c.client.Sites(serverId).List()
	if err != nil {
		return nil, err
	}
	c.sitesMu.Lock()
	c.sites[serverId] = sites
	c.sitesMu.Unlock()
	return sites, nil
}

type ServerSites struct {
	forge.Server
	Sites []forge.Site
}

func (c *cache) ServerSites(servers []forge.Server) ([]ServerSites, error) {
	if servers == nil {
		var err error
		servers, err = c.Servers()
		if err != nil {
			return nil, err
		}
	}
	dataChan := make(chan ServerSites)
	errChan := make(chan error, len(servers))
	for _, server := range servers {
		go func(server forge.Server) {
			sites, err := c.Sites(server.Id)
			if err != nil {
				errChan <- err
			}
			dataChan <- ServerSites{Server: server, Sites: sites}
		}(server)
	}
	results := make([]ServerSites, 0, len(servers))
	for i := 0; i < len(servers); i++ {
		select {
		case data := <-dataChan:
			results = append(results, data)
		case err := <-errChan:
			return nil, err
		}
	}
	return results, nil
}

type cacheDump struct {
	Updated time.Time
	Servers []forge.Server
	Sites   map[int][]forge.Site
}

var cacheForever = time.Duration(-1)

func (c *cache) Load(fileName string, maxAge time.Duration) error {
	f, err := os.Open(fileName)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var dump cacheDump
	if err := json.NewDecoder(f).Decode(&dump); err != nil {
		return err
	}

	if maxAge != cacheForever && time.Since(dump.Updated) > maxAge {
		return nil
	}

	c.servers = dump.Servers
	c.sites = dump.Sites
	return nil
}

func (c *cache) Dump(fileName string) error {
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	dump := cacheDump{
		Updated: time.Now(),
		Servers: c.servers,
		Sites:   c.sites,
	}
	return json.NewEncoder(f).Encode(dump)
}
