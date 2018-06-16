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
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"time"

	"github.com/gobwas/glob"
	"github.com/zippoxer/george/forge"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	encryptionKey = &[32]byte{0xfb, 0xb7, 0x41, 0xbf, 0xb2, 0xe5, 0x8, 0xb8, 0x3e, 0x6e, 0x4, 0x5f, 0x39, 0x2c, 0x55, 0xb9, 0xa1, 0x4e, 0xbb, 0x72, 0x5a, 0xd7, 0xa0, 0xe1, 0xb3, 0x83, 0x11, 0xeb, 0x98, 0xd7, 0x19, 0xce}
)

var (
	app = kingpin.New("george", "A toolkit for Laravel Forge.")

	appLogin    = app.Command("login", "Login with API key provided at https://forge.laravel.com/user/profile#/api")
	appLoginKey = appLogin.Arg("api-key", "").Required().String()

	appSSH       = app.Command("ssh", "SSH to a server by name, IP or site domain. Wildcards are supported.")
	appSSHTarget = appSSH.Arg("server", "Server name, IP or site domain.").String()
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	app.HelpFlag.Hidden()
	cmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	key, err := loadAPIKey()
	if os.IsNotExist(err) {
		if cmd != "login" {
			log.Fatal("You're not logged in. Login with 'george <api-key>'")
		}
	} else if err != nil {
		log.Fatal(err)
	}

	client := forge.New(key)

	switch cmd {
	case appLogin.FullCommand():
		err = saveAPIKey(*appLoginKey)
		if err != nil {
			log.Fatal(err)
		}
	case appSSH.FullCommand():
		target := *appSSHTarget
		if target == "" {
			target = "*"
		}
		expr, err := glob.Compile(target)
		if err != nil {
			log.Fatal(err)
		}

		sshKey, err := readSSHKey()
		if err != nil {
			log.Fatal(err)
		}
		keyName := generateKeyName(sshKey)

		servers, err := client.Servers().List()
		if err != nil {
			log.Fatal(err)
		}
		var matchingServers []forge.Server
		for i := range servers {
			if expr.Match(servers[i].Name) ||
				expr.Match(servers[i].IPAddress) {
				matchingServers = append(matchingServers, servers[i])
			}
		}
		if len(matchingServers) == 0 {
			log.Fatal("Server not found.")
		}
		if len(matchingServers) > 1 {
			fmt.Printf("Available servers:\n")
			for _, s := range matchingServers {
				fmt.Printf("  %s (%s)\n", s.Name, s.IPAddress)
			}
			os.Exit(1)
		}

		server := servers[0]
		keys, err := client.Keys(server.Id).List()
		if err != nil {
			log.Fatal(err)
		}
		var key *forge.Key
		for i := range keys {
			if keys[i].Name == keyName {
				key = &keys[i]
				break
			}
		}
		if key == nil {
			key, err := client.Keys(server.Id).Create(keyName, string(sshKey))
			if err != nil {
				log.Fatal(err)
			}
			for key.Status == "installing" {
				key, err = client.Keys(server.Id).Get(key.Id)
				if err != nil {
					log.Fatal(err)
				}
				time.Sleep(time.Millisecond * 500)
			}
			if key.Status != "installed" {
				log.Fatalf("failed installing SSH key: status is %v", key.Status)
			}
		}

		cmd := exec.Command("ssh", "forge@"+server.IPAddress)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			log.Fatal(err)
		}
	default:
		app.FatalUsage("no command specified.")
	}
	return
}

func readSSHKey() ([]byte, error) {
	usr, err := user.Current()
	if err != nil {
		return nil, err
	}
	privateKeyPath := filepath.Join(usr.HomeDir, ".ssh", "id_rsa")
	publicKeyPath := privateKeyPath + ".pub"
	data, err := ioutil.ReadFile(publicKeyPath)
	if os.IsNotExist(err) || len(data) == 0 {
		fmt.Println("Generating an SSH key...")
		// -N="" means empty password.
		_, err := exec.Command("ssh-keygen", "-N", "", "-f", privateKeyPath).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("ssh-keygen: %v", err)
		}
		return readSSHKey()
	}
	return data, err
}

func generateKeyName(key []byte) string {
	sum := sha512.Sum512_224(key)
	sumStr := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:])
	return "george-" + sumStr
}

func saveAPIKey(key string) error {
	ciphertext, err := Encrypt([]byte(key), encryptionKey)
	if err != nil {
		return err
	}
	usr, err := user.Current()
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(usr.HomeDir, ".george-key"), ciphertext, 0644)
}

func loadAPIKey() (string, error) {
	path, err := apiKeyPath()
	if err != nil {
		return "", err
	}
	ciphertext, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	key, err := Decrypt(ciphertext, encryptionKey)
	if err != nil {
		return "", err
	}
	return string(key), nil
}

func apiKeyPath() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	return filepath.Join(usr.HomeDir, ".george-key"), nil
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
