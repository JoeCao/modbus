package modbus

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// RTUTCPClientHandler implements Packager and Transporter interface.
type RTUTCPClientHandler struct {
	rtuPackager
	rtuTcpTransporter
}

// NewRTUTCPClientHandler allocates and initializes a RTUTCPClientHandler.
func NewRTUTCPClientHandler(address string) *RTUTCPClientHandler {
	handler := &RTUTCPClientHandler{}
	handler.Address = address
	handler.Timeout = tcpTimeout
	handler.IdleTimeout = tcpIdleTimeout
	return handler
}

func RTUTCPClient(address string) Client {
	handler := NewRTUTCPClientHandler(address)
	return NewClient(handler)
}

// No need to redefine rtutcpPackager as rtuPackager can be reused

func (mb *RTUTCPClientHandler) Encode(pdu *ProtocolDataUnit) (adu []byte, err error) {
	length := len(pdu.Data) + 4
	if length > rtuMaxSize {
		err = fmt.Errorf("modbus: length of data '%v' must not be bigger than '%v'", length, rtuMaxSize)
		return
	}
	adu = make([]byte, length)

	adu[0] = mb.SlaveId
	adu[1] = pdu.FunctionCode
	copy(adu[2:], pdu.Data)

	// Append crc
	var crc crc
	crc.reset().pushBytes(adu[0 : length-2])
	checksum := crc.value()

	adu[length-1] = byte(checksum >> 8)
	adu[length-2] = byte(checksum)
	return
}

func (mb *RTUTCPClientHandler) Verify(aduRequest []byte, aduResponse []byte) (err error) {
	length := len(aduResponse)
	// Minimum size (including address, function and CRC)
	if length < rtuMinSize {
		err = fmt.Errorf("modbus: response length '%v' does not meet minimum '%v'", length, rtuMinSize)
		return
	}
	// Slave address must match
	if aduResponse[0] != aduRequest[0] {
		err = fmt.Errorf("modbus: response slave id '%v' does not match request '%v'", aduResponse[0], aduRequest[0])
		return
	}
	return
}

func (mb *RTUTCPClientHandler) Decode(adu []byte) (pdu *ProtocolDataUnit, err error) {
	length := len(adu)
	// Calculate checksum
	var crc crc
	crc.reset().pushBytes(adu[0 : length-2])
	checksum := uint16(adu[length-1])<<8 | uint16(adu[length-2])
	if checksum != crc.value() {
		err = fmt.Errorf("modbus: response crc '%v' does not match expected '%v'", checksum, crc.value())
		return
	}
	// Function code & data
	pdu = &ProtocolDataUnit{}
	pdu.FunctionCode = adu[1]
	pdu.Data = adu[2 : length-2]
	return
}

// rtuTcpTransporter implements Transporter interface.
type rtuTcpTransporter struct {
	// Connect string
	Address string
	// Connect & Read timeout
	Timeout time.Duration
	// Idle timeout to close the connection
	IdleTimeout time.Duration
	// Transmission logger
	Logger *log.Logger

	// TCP connection
	mu           sync.Mutex
	conn         net.Conn
	closeTimer   *time.Timer
	lastActivity time.Time
}

