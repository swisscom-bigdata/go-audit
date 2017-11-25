package main

import (
	"bytes"
	"encoding/binary"
	"log"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestNetlinkClientKeepConnection(t *testing.T) {
	n := makeNelinkClient(t)
	defer syscall.Close(n.fd)

	n.KeepConnection()
	msg, err := n.Receive()
	assert.NoError(t, err)

	expectedData := []byte{4, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(expectedData[12:16], uint32(os.Getpid()))

	assert.Equal(t, uint16(1001), msg.Header.Type, "Header.Type mismatch")
	assert.Equal(t, uint16(5), msg.Header.Flags, "Header.Flags mismatch")
	assert.Equal(t, uint32(1), msg.Header.Seq, "Header.Seq mismatch")
	assert.Equal(t, uint32(56), msg.Header.Len, "Packet size is wrong - this test is brittle though")
	assert.EqualValues(t, msg.Data[:40], expectedData, "data was wrong")

	// Make sure we get errors printed
	lb := hookLogger()
	defer resetLogger()
	syscall.Close(n.fd)
	n.KeepConnection()
	assert.True(t, strings.Contains(lb.String(), "error occurred while trying to keep the connection"), "Figured we would have an error")
}

func TestNetlinkClientSendReceive(t *testing.T) {
	var err error
	var msg *syscall.NetlinkMessage

	// Build our client
	n := makeNelinkClient(t)
	defer syscall.Close(n.fd)

	// Make sure we can encode/decode properly
	payload := &AuditStatusPayload{
		Mask:    4,
		Enabled: 1,
		Pid:     uint32(1006),
	}

	packet := &NetlinkPacket{
		Type:  uint16(1001),
		Flags: syscall.NLM_F_REQUEST | syscall.NLM_F_ACK,
		Pid:   uint32(1006),
	}

	msg = sendReceive(t, n, packet, payload)

	assert.Equal(t, uint32(1006), msg.Header.Pid, "Header.Pid mismatch")
	assert.Equal(t, packet.Type, msg.Header.Type, "Header.Type mismatch")
	assert.Equal(t, packet.Flags, msg.Header.Flags, "Header.Flags mismatch")
	assert.Equal(t, uint32(1), msg.Header.Seq, "Header.Seq mismatch")
	assert.Equal(t, uint32(56), msg.Header.Len, "Packet size is wrong - this test is brittle though")
	assert.EqualValues(t, msg.Data[:40], []byte{4, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 238, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, "data was wrong")

	// Make sure sequences numbers increment on our side
	msg = sendReceive(t, n, packet, payload)
	assert.Equal(t, uint32(2), msg.Header.Seq, "Header.Seq did not increment")

	// Make sure 0 length packets result in an error
	syscall.Sendto(n.fd, []byte{}, 0, n.address)
	_, err = n.Receive()
	assert.Equal(t, "got a 0 length packet", err.Error(), "Error was incorrect")

	// Make sure we get errors from sendto back
	syscall.Close(n.fd)
	err = n.Send(packet, payload)
	assert.Equal(t, "bad file descriptor", err.Error(), "Error was incorrect")

	// Make sure we get errors from recvfrom back
	n.fd = 0
	_, err = n.Receive()
	assert.Equal(t, "socket operation on non-socket", err.Error(), "Error was incorrect")
}

func TestNewNetlinkClient(t *testing.T) {
	lb := hookLogger()
	defer resetLogger()

	n, err := NewNetlinkClient(1024)
	assert.NoError(t, err)
	if n == nil {
		t.Fatal("Expected a netlink client but had an error instead!")
	} else {
		assert.True(t, (n.fd > 0), "No file descriptor")
		assert.True(t, (n.address != nil), "Address was nil")
		assert.Equal(t, uint32(0), n.seq, "Seq should start at 0")
		assert.True(t, MAX_AUDIT_MESSAGE_LENGTH >= len(n.buf), "Client buffer is too small")

		assert.Equal(t, "Socket receive buffer size: ", lb.String()[:28], "Expected some nice log lines")
		assert.Equal(t, "", elb.String(), "Did not expect any error messages")
	}
}

// Helper to make a client listening on a unix secket
func makeNelinkClient(t *testing.T) *NetlinkClient {
	os.Remove("go-audit.test.sock")
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_RAW, 0)
	if err != nil {
		t.Fatal("Could not create a socket:", err)
	}

	n := &NetlinkClient{
		fd:      fd,
		address: &syscall.SockaddrUnix{Name: "go-audit.test.sock"},
		buf:     make([]byte, MAX_AUDIT_MESSAGE_LENGTH),
	}

	if err = syscall.Bind(fd, n.address); err != nil {
		syscall.Close(fd)
		t.Fatal("Could not bind to netlink socket:", err)
	}

	return n
}

// Helper to send and then receive a message with the netlink client
func sendReceive(t *testing.T, n *NetlinkClient, packet *NetlinkPacket, payload *AuditStatusPayload) *syscall.NetlinkMessage {
	err := n.Send(packet, payload)
	assert.NoError(t, err)

	msg, err := n.Receive()
	assert.NoError(t, err)

	return msg
}

// Resets global loggers
func resetLogger() {
	log.SetOutput(os.Stderr)
}

// Hooks the global loggers writers so you can assert their contents
func hookLogger() *bytes.Buffer {
	lb := &bytes.Buffer{}
	logrus.SetOutput(lb)
	return lb
}
