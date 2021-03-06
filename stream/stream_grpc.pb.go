// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

package stream

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// LcmClient is the client API for Lcm service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type LcmClient interface {
	Stream(ctx context.Context, opts ...grpc.CallOption) (Lcm_StreamClient, error)
}

type lcmClient struct {
	cc grpc.ClientConnInterface
}

func NewLcmClient(cc grpc.ClientConnInterface) LcmClient {
	return &lcmClient{cc}
}

func (c *lcmClient) Stream(ctx context.Context, opts ...grpc.CallOption) (Lcm_StreamClient, error) {
	stream, err := c.cc.NewStream(ctx, &Lcm_ServiceDesc.Streams[0], "/stream.Lcm/Stream", opts...)
	if err != nil {
		return nil, err
	}
	x := &lcmStreamClient{stream}
	return x, nil
}

type Lcm_StreamClient interface {
	Send(*Message) error
	Recv() (*Message, error)
	grpc.ClientStream
}

type lcmStreamClient struct {
	grpc.ClientStream
}

func (x *lcmStreamClient) Send(m *Message) error {
	return x.ClientStream.SendMsg(m)
}

func (x *lcmStreamClient) Recv() (*Message, error) {
	m := new(Message)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// LcmServer is the server API for Lcm service.
// All implementations must embed UnimplementedLcmServer
// for forward compatibility
type LcmServer interface {
	Stream(Lcm_StreamServer) error
	mustEmbedUnimplementedLcmServer()
}

// UnimplementedLcmServer must be embedded to have forward compatible implementations.
type UnimplementedLcmServer struct {
}

func (UnimplementedLcmServer) Stream(Lcm_StreamServer) error {
	return status.Errorf(codes.Unimplemented, "method Stream not implemented")
}
func (UnimplementedLcmServer) mustEmbedUnimplementedLcmServer() {}

// UnsafeLcmServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to LcmServer will
// result in compilation errors.
type UnsafeLcmServer interface {
	mustEmbedUnimplementedLcmServer()
}

func RegisterLcmServer(s grpc.ServiceRegistrar, srv LcmServer) {
	s.RegisterService(&Lcm_ServiceDesc, srv)
}

func _Lcm_Stream_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(LcmServer).Stream(&lcmStreamServer{stream})
}

type Lcm_StreamServer interface {
	Send(*Message) error
	Recv() (*Message, error)
	grpc.ServerStream
}

type lcmStreamServer struct {
	grpc.ServerStream
}

func (x *lcmStreamServer) Send(m *Message) error {
	return x.ServerStream.SendMsg(m)
}

func (x *lcmStreamServer) Recv() (*Message, error) {
	m := new(Message)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// Lcm_ServiceDesc is the grpc.ServiceDesc for Lcm service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Lcm_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "stream.Lcm",
	HandlerType: (*LcmServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Stream",
			Handler:       _Lcm_Stream_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "stream/stream.proto",
}
