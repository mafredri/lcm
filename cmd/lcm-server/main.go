package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"

	"github.com/mafredri/lcm"
	"github.com/mafredri/lcm/stream"
	pb "github.com/mafredri/lcm/stream"

	"google.golang.org/grpc"
)

func main() {
	bind := flag.String("bind", "", "Bind to interface")
	port := flag.Int("port", 9999, "Port to listen on")
	flag.Parse()

	log.Default().SetFlags(log.LstdFlags | log.Lmicroseconds)

	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", *bind, *port))
	if err != nil {
		panic(err)
	}

	s, err := lcm.Open(lcm.DefaultTTY)
	if err != nil {
		panic(err)
	}
	defer s.Close()

	grpcSrv := grpc.NewServer()
	pb.RegisterLcmServer(grpcSrv, newServer(s))
	log.Fatal(grpcSrv.Serve(lis))
}

type lcmServer struct {
	stream.LcmServer
	m *lcm.LCM
}

func (srv *lcmServer) recvStream(stream pb.Lcm_StreamServer) error {
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		err = srv.m.Write(in.GetData())
		if err != nil {
			return err
		}
	}
}

func (srv *lcmServer) sendStream(stream pb.Lcm_StreamServer) error {
	for {
		m := srv.m.Read()
		err := stream.Send(&pb.Message{Data: m})
		if err != nil {
			return err
		}
	}
}

func (srv *lcmServer) Stream(stream pb.Lcm_StreamServer) error {
	log.Println("Client connected to stream")
	errc := make(chan error, 2)
	go func() { errc <- srv.recvStream(stream) }()
	go func() { errc <- srv.sendStream(stream) }()
	err := <-errc
	log.Printf("Client disconnected from stream: %v", err)
	return err
}

func newServer(m *lcm.LCM) *lcmServer {
	return &lcmServer{
		m: m,
	}
}
