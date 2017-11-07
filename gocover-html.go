package main

import (
	"flag"
	"os"
)

func main() {
	profile := flag.String("p", "", "Path to profile file.")
	out := flag.String("o", "", "HTML export file.")
	flag.Parse()

	if *profile == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	err := htmlOutput(*profile, *out)
	if err != nil {
		panic(err)
	}
}
