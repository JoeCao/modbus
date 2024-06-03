package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/goburrow/modbus"
	"log"
	"os"
	"time"
)

const (
	my_tcpDevice = "192.168.1.95:502"
)

func main() {
	err := RTUTCPRead()
	if err != nil {
		log.Fatal(err)
	}
}

func Read() error {
	handler := modbus.NewTCPClientHandler(my_tcpDevice)
	handler.Timeout = 5 * time.Second
	handler.SlaveId = 1
	handler.Logger = log.New(os.Stdout, "tcp: ", log.LstdFlags)
	err := handler.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect handler: %w", err)
	}
	defer handler.Close()
	client := modbus.NewClient(handler)
	results, err := client.ReadHoldingRegisters(2, 1)
	if err != nil || results == nil {
		return fmt.Errorf("failed to read holding registers: %w", err)
	}
	fmt.Println(results)
	// Convert the result to int16
	if len(results) < 2 {
		return fmt.Errorf("unexpected result length: %d", len(results))
	}
	// 创建 Reader
	r := bytes.NewReader(results)
	// 读取一个 uint16
	var u uint16
	if err := binary.Read(r, binary.BigEndian, &u); err != nil {
		fmt.Println("binary.Read failed:", err)
	}
	fmt.Printf("uint16: %d\n", u)
	//value := int16(binary.BigEndian.Uint16(results))
	//fmt.Println("Converted int16 value:", value)

	return nil
}

func RTUTCPRead() error {
	handler := modbus.NewRTUTCPClientHandler(my_tcpDevice)
	handler.Timeout = 5 * time.Second
	handler.SlaveId = 1
	handler.Logger = log.New(os.Stdout, "tcp: ", log.LstdFlags)
	err := handler.Connect()
	defer handler.Close()
	client := modbus.NewClient(handler)
	results, err := client.ReadHoldingRegisters(1, 1)
	if err != nil || results == nil {
		return fmt.Errorf("failed to read holding registers: %w", err)
	}
	fmt.Println(results)
	return nil
}
