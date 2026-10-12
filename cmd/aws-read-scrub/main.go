// Command aws-read-scrub turns a recorded read (`infrafactory rehearse
// read`) and the kept pre-destroy tfstate into fixtures fit to commit.
// It is a dev tool, not the public CLI: harness.WriteScrubbedAWSRead
// says what it replaces, and harness.CheckScrubbedRead what it refuses
// to write.
//
// Usage:
//
//	aws-read-scrub --read <recorded read> --state <tfstate> --out <dir>
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/redscaresu/infrafactory/internal/harness"
)

func main() {
	read := flag.String("read", "", "the recorded read that rehearse read wrote")
	state := flag.String("state", "", "the kept pre-destroy tfstate")
	out := flag.String("out", "", "the directory to write the scrubbed files into")
	flag.Parse()
	if *read == "" || *state == "" || *out == "" || flag.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "usage: aws-read-scrub --read <recorded read> --state <tfstate> --out <dir>")
		os.Exit(2)
	}
	if err := harness.WriteScrubbedAWSRead(*read, *state, *out); err != nil {
		fmt.Fprintln(os.Stderr, "aws-read-scrub:", err)
		os.Exit(1)
	}
}
