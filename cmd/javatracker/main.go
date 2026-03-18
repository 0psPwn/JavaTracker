package main

import (
	"flag"
	"log"

	"javatracker/internal/javatracker"
)

func main() {
	addr := flag.String("addr", ":8090", "HTTP listen address")
	root := flag.String("root", "", "Java project root to index on startup")
	flag.Parse()

	if err := javatracker.Run(*addr, *root); err != nil {
		log.Fatal(err)
	}
}
