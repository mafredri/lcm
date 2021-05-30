/*

lcm-monitor intercepts the communication between the ASUSTOR LCD daemon (lcmd)
and the LCD display and saves the input (from LCD) and output (from lcmd) to
files.

The socat unix command must be installed on the target system.

Usage:
	lcm-monitor -in input.txt -out output.txt

*/
package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/tarm/serial"
)

const (
	ttyS1 = "/dev/ttyS1" // Virtual, managed by socat.
	ttyV1 = "/dev/ttyV1" // Actual, renamed ttyS1.
)

func main() {
	out := flag.String("out", "", "output file")
	in := flag.String("in", "", "input file")
	flag.Parse()

	if err := run(*in, *out); err != nil {
		panic(err)
	}
}

func run(infile, outfile string) error {
	if infile == "" || outfile == "" {
		return errors.New("in and out must be set")
	}

	if _, err := os.Stat(ttyS1); os.IsExist(err) {
		os.Rename(ttyS1, ttyV1)
	}

	c := &serial.Config{Name: ttyV1, Baud: 115200}
	s, err := serial.OpenPort(c)
	if err != nil {
		return err
	}
	defer s.Close()

	socat := exec.Command("/root/socat", "-", fmt.Sprintf("PTY,link=%s,raw,echo=0,waitslave", ttyS1))
	stdout, err := socat.StdoutPipe()
	if err != nil {
		return err
	}
	defer stdout.Close()
	stdin, err := socat.StdinPipe()
	if err != nil {
		return err
	}
	defer stdin.Close()

	err = socat.Start()
	if err != nil {
		return err
	}

	errc := make(chan error, 1)
	go func() { errc <- tee(s, stdin, " IN", infile) }()
	go func() { errc <- tee(stdout, s, "OUT", outfile) }()
	go func() { errc <- socat.Wait() }()

	return <-errc
}

func tee(r io.Reader, w io.Writer, id, file string) error {
	out, err := os.Create(file)
	if err != nil {
		return err
	}
	defer out.Close()

	var buf bytes.Buffer
	rr := bufio.NewReader(io.TeeReader(r, w))
	for {
		b, err := rr.ReadByte()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if b == 0xF0 || b == 0xF1 {
			if buf.Len() > 0 {
				s := hex.EncodeToString(buf.Bytes())
				fmt.Fprintf(out, "%s (%s)\n", s, buf.String())
				buf.Reset()
			}
			t := time.Now().Format("15:04:05.999999999")
			for len(t) < 18 {
				t += "0"
			}
			_, err = fmt.Fprintf(out, "%s[%s]: ", t, id)
			if err != nil {
				return err
			}
		}
		buf.WriteByte(b)
	}
}
