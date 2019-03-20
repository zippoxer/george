package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"text/template"
	"time"

	"github.com/phayes/freeport"
	"github.com/skratchdot/open-golang/open"
	"github.com/zippoxer/george/forge"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	// encryptionKey is used to encrypt Forge's API key before saving it.
	encryptionKey = &[32]byte{0xfb, 0xb7, 0x41, 0xbf, 0xb2, 0xe5, 0x8, 0xb8, 0x3e, 0x6e, 0x4, 0x5f, 0x39, 0x2c, 0x55, 0xb9, 0xa1, 0x4e, 0xbb, 0x72, 0x5a, 0xd7, 0xa0, 0xe1, 0xb3, 0x83, 0x11, 0xeb, 0x98, 0xd7, 0x19, 0xce}
)

var (
	app = kingpin.New("george", "A toolkit for Laravel Forge.")

	appLogin = app.Command("login",
		"Login with API key provided at https://forge.laravel.com/user/profile#/api")
	appLoginKey = appLogin.Arg("api-key", "").Required().String()

	appSSH = app.Command("ssh",
		"SSH to a server by name, IP or site domain. Wildcards are supported.")
	appSSHTarget = appSSH.
			Arg("server", "Server name, IP or site domain.").
			Required().
			String()

	appTunnel = app.Command("tunnel",
		"Opens an SSH tunnel that forwards a server's port a local port.")
	appTunnelTarget = appTunnel.
			Arg("target", "Server name, IP or site domain.").
			Required().
			String()
	appTunnelRemote = appTunnel.
			Arg("remote-port", "").
			Default("3306").
			Uint16()
	appTunnelLocal = appTunnel.
			Arg("local-port", "").
			Default("3307").
			Uint16()

	appMySQLDump     = app.Command("mysqldump", "mysqldump a website.")
	appMySQLDumpSite = appMySQLDump.Arg("site", "Site name.").Required().String()

	appLog     = app.Command("log", "Print the latest Laravel application log.")
	appLogSite = appLog.Arg("site", "Site name.").Required().String()

	appSequelPro     = app.Command("sequelpro", "Open a site's database in Sequel Pro.")
	appSequelProSite = appSequelPro.Arg("site", "Site name.").
				Required().
				String()

	appWinSCP       = app.Command("winscp", "Opens an SFTP connection to a site or server in WinSCP.")
	appWinSCPTarget = appWinSCP.Arg("target", "Server name, IP or site domain.").
			Required().
			String()
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
	case appLog.FullCommand():
		server, site, err := george.SearchSite(*appLogSite)
		if err != nil {
			log.Fatal(err)
		}
		session, err := george.SSH(server.Id)
		if err != nil {
			log.Fatal(err)
		}
		session.Stderr = os.Stderr
		pr, err := session.StdoutPipe()
		if err != nil {
			log.Fatal(err)
		}
		cmd := fmt.Sprintf("cat %q | gzip", site.Name+"/storage/logs/laravel.log")
		if err := session.Start(cmd); err != nil {
			log.Fatalf("%s: %v", cmd, err)
		}
		gzr, err := gzip.NewReader(pr)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := io.Copy(os.Stdout, gzr); err != nil {
			log.Fatal(err)
		}
		if err := session.Wait(); err != nil {
			log.Fatalf("%s: %v", cmd, err)
		}
	case appTunnel.FullCommand():
		server, site, err := george.Search(*appTunnelTarget)
		if err != nil {
			log.Fatal(err)
		}

		if site != nil && *appTunnelRemote == 3306 {
			go func() {
				env, err := client.Env(server.Id, site.Id).Get()
				if err != nil {
					fmt.Printf("failed fetching .env: %v\n", err)
				}
				dbConn := env.Get("DB_CONNECTION")
				dbHost := env.Get("DB_HOST")
				dbPort := env.Get("DB_PORT")
				dbName := env.Get("DB_DATABASE")
				dbUser := env.Get("DB_USERNAME")
				dbPwd := env.Get("DB_PASSWORD")
				fmt.Printf("\n%s credentials for %s:\n", dbConn, site.Name)
				fmt.Printf("  host: %s:%s\n  user: %s\n  password: %s\n  database: %s\n",
					dbHost, dbPort, dbUser, dbPwd, dbName)
			}()
		}

		err = george.SSHInstallKey(server.Id)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("tunneling %s:%d to 127.0.0.1:%d\n",
			server.IPAddress, *appTunnelRemote, *appTunnelLocal)
		cmd := exec.Command("ssh",
			"-L",
			fmt.Sprintf("%d:%s:%d", *appTunnelLocal, server.IPAddress, *appTunnelRemote),
			"-N",
			fmt.Sprintf("forge@%s", server.IPAddress))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		c := make(chan os.Signal, 2)
		signal.Notify(c, os.Interrupt, os.Kill)
		go func() {
			<-c
			cmd.Process.Kill()
			os.Exit(1)
		}()

		err = cmd.Run()
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
		env, err := client.Env(server.Id, site.Id).Get()
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
		dbHost := env.Get("DB_HOST")
		dbPort := env.Get("DB_PORT")
		dbName := env.Get("DB_DATABASE")
		dbUser := env.Get("DB_USERNAME")
		dbPwd := env.Get("DB_PASSWORD")

		// Open an SSH session and execute mysqldump.
		session, err := george.SSH(server.Id)
		if err != nil {
			log.Fatal(err)
		}
		session.Stderr = os.Stderr
		pr, err := session.StdoutPipe()
		if err != nil {
			log.Fatal(err)
		}
		cmd := fmt.Sprintf("mysqldump --host=%q --port=%q --user=%q --password=%q %q | gzip",
			dbHost, dbPort, dbUser, dbPwd, dbName)
		if err := session.Start(cmd); err != nil {
			log.Fatalf("mysqldump: %v", err)
		}
		gzr, err := gzip.NewReader(pr)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := io.Copy(os.Stdout, gzr); err != nil {
			log.Fatal(err)
		}
		if err := session.Wait(); err != nil {
			log.Fatalf("mysqldump: %v", err)
		}
	case appSequelPro.FullCommand():
		server, site, err := george.SearchSite(*appSequelProSite)
		if err != nil {
			log.Fatal(err)
		}

		// Parse .env file and extract MySQL connection settings.
		env, err := client.Env(server.Id, site.Id).Get()
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
		dbPort := env.Get("DB_PORT")
		dbName := env.Get("DB_DATABASE")
		dbUser := env.Get("DB_USERNAME")
		dbPwd := env.Get("DB_PASSWORD")

		// Open an SSH session and execute mysqldump.
		err = george.SSHInstallKey(server.Id)
		if err != nil {
			log.Fatal(err)
		}

		localPort, err := freeport.GetFreePort()
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("tunneling for database %s:%s to 127.0.0.1:%d\n",
			server.IPAddress, dbPort, localPort)
		cmd := exec.Command("ssh",
			"-L",
			fmt.Sprintf("%d:%s:%s", localPort, server.IPAddress, dbPort),
			"-N",
			fmt.Sprintf("forge@%s", server.IPAddress))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		c := make(chan os.Signal, 2)
		signal.Notify(c, os.Interrupt, os.Kill)
		go func() {
			<-c
			cmd.Process.Kill()
			os.Exit(1)
		}()

		go func() {
			time.Sleep(1 * time.Second)

			fmt.Printf("Opening database in Sequel Pro...")

			const fileTemplate = `
	<?xml version="1.0" encoding="UTF-8"?>
	<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
	<plist version="1.0">
	<dict>
		<key>ContentFilters</key>
		<dict/>
		<key>auto_connect</key>
		<true/>
		<key>data</key>
		<dict>
			<key>connection</key>
			<dict>
				<key>colorIndex</key>
				<integer>0</integer>
				<key>database</key>
				<string>{{.DbName}}</string>
				<key>host</key>
				<string>127.0.0.1</string>
				<key>name</key>
				<string>{{.SiteName}}</string>
				<key>password</key>
				<string>{{.DbPwd}}</string>
				<key>port</key>
				<string>{{.LocalPort}}</string>
				<key>rdbms_type</key>
				<string>mysql</string>
				<key>sslCACertFileLocation</key>
				<string></string>
				<key>sslCACertFileLocationEnabled</key>
				<integer>0</integer>
				<key>sslCertificateFileLocation</key>
				<string></string>
				<key>sslCertificateFileLocationEnabled</key>
				<integer>0</integer>
				<key>sslKeyFileLocation</key>
				<string></string>
				<key>sslKeyFileLocationEnabled</key>
				<integer>0</integer>
				<key>type</key>
				<string>SPTCPIPConnection</string>
				<key>useSSL</key>
				<integer>0</integer>
				<key>user</key>
				<string>{{.DbUser}}</string>
			</dict>
		</dict>
		<key>encrypted</key>
		<false/>
		<key>format</key>
		<string>connection</string>
		<key>queryFavorites</key>
		<array/>
		<key>queryHistory</key>
		<array />
		<key>rdbms_type</key>
		<string>mysql</string>
		<key>rdbms_version</key>
		<string>5.6.10</string>
		<key>version</key>
		<integer>1</integer>
	</dict>
	</plist>
	`

			t := template.Must(template.New("spf").Parse(fileTemplate))
			file, err := os.Create(os.TempDir() + site.Name + ".spf")
			if err != nil {
				log.Fatal(err)
			}

			t.Execute(file, struct {
				DbName    string
				DbPwd     string
				DbUser    string
				LocalPort int
				SiteName  string
			}{
				dbName,
				dbPwd,
				dbUser,
				localPort,
				site.Name,
			})

			file.Close()

			open.Run(file.Name())
		}()

		err = cmd.Run()
		if err != nil {
			log.Fatal(err)
		}
	case appWinSCP.FullCommand():
		winscpPaths := []string{
			`C:\Program Files (x86)\WinSCP\WinSCP.exe`,
			`C:\Program Files\WinSCP\WinSCP.exe`,
		}
		var path string
		for _, p := range winscpPaths {
			if _, err := os.Stat(p); err == nil {
				path = p
			}
		}
		if path == "" {
			log.Fatal("WinSCP.exe does not exist.")
		}

		server, site, err := george.Search(*appWinSCPTarget)
		if err != nil {
			log.Fatal(err)
		}

		err = george.SSHInstallKey(server.Id)
		if err != nil {
			log.Fatal(err)
		}

		// Generate PPK (if not exists) using WinSCP.
		privateKeyPath := george.SSHPrivateKeyPath()
		ppkPath := privateKeyPath + ".ppk"
		if _, err := os.Stat(ppkPath); err != nil {
			cmd := exec.Command(
				path,
				"/keygen",
				privateKeyPath,
				fmt.Sprintf(`/output=%s`, ppkPath),
			)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin
			err = cmd.Run()
			if err != nil {
				log.Fatal(err)
			}
		}

		// Start WinSCP to open the connection to the given server.
		connURL := &url.URL{
			Scheme: "sftp",
			User:   url.User("forge"),
			Host:   server.IPAddress,
			Path:   "/",
		}
		args := []string{
			connURL.String(),
			"/privatekey=" + ppkPath,
		}
		if site != nil {
			args = append(args, "/rawsettings", "RemoteDirectory=/home/forge/"+site.Name)
		}
		cmd := exec.Command(
			path,
			args...,
		)
		if err := cmd.Run(); err != nil {
			log.Fatal(err)
		}
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
