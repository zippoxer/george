package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base32"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"time"

	"github.com/gobwas/glob"
	"github.com/zippoxer/george/forge"
)

type George struct {
	client  *forge.Client
	cache   *cache
	homeDir string
}

func New(client *forge.Client) *George {
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}
	return &George{
		client:  client,
		cache:   newCache(client),
		homeDir: usr.HomeDir,
	}
}

func (g *George) Search(pattern string) (*forge.Server, *forge.Site, error) {
	if pattern == "" {
		pattern = "*"
	}
	expr, err := glob.Compile(pattern)
	if err != nil {
		return nil, nil, err
	}

	servers, err := g.cache.Servers()
	if err != nil {
		return nil, nil, err
	}
	var matchingServers []forge.Server
	for _, server := range servers {
		if expr.Match(server.Name) ||
			expr.Match(server.IPAddress) {
			matchingServers = append(matchingServers, server)
		}
	}
	if len(matchingServers) > 1 {
		err = g.printServers(servers, expr)
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("More than one server matches %q.", pattern)
	}
	if len(matchingServers) == 0 {
		serverSites, err := g.cache.ServerSites(servers)
		if err != nil {
			return nil, nil, err
		}
		var matchingSites []forge.Site
		for _, server := range serverSites {
			for _, site := range server.Sites {
				if expr.Match(site.Name) {
					matchingSites = append(matchingSites, site)
				}
			}
		}
		if len(matchingSites) > 1 {
			err = g.printServers(servers, expr)
			if err != nil {
				return nil, nil, err
			}
			return nil, nil, fmt.Errorf("More than one site matches %q.", pattern)
		}
		if len(matchingSites) == 0 {
			return nil, nil, fmt.Errorf("Server or site not found.")
		}
		site := &matchingSites[0]
		var server *forge.Server
		for _, s := range servers {
			if s.Id == site.ServerId {
				server = &s
				break
			}
		}
		if server == nil {
			return nil, nil, fmt.Errorf("Site found, but it's server wasn't found. Seems like data is corrupt.")
		}
		return server, site, nil
	}
	return &matchingServers[0], nil, nil
}

func (g *George) printServers(servers []forge.Server, expr glob.Glob) error {
	fmt.Printf("Available servers:\n")
	serverSites, err := g.cache.ServerSites(servers)
	if err != nil {
		return err
	}
	for _, server := range serverSites {
		fmt.Printf("  %s (%s)\n", server.Name, server.IPAddress)
		for _, site := range server.Sites {
			if expr != nil && !expr.Match(site.Name) {
				continue
			}
			fmt.Printf("    %s\n", site.Name)
		}
	}
	return nil
}

// SSHInstallKey registers the public SSH key with the given forge server,
// returning an error if installation failed.
func (g *George) SSHInstallKey(serverId int) error {
	publicKey, err := g.sshPublicKey()
	if err != nil {
		return err
	}
	keyName := g.sshKeyName(publicKey)

	keys, err := g.client.Keys(serverId).List()
	if err != nil {
		return err
	}
	var key *forge.Key
	for i := range keys {
		if keys[i].Name == keyName {
			key = &keys[i]
			break
		}
	}
	if key == nil {
		key, err := g.client.Keys(serverId).Create(keyName, string(publicKey))
		if err != nil {
			return err
		}
		for key.Status == "installing" {
			key, err = g.client.Keys(serverId).Get(key.Id)
			if err != nil {
				return err
			}
			time.Sleep(time.Millisecond * 500)
		}
		if key.Status != "installed" {
			return fmt.Errorf("failed installing SSH key: status is %v", key.Status)
		}
	}
	return nil
}

func (g *George) sshPublicKey() ([]byte, error) {
	privateKeyPath := filepath.Join(g.homeDir, ".ssh", "id_rsa")
	publicKeyPath := privateKeyPath + ".pub"
	data, err := ioutil.ReadFile(publicKeyPath)
	if os.IsNotExist(err) || len(data) == 0 {
		fmt.Println("Generating an SSH key...")
		// -N="" means empty password.
		_, err := exec.Command("ssh-keygen", "-N", "", "-f", privateKeyPath).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("ssh-keygen: %v", err)
		}
		return g.sshPublicKey()
	}
	return data, err
}

// sshKeyName generates a unique, reproducible name for the given SSH key.
func (s *George) sshKeyName(key []byte) string {
	sum := sha512.Sum512_224(key)
	sumStr := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:])
	return "george-" + sumStr
}

// Encrypt encrypts data using 256-bit AES-GCM.  This both hides the content of
// the data and provides a check that it hasn't been altered. Output takes the
// form nonce|ciphertext|tag where '|' indicates concatenation.
func Encrypt(plaintext []byte, key *[32]byte) (ciphertext []byte, err error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	_, err = io.ReadFull(rand.Reader, nonce)
	if err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt decrypts data using 256-bit AES-GCM.  This both hides the content of
// the data and provides a check that it hasn't been altered. Expects input
// form nonce|ciphertext|tag where '|' indicates concatenation.
func Decrypt(ciphertext []byte, key *[32]byte) (plaintext []byte, err error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("malformed ciphertext")
	}

	return gcm.Open(nil,
		ciphertext[:gcm.NonceSize()],
		ciphertext[gcm.NonceSize():],
		nil,
	)
}
