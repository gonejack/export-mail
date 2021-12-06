package main

import (
	"log"

	"github.com/gonejack/export-mail/cmd"
)

func main() {
	var c cmd.Exporter

	if e := c.Run(); e != nil {
		log.Fatalln(e)
	}
}
