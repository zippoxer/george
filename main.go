package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"

	"github.com/joho/godotenv"

	"github.com/zippoxer/george/forge"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	encryptionKey = &[32]byte{0xfb, 0xb7, 0x41, 0xbf, 0xb2, 0xe5, 0x8, 0xb8, 0x3e, 0x6e, 0x4, 0x5f, 0x39, 0x2c, 0x55, 0xb9, 0xa1, 0x4e, 0xbb, 0x72, 0x5a, 0xd7, 0xa0, 0xe1, 0xb3, 0x83, 0x11, 0xeb, 0x98, 0xd7, 0x19, 0xce}
)

var (
	app = kingpin.New("george", "A toolkit for Laravel Forge.")

	appLogin = app.Command("login",
		"Login with API key provided at https://forge.laravel.com/user/profile#/api")
	appLoginKey = appLogin.Arg("api-key", "").Required().String()

	appSSH = app.Command("ssh",
		"SSH to a server by name, IP or site domain. Wildcards are supported.")
	appSSHTarget = appSSH.Arg("server", "Server name, IP or site domain.").String()

	appMySQLDump = app.Command("mysqldump",
		"mysqldump a website.")
	appMySQLDumpSite = appMySQLDump.Arg("site", "Site name.").Required().String()
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
	george := New(client)

	switch cmd {
	case appLogin.FullCommand():
		err = saveAPIKey(*appLoginKey)
		if err != nil {
			log.Fatal(err)
		}
	case appSSH.FullCommand():
		server, site, err := george.Search(*appSSHTarget)
		if err != nil {
			log.Fatal(err)
		}
		err = george.SSHInstallKey(server.Id)
		if err != nil {
			log.Fatal(err)
		}
		var flags, args []string
		args = append(args, "forge@"+server.IPAddress)
		if site != nil {
			flags = append(flags, "-t")
			args = append(args, fmt.Sprintf("cd %q; bash -l", site.Name))
		}
		cmd := exec.Command("ssh", append(flags, args...)...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			log.Fatal(err)
		}
	case appMySQLDump.FullCommand():
		server, site, err := george.SearchSite(*appMySQLDumpSite)
		if err != nil {
			log.Fatal(err)
		}

		// Parse .env file and extract MySQL connection settings.
		envRaw, err := client.Env(server.Id, site.Id).Get()
		if err != nil {
			log.Fatal(err)
		}
		env, err := godotenv.Unmarshal(envRaw)
		if err != nil {
			log.Fatal(err)
		}
		dbConn, _ := env["DB_CONNECTION"]
		if dbConn == "" {
			log.Fatal("No database found for this site.")
		}
		if dbConn != "mysql" {
			log.Fatalf("Unsupported database %s", dbConn)
		}
		dbHost, _ := env["DB_HOST"]
		dbPort, _ := env["DB_PORT"]
		dbName, _ := env["DB_DATABASE"]
		dbUser, _ := env["DB_USERNAME"]
		dbPwd, _ := env["DB_PASSWORD"]

		// Open an SSH session and execute mysqldump.
		session, err := george.SSH(server.Id)
		if err != nil {
			log.Fatal(err)
		}
		pr, pw := io.Pipe()
		done := make(chan struct{})
		go func() {
			gr, err := gzip.NewReader(pr)
			if err != nil {
				log.Fatal(err)
			}
			_, err = io.Copy(os.Stdout, gr)
			if err == io.ErrClosedPipe {
				done <- struct{}{}
				return
			}
			if err != nil {
				log.Fatal(err)
			}
		}()
		session.Stdout = pw
		session.Stderr = os.Stderr
		cmd := fmt.Sprintf("mysqldump --host=%q --port=%q --user=%q --password=%q %q | gzip",
			dbHost, dbPort, dbUser, dbPwd, dbName)
		if err := session.Start(cmd); err != nil {
			log.Fatalf("mysqldump: %v", err.Error())
		}
		if err := session.Wait(); err != nil {
			log.Fatalf("mysqldump: %v", err.Error())
		}
		pr.Close()
		<-done
	default:
		app.FatalUsage("No command specified.")
	}
	return
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
