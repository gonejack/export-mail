# export-mail
This command line tool exports .eml files from POP3 account.

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/gonejack/export-mail)
![Build](https://github.com/gonejack/export-mail/actions/workflows/go.yml/badge.svg)
[![GitHub license](https://img.shields.io/github/license/gonejack/export-mail.svg?color=blue)](LICENSE)

### Install
```shell
> go get github.com/gonejack/export-mail
```

### Usage
```shell
> export-mail --host imap.example.com --username username --password password
```
```
Flags:
  -h, --help                 Show context-sensitive help.
      --host=STRING          Set pop3 host.
      --port=995             Set pop3 port.
      --password=STRING      Set Password.
      --username=STRING      Set Username.
      --enable-tls           Set pop3 TLS enabled.
      --delete               Delete exported mails from server.
      --save-dir="./mail"    Set save directory.
      --total=0              How many to get.
  -v, --verbose              Verbose printing.
      --about                About.
```
