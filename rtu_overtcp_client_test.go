package modbus

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

func TestRTUTCPEncoding(t *testing.T) {
	encoder := RTUTCPClientHandler{}
	encoder.SlaveId = 0x01

	pdu := ProtocolDataUnit{}
	pdu.FunctionCode = 0x03
	pdu.Data = []byte{0x50, 0x00, 0x00, 0x18}

	adu, err := encoder.Encode(&pdu)
	if err != nil {
		t.Fatal(err)
	}
	expected := []byte{0x01, 0x03, 0x50, 0x00, 0x00, 0x18, 0x54, 0xC0}
	if !bytes.Equal(expected, adu) {
		t.Fatalf("adu: expected %v, actual %v", expected, adu)
	}
}

func TestRTUTCPDecoding(t *testing.T) {
	decoder := RTUTCPClientHandler{}
	adu := []byte{0x01, 0x10, 0x8A, 0x00, 0x00, 0x03, 0xAA, 0x10}

	pdu, err := decoder.Decode(adu)
	if err != nil {
		t.Fatal(err)
	}

	if 16 != pdu.FunctionCode {
		t.Fatalf("Function code: expected %v, actual %v", 16, pdu.FunctionCode)
	}
	expected := []byte{0x8A, 0x00, 0x00, 0x03}
	if !bytes.Equal(expected, pdu.Data) {
		t.Fatalf("Data: expected %v, actual %v", expected, pdu.Data)
	}
}

func TestRTUTCPTransporter(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_, err = io.Copy(conn, conn)
		if err != nil {
			t.Error(err)
			return
		}
	}()
	client := &RTUTCPClientHandler{
		tcpTransporter: tcpTransporter{
			Address:     ln.Addr().String(),
			Timeout:     1 * time.Second,
			IdleTimeout: 100 * time.Millisecond,
		},
	}
	req := []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x02, 0xC4, 0x0B}
	rsp, err := client.Send(req)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(req, rsp) {
		t.Fatalf("unexpected response: %x", rsp)
	}
	time.Sleep(150 * time.Millisecond)
	if client.conn != nil {
		t.Fatalf("connection is not closed: %+v", client.conn)
	}
}

func BenchmarkRTUTCPEncoder(b *testing.B) {
	encoder := RTUTCPClientHandler{
		rtuPackager: rtuPackager{
			SlaveId: 10,
		},
	}
	pdu := ProtocolDataUnit{
		FunctionCode: 1,
		Data:         []byte{2, 3, 4, 5, 6, 7, 8, 9},
	}
	for i := 0; i < b.N; i++ {
		_, err := encoder.Encode(&pdu)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRTUTCPDecoder(b *testing.B) {
	decoder := RTUTCPClientHandler{
		rtuPackager: rtuPackager{
			SlaveId: 10,
		},
	}
	adu := []byte{0x01, 0x10, 0x8A, 0x00, 0x00, 0x03, 0xAA, 0x10}
	for i := 0; i < b.N; i++ {
		_, err := decoder.Decode(adu)
		if err != nil {
			b.Fatal(err)
		}
	}
}
