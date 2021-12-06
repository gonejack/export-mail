package cmd

import (
	"crypto/md5"
	"fmt"
	"io"
	"math"
	"mime"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/antonfisher/nested-logrus-formatter"
	"github.com/dustin/go-humanize"
	"github.com/emersion/go-message"
	"github.com/knadh/go-pop3"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/transform"
)

func init() {
	logrus.SetOutput(os.Stdout)

	format := &formatter.Formatter{
		TimestampFormat: "2006-01-02 15:04:05",
		NoColors:        false,
		HideKeys:        false,
		CallerFirst:     true,
	}
	logrus.SetFormatter(format)
}

type options struct {
	Host     string `help:"Set pop3 host."`
	Port     int    `help:"Set pop3 port." default:"995"`
	Username string `help:"Set Username."`
	Password string `help:"Set Password."`

	DisableTLS bool `name:"disable-tls" help:"Turn TLS off."`
	Delete     bool `name:"also-remove" help:"Remove from server."`
	Total      int  `name:"fetch-limit" help:"How many mails going to save, 0 for unlimit." default:"0"`

	SaveDir string `help:"Output directory." default:"./mail"`
	Verbose bool   `short:"v" help:"Verbose printing."`
	About   bool   `help:"About."`
}

func (o *options) Parse() (err error) {
	p, _ := kong.New(o,
		kong.Name("export-mail"),
		kong.Description("Command line tool for downloading mails."),
		kong.UsageOnError(),
	)
	_, err = p.Parse(os.Args[1:])
	return
}

type Exporter struct {
	options

	*pop3.Conn
}

func (e *Exporter) Run() (err error) {
	err = e.options.Parse()
	if err != nil {
		return fmt.Errorf("parse argument failed: %w", err)
	}

	if e.About {
		fmt.Println("Visit https://github.com/gonejack/export-mail")
		return
	}
	if e.Verbose {
		logrus.SetLevel(logrus.DebugLevel)
	}
	if e.Total == 0 {
		e.Total = math.MaxInt
	}

	return e.run()
}
func (e *Exporter) run() (err error) {
	err = e.connect()
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	defer e.disconnect()

	count, size, err := e.Stat()
	if err != nil {
		return
	}
	logrus.Infof("found %d messages on server, total size %s", count, humanize.IBytes(uint64(size)))

	id, n, total := count, 50, 0
	for {
		switch {
		case id == 0:
			return
		case total >= e.Total:
			return
		case n == 0:
			if e.Delete {
				_ = e.disconnect()
				err = e.connect()
				if err != nil {
					return fmt.Errorf("connect failed: %w", err)
				}
			}
			n = 50
		}
		err = e.save(id)
		if err != nil {
			return
		}
		if e.Delete {
			err = e.Dele(id)
		}
		if err != nil {
			return
		}
		id, n, total = id-1, n-1, total+1
	}
}
func (e *Exporter) save(id int) (err error) {
	log := logrus.WithField("messageId", id)

	log.Debugf("read header")
	msg, err := e.Top(id, 0)
	if err != nil {
		return fmt.Errorf("read message %d failed: %s", id, err)
	}
	name := name(msg)
	file := name + ".eml"
	path := filepath.Join(e.SaveDir, file)
	log.Debugf("read header done: %s", name)

	_, err = os.Stat(path)
	if err == nil {
		log.Warnf("file %s already exist, skipped", file)
		return
	}

	log.Debugf("read body")
	bf, err := e.RetrRaw(id)
	if err != nil {
		return fmt.Errorf("read body of %s failed: %s", name, err)
	}
	log.Debugf("read body done")

	log.Debugf("save message %s", file)
	err = os.MkdirAll(e.SaveDir, 0766)
	if err != nil {
		return fmt.Errorf("save message %s failed: %s", path, err)
	}
	err = os.WriteFile(path, bf.Bytes(), 0766)
	if err != nil {
		return fmt.Errorf("save message %s failed: %s", path, err)
	}
	log.Infof("save message %s done", file)

	return
}
func (e *Exporter) connect() (err error) {
	clt := pop3.New(pop3.Opt{
		Host:       e.Host,
		Port:       e.Port,
		TLSEnabled: !e.DisableTLS,
	})

	logrus.Infof("connecting %s:%d", e.Host, e.Port)
	e.Conn, err = clt.NewConn()
	if err != nil {
		return
	}

	logrus.Infof("authenticating")
	err = e.Auth(e.Username, e.Password)
	if err != nil {
		return
	}

	return
}
func (e *Exporter) disconnect() (err error) {
	logrus.Infof("disconnecting %s:%d", e.Host, e.Port)
	return e.Quit()
}

func name(msg *message.Entity) string {
	wd := &mime.WordDecoder{
		CharsetReader: func(charset string, input io.Reader) (io.Reader, error) {
			enc, err := htmlindex.Get(charset)
			if err != nil {
				return nil, err
			}
			return transform.NewReader(input, enc.NewDecoder()), nil
		},
	}

	from := msg.Header.Get("From")
	dec, err := wd.DecodeHeader(from)
	if err == nil {
		from = dec
	}
	from = strings.Trim(maxLen(from, 30), "<>")

	date := date(msg).Format("2006-01-02 15:04:05")

	msd := strmd5(msg.Header.Get("Message-Id"))
	if len(msd) > 5 {
		msd = msd[:5]
	}

	subj := msg.Header.Get("Subject")
	dec, err = wd.DecodeHeader(subj)
	if err == nil {
		subj = dec
	}
	subj = maxLen(subj, 30)

	return safetyName(fmt.Sprintf("[%s][%s][%s][%s]", from, date, msd, subj))
}
func date(msg *message.Entity) (t time.Time) {
	p, err := mail.ParseDate(msg.Header.Get("Date"))
	if err == nil {
		t = p
	} else {
		t = time.Now()
	}
	return
}
func safetyName(name string) string {
	return regexp.MustCompile(`[<>:"/\\|?*]`).ReplaceAllString(name, ".")
}
func maxLen(str string, max int) string {
	var rs []rune
	for i, r := range []rune(str) {
		if i >= max {
			if i > 0 {
				rs = append(rs, '.', '.', '.')
			}
			break
		}
		rs = append(rs, r)
	}
	return string(rs)
}
func strmd5(str string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(str)))
}