func (mb *rtuTcpTransporter) Send(aduRequest []byte) (aduResponse []byte, err error) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	// Ensure connection is established
	if err = mb.connect(); err != nil {
		return
	}
	// Start the timer to close when idle
	mb.lastActivity = time.Now()
	mb.startCloseTimer()
	// Set write and read timeout
	var timeout time.Time
	if mb.Timeout > 0 {
		timeout = mb.lastActivity.Add(mb.Timeout)
	}
	if err = mb.conn.SetDeadline(timeout); err != nil {
		return
	}
	// Send the request
	mb.logf("modbus: sending % x\n", aduRequest)
	if _, err = mb.conn.Write(aduRequest); err != nil {
		return
	}

	function := aduRequest[1]
	functionFail := aduRequest[1] & 0x80
	bytesToRead := calculateResponseLength(aduRequest)
	time.Sleep(mb.calculateDelay(len(aduRequest) + bytesToRead))

	var n int
	var n1 int
	var data [rtuMaxSize]byte
	// Read the minimum length first
	n, err = io.ReadAtLeast(mb.conn, data[:], rtuMinSize)
	if err != nil {
		return
	}
	// If the function is correct
	if data[1] == function {
		// Read the rest of the bytes
		if n < bytesToRead {
			if bytesToRead > rtuMinSize && bytesToRead <= rtuMaxSize {
				if bytesToRead > n {
					n1, err = io.ReadFull(mb.conn, data[n:bytesToRead])
					n += n1
				}
			}
		}
	} else if data[1] == functionFail {
		// For error response, read 5 bytes
		if n < rtuExceptionSize {
			n1, err = io.ReadFull(mb.conn, data[n:rtuExceptionSize])
		}
		n += n1
	}

	if err != nil {
		return
	}

	// Verify CRC
	crc := calculateCRC(data[:n-2])
	if binary.LittleEndian.Uint16(data[n-2:]) != crc {
		err = fmt.Errorf("modbus: CRC check failed, expected '%x', got '%x'", crc, data[n-2:])
		return
	}
	aduResponse = data[:n]
	mb.logf("modbus: received % x\n", aduResponse)
	return
}

// calculateCRC calculates the Modbus RTU frame's CRC
func calculateCRC(data []byte) uint16 {
	var crc uint16 = 0xFFFF
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if (crc & 1) != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}

// 计算请求和响应之间的延迟时间
func (mb *rtuTcpTransporter) calculateDelay(byteCount int) time.Duration {
	// 具体实现需要根据通信速度和数据长度来计算延迟时间
	// 此处仅为示例，实际需要根据通信环境进行调整
	return time.Duration(byteCount) * time.Millisecond
}

// Connect establishes a new connection to the address in Address.
// Connect and Close are exported so that multiple requests can be done with one session
func (mb *rtuTcpTransporter) Connect() error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	return mb.connect()
}

func (mb *rtuTcpTransporter) connect() error {
	if mb.conn == nil {
		dialer := net.Dialer{Timeout: mb.Timeout}
		conn, err := dialer.Dial("tcp", mb.Address)
		if err != nil {
			return err
		}
		mb.conn = conn
	}
	return nil
}

func (mb *rtuTcpTransporter) startCloseTimer() {
	if mb.IdleTimeout <= 0 {
		return
	}
	if mb.closeTimer == nil {
		mb.closeTimer = time.AfterFunc(mb.IdleTimeout, mb.closeIdle)
	} else {
		mb.closeTimer.Reset(mb.IdleTimeout)
	}
}

// Close closes current connection.
func (mb *rtuTcpTransporter) Close() error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	return mb.close()
}

// flush flushes pending data in the connection,
// returns io.EOF if connection is closed.
func (mb *rtuTcpTransporter) flush(b []byte) (err error) {
	if err = mb.conn.SetReadDeadline(time.Now()); err != nil {
		return
	}
	// Timeout setting will be reset when reading
	if _, err = mb.conn.Read(b); err != nil {
		// Ignore timeout error
		if netError, ok := err.(net.Error); ok && netError.Timeout() {
			err = nil
		}
	}
	return
}

func (mb *rtuTcpTransporter) logf(format string, v ...interface{}) {
	if mb.Logger != nil {
		mb.Logger.Printf(format, v...)
	}
}

// closeLocked closes current connection. Caller must hold the mutex before calling this method.
func (mb *rtuTcpTransporter) close() (err error) {
	if mb.conn != nil {
		err = mb.conn.Close()
		mb.conn = nil
	}
	return
}

// closeIdle closes the connection if last activity is passed behind IdleTimeout.
func (mb *rtuTcpTransporter) closeIdle() {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	if mb.IdleTimeout <= 0 {
		return
	}
	idle := time.Now().Sub(mb.lastActivity)
	if idle >= mb.IdleTimeout {
		mb.logf("modbus: closing connection due to idle timeout: %v", idle)
		mb.close()
	}
}
