# george

A command-line toolkit for [Laravel Forge](https://forge.laravel.com).

## Getting started

### Installing a binary for Windows or Linux

Choose a binary from [releases](https://github.com/zippoxer/george/releases), download it and move it to a directory that's in your PATH environment variable.

If you're on Windows, you should use `george` with [Git Bash](https://git-scm.com/downloads) or another bash-like shell with the `ssh` command and the `~/.ssh` directory available.

### Installing with Go

First, make sure you have [Go](https://golang.org) installed. Then, install `george` with:

```bash
go get -u github.com/zippoxer/george
```

If you get `george: command not found`, then add `$GOPATH/bin` (default for linux is `$HOME/go`, for Windows `%USERPROFILE%\go`) to your PATH and try again.

### Logging in to Forge

Finally, login with API key provided at https://forge.laravel.com/user/profile#/api

```bash
george login "<Forge API Key>"
```

## Commands

### SSH

Quickly SSH into a server or site. No need to register your SSH key, `george` automatically registers your `~/.ssh/id_rsa.pub` into Forge.

```bash
$ george ssh www.example.com
```

If you've typed in a site domain, `george` will drop you right into it's laravel directory!

```bash
forge@example-server:~/www.example.com.il$ php artisan fix-website
```

You can also SSH into a server by it's name or IP address:

```bash
george ssh server-name
george ssh 128.64.32.16
```

### MySQLDump

Tired of using `ssh`, `mysqldump` & `rsync` only to dump your site's database? Don't worry, `george` has got you covered!

A single command to dump a site's database:

```bash
george mysqldump www.example.com > example.sql
```

Behind the scenes, `george` compresses the stream with gzip to transfer the dump _even faster_.

### Log

Print a site's `laravel.log`:

```bash
george log www.example.com
```

For better viewing experience, you might want to open `laravel.log` with [Visual Studio Code](https://code.visualstudio.com/):

```bash
george log www.example.com | code -
```

Behind the scenes, `george` compresses the transfer of the log file with gzip, so even large log files should open within seconds.

### Sequel Pro (Mac only)

Opens a site database in Sequel Pro.

```bash
george sequelpro www.example.com
```

### WinSCP (Windows only)

Opens an SFTP connection to a site or server in WinSCP.

```bash
george winscp www.example.com
```

## About automatic SSH key registration

Before `george` connects to a server for the first time, it registers your default public SSH key (`~/.ssh/id_rsa.pub`) using Forge's API. Unless you switch your SSH key, `george` only registers you once per server.

### How it works

Since the registered key is named after a SHA-2 hash of your public SSH key (`GEORGE_<hash of your public key>`), the next time you connect to the same server, `george` will notice you're already registered and continue without registering you again.
