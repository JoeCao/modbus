package main

import (
	"fmt"
	"github.com/goburrow/modbus"
	"log"
	"os"
	"time"
)

const (
	rtu_tcpDevice = "127.0.0.1:502"
)

func RTUTCPRead() error {
	handler := modbus.NewRTUTCPClientHandler(rtu_tcpDevice)
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
