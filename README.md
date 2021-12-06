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
  -h, --help               Show context-sensitive help.
      --host=STRING        Set pop3 host.
      --port=995           Set pop3 port.
      --username=STRING    Set Username.
      --password=STRING    Set Password.
      --disable-tls        Turn TLS off.
      --server-remove      Remove from server after export.
      --num=9999           How many mails going to save.
  -v, --verbose            Verbose printing.
      --about              About.
```
