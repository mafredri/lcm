package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/mafredri/lcm"
	"github.com/mafredri/lcm/stream"

	"google.golang.org/grpc"
)

func main() {
	addr := flag.String("addr", "", "Server address")
	port := flag.Int("port", 9999, "Server port")
	flag.Parse()

	conn, err := grpc.Dial(fmt.Sprintf("%s:%d", *addr, *port), grpc.WithInsecure())
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	client := stream.NewLcmClient(conn)

	for {
		s, err := client.Stream(context.TODO())
		if err != nil {
			panic(err)
		}
		err = watch(s)
		if err != nil {
			log.Println("watch", err)
		}
		s.CloseSend()
	}
}

func watch(s stream.Lcm_StreamClient) error {
	go func() {
		for {
			for _, d := range [][]byte{
				lcm.DisplayStatus,
				// Write msg
				setDisplay(lcm.DisplayTop, 0, "HELLO"),
				setDisplay(lcm.DisplayBottom, 0, "WORLD"),
				// Clear top.
				setDisplay(lcm.DisplayTop, 0, ""),
				// Test indentation.
				setDisplay(lcm.DisplayTop, 15, "HELLO"),
				setDisplay(lcm.DisplayTop, 14, "HELLO"),
				setDisplay(lcm.DisplayTop, 13, "HELLO"),
				setDisplay(lcm.DisplayTop, 12, "HELLO"),
				setDisplay(lcm.DisplayTop, 11, "HELLO"),
				// Lower case.
				setDisplay(lcm.DisplayTop, 0, "Hello"),
				lcm.DisplayStatus,
				lcm.ClearDisplay,
				lcm.DisplayOff,
				lcm.DisplayOn,
			} {
				log.Printf("Sending message: %#x", d)
				err := s.Send(&stream.Message{Data: d})
				if err != nil {
					log.Printf("Error sending message: %v", err)
					return
				}
				time.Sleep(2000 * time.Millisecond)
			}
		}
	}()

	for {
		m, err := s.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		fmt.Printf("Got (hex): %s\n", hex.EncodeToString(m.Data))
		fmt.Printf("Got (bin): %08b\n", m.Data)
		fmt.Printf("Got (str): %q\n", m.Data)

		if m.Data[0] == lcm.CommandByte && m.Data[2] == 0x80 {
			err = s.Send(&stream.Message{Data: lcm.ButtonReply})
			if err != nil {
				log.Printf("Error sending button reply: %v", err)
			}
			b := lcm.Button(m.Data[3])
			switch b {
			case lcm.Up:
			case lcm.Down:
			case lcm.Back:
			case lcm.Enter:
			}

			log.Printf("Button press: %s", b)
		}
	}
}

func setDisplay(line lcm.DisplayLine, indent int, text string) []byte {
	b, _ := lcm.SetDisplay(line, indent, text)
	return b
}
