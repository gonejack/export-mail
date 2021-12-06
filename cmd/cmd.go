package cmd

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime"
	"net/mail"
	"os"
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

	TLSDisabled  bool `name:"disable-tls" help:"Turn TLS off."`
	ServerRemove bool `name:"server-remove" help:"Remove from server after export."`

	Num int `name:"num" help:"How many mails going to save, 0 for no limit." default:"0"`

	Verbose bool `short:"v" help:"Verbose printing."`
	About   bool `help:"About."`
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

	if e.Num == 0 {
		e.Num = math.MaxInt
	}

	uids, err := e.Uidl(0)
	if err != nil {
		return fmt.Errorf("list message failed: %w", err)
	}
	saved := e.readUid()

	n := 0
	for i := len(uids) - 1; i >= 0; i-- {
		id := uids[i]

		if id.UID != "" {
			_, exist := saved[id.UID]
			if exist {
				logrus.Debugf("messsage %d %s is saved before, skipped", id.ID, id.UID)
				continue
			}
		}

		err = e.save(id.ID)
		if err != nil {
			logrus.Errorf("save message %d failed: %s", id.ID, err)
			continue
		}

		if e.ServerRemove {
			err = e.Dele(id.ID)
			if err != nil {
				logrus.Errorf("remove message %d failed: %s", id.ID, err)
				continue
			}
		}

		if id.UID != "" {
			saved[id.UID] = id.ID
			e.saveUid(saved)
		}

		if n += 1; n >= e.Num {
			return
		}
	}

	return
}
func (e *Exporter) save(id int) (err error) {
	log := logrus.WithField("messageId", id)

	log.Debugf("read header")
	msg, err := e.Top(id, 0)
	if err != nil {
		return fmt.Errorf("read header failed: %w", err)
	}
	name := name(msg)
	file := name + ".eml"
	log.Debugf("read header done: %s", name)

	_, err = os.Stat(file)
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
	err = os.WriteFile(file, bf.Bytes(), 0766)
	if err != nil {
		return fmt.Errorf("save message %s failed: %s", file, err)
	}
	log.Infof("save message %s done", file)

	return
}
func (e *Exporter) connect() (err error) {
	clt := pop3.New(pop3.Opt{
		Host:       e.Host,
		Port:       e.Port,
		TLSEnabled: !e.TLSDisabled,
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
func (e *Exporter) readUid() (dat map[string]int) {
	dat = make(map[string]int)
	f, err := os.Open("saved-uid.json")
	if err != nil {
		return
	}
	defer f.Close()
	_ = json.NewDecoder(f).Decode(&dat)
	return
}
func (e *Exporter) saveUid(dat map[string]int) {
	f, err := os.Create("saved-uid.json")
	if err != nil {
		return
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", " ")
	_ = enc.Encode(dat)
	return
}

func name(msg *message.Entity) string {
	from := msg.Header.Get("From")
	dec, err := rfc2047Decode(from)
	if err == nil {
		addrs, _ := mail.ParseAddressList(dec)
		if len(addrs) > 0 {
			from = addrs[0].Address
		}
	}
	from = strings.Trim(maxLen(from, 30), "<>")

	date := date(msg).Format("2006-01-02 15:04:05")

	msd := md5s(msg.Header.Get("Message-Id"))
	if len(msd) > 5 {
		msd = msd[:5]
	}

	subj := msg.Header.Get("Subject")
	dec, err = rfc2047Decode(subj)
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
func md5s(str string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(str)))
}
func rfc2047Decode(word string) (string, error) {
	match := strings.HasPrefix(word, "=?") && strings.Contains(word, "?=")
	if !match {
		return word, nil
	}
	switch {
	case strings.Contains(word, "?Q?"):
	case strings.Contains(word, "?q?"):
	case strings.Contains(word, "?B?"):
	case strings.Contains(word, "?b?"):
	default:
		return word, nil
	}

	parts := strings.Split(word, "?")
	if len(parts) < 5 {
		return word, nil
	}

	if parts[2] == "B" && strings.HasSuffix(parts[3], "=") {
		b64s := strings.TrimRight(parts[3], "=")
		text, _ := base64.RawURLEncoding.DecodeString(b64s)
		parts[3] = base64.StdEncoding.EncodeToString(text)
	}

	word = strings.Join(parts, "?")
	dec := &mime.WordDecoder{
		CharsetReader: func(charset string, input io.Reader) (io.Reader, error) {
			enc, err := htmlindex.Get(charset)
			if err != nil {
				return nil, err
			}
			return transform.NewReader(input, enc.NewDecoder()), nil
		},
	}
	return dec.DecodeHeader(word)
}
