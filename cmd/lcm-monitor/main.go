/*

lcm-monitor intercepts the communication between the ASUSTOR LCD daemon (lcmd)
and the LCD display and saves the input (from LCD) and output (from lcmd) to
files.

The socat unix command must be installed on the target system.

Usage:
	lcm-monitor -out output.txt

*/
package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pkg/term"
)

const (
	ttyS1 = "/dev/ttyS1" // Virtual, managed by socat.
	ttyV1 = "/dev/ttyV1" // Actual, renamed ttyS1.
)

func main() {
	baud := flag.Int("baud", 115200, "baud rate")
	out := flag.String("out", "", "output file")
	socat := flag.String("socat", "/usr/bin/socat", "socat binary")
	flag.Parse()

	if err := run(*baud, *out, *socat); err != nil {
		panic(err)
	}
}

func run(baud int, outfile, socatBin string) error {
	if outfile == "" {
		return errors.New("out must be set")
	}

	if _, err := os.Stat(ttyS1); os.IsExist(err) {
		os.Rename(ttyS1, ttyV1)
	}

	s, err := term.Open(ttyV1, term.Speed(baud), term.RawMode)
	if err != nil {
		return err
	}
	defer s.Close()

	socat := exec.Command(socatBin, "-", fmt.Sprintf("PTY,link=%s,raw,echo=0", ttyS1))
	socat.Env = append(socat.Env, fmt.Sprintf("LD_LIBRARY_PATH=%s", filepath.Dir(socatBin)))
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

	out, err := os.Create(outfile)
	if err != nil {
		return err
	}
	defer out.Close()

	errc := make(chan error, 1)
	go func() { errc <- tee(s, stdin, " IN", out) }()
	go func() { errc <- tee(stdout, s, "OUT", out) }()
	go func() { errc <- socat.Wait() }()

	return <-errc
}

func tee(r io.Reader, w io.Writer, id string, out io.Writer) error {
	var buf bytes.Buffer
	rr := bufio.NewReader(io.TeeReader(r, w))
	// t := time.Now()
	for {
		b, err := rr.ReadByte()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		// if b == 0x4D || b == 0x53 {
		// 	if buf.Len() > 0 {
		// 		s := hex.EncodeToString(buf.Bytes())
		// 		ts := t.Format("15:04:05.999999999")
		// 		for len(ts) < 18 {
		// 			ts += "0"
		// 		}
		// 		_, err = fmt.Fprintf(out, "%s[%s]: %s (%s)\n", ts, id, s, buf.String())
		// 		if err != nil {
		// 			return err
		// 		}
		// 		buf.Reset()
		// 	}
		// 	t = time.Now()
		// }
		buf.WriteByte(b)
		buf.WriteTo(out)
		buf.Reset()
	}
}
