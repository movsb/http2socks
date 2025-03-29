package http2socks

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/netip"
)

type HelloRequest struct {
	Version      uint8
	NumberOfAuth uint8
	AuthMethods  [16]byte
}

var errVersion = fmt.Errorf(`version error`)

func (m *HelloRequest) Decode(r io.Reader) error {
	var buf [128]byte
	if _, err := io.ReadFull(r, buf[:2]); err != nil {
		return err
	}
	m.Version = buf[0]
	m.NumberOfAuth = buf[1]
	if m.Version != 0x05 {
		return errVersion
	}
	if m.NumberOfAuth <= 0 || m.NumberOfAuth > uint8(len(m.AuthMethods)) {
		return fmt.Errorf(`too many auth`)
	}
	if _, err := io.ReadFull(r, buf[:m.NumberOfAuth]); err != nil {
		return fmt.Errorf(`error reading auth methods`)
	}
	copy(m.AuthMethods[:], buf[:m.NumberOfAuth])
	return nil
}

func writeBytes(w io.Writer, b []byte) error {
	if nw, err := w.Write(b); nw != len(b) || err != nil {
		return errors.Join(fmt.Errorf(`nw:%d vs n:%d`, nw, len(b)), err)
	}
	return nil
}

func (m *HelloRequest) Encode(w io.Writer) error {
	var buf [128]byte
	buf[0] = m.Version
	buf[1] = m.NumberOfAuth
	copy(buf[2:], m.AuthMethods[:m.NumberOfAuth])
	return writeBytes(w, buf[:1+1+m.NumberOfAuth])
}

type HelloResponse struct {
	Version    uint8
	ChosenAuth uint8
}

func (m *HelloResponse) Decode(r io.Reader) error {
	var buf [128]byte
	if _, err := io.ReadFull(r, buf[:2]); err != nil {
		return err
	}
	m.Version = buf[0]
	m.ChosenAuth = buf[1]
	if m.Version != 0x05 {
		return errVersion
	}
	return nil
}

func (m *HelloResponse) Encode(w io.Writer) error {
	var buf [128]byte
	buf[0] = m.Version
	buf[1] = m.ChosenAuth
	return writeBytes(w, buf[:2])
}

type Command uint8

const (
	TCPStream  Command = 0x01
	TCPBinding Command = 0x02
	UDPBinding Command = 0x03
)

func (c Command) String() string {
	switch c {
	case TCPStream:
		return `TCPStream`
	case TCPBinding:
		return `TCPBinding`
	case UDPBinding:
		return `UDPBinding`
	}
	return `UnknownCommand`
}

type AddressType uint8

const (
	IPv4   AddressType = 0x01
	Domain AddressType = 0x03
	IPv6   AddressType = 0x04
)

func (a AddressType) String() string {
	switch a {
	case IPv4:
		return `IPv4`
	case IPv6:
		return `IPv6`
	case Domain:
		return `Domain`
	default:
		return `UnknownAddressType`
	}
}

type ConnectionRequest struct {
	Version uint8
	Command Command
	Address struct {
		Type   AddressType
		Domain string
		IP     netip.Addr
		Port   uint16
	}
}

func (m *ConnectionRequest) Decode(r io.Reader) error {
	var buf [256]byte
	if _, err := io.ReadFull(r, buf[:2]); err != nil {
		return err
	}
	m.Version = buf[0]
	if m.Version != 0x05 {
		return errVersion
	}
	m.Command = Command(buf[1])
	if m.Command.String() == `UnknownCommand` {
		return fmt.Errorf(`unknown command`)
	}
	if _, err := io.ReadFull(r, buf[:1]); err != nil {
		return err
	}
	if buf[0] != 0x00 {
		return fmt.Errorf(`error reserved`)
	}
	if _, err := io.ReadFull(r, buf[:1]); err != nil {
		return err
	}
	m.Address.Type = AddressType(buf[0])
	switch m.Address.Type {
	case IPv4:
		if _, err := io.ReadFull(r, buf[:4]); err != nil {
			return err
		}
		var ip [4]byte
		copy(ip[:], buf[:4])
		m.Address.IP = netip.AddrFrom4(ip)
	case IPv6:
		if _, err := io.ReadFull(r, buf[:16]); err != nil {
			return err
		}
		var ip [16]byte
		copy(ip[:], buf[:16])
		m.Address.IP = netip.AddrFrom16(ip)
	case Domain:
		if _, err := io.ReadFull(r, buf[:1]); err != nil {
			return err
		}
		n := buf[0]
		if n < 1 || int(n) > 255 {
			return fmt.Errorf(`domain too long`)
		}
		if _, err := io.ReadFull(r, buf[:n]); err != nil {
			return err
		}
		m.Address.Domain = string(buf[:n])
	default:
		return fmt.Errorf(`unknown address type: %v`, m.Address.Type)
	}
	if _, err := io.ReadFull(r, buf[:2]); err != nil {
		return err
	}
	m.Address.Port = uint16(buf[0])<<8 + uint16(buf[1])
	return nil
}

func (m *ConnectionRequest) Encode(w io.Writer) error {
	var buf [512]byte
	buf[0] = m.Version
	buf[1] = byte(m.Command)
	buf[2] = 0
	buf[3] = byte(m.Address.Type)
	n := 4
	switch m.Address.Type {
	case IPv4:
		b := m.Address.IP.As4()
		n += copy(buf[n:], b[:])
	case IPv6:
		b := m.Address.IP.As16()
		n += copy(buf[n:], b[:])
	case Domain:
		buf[n:][0] = uint8(len(m.Address.Domain))
		n += 1
		n += copy(buf[n:], []byte(m.Address.Domain))
	}
	buf[n:][0] = byte(m.Address.Port >> 8)
	buf[n:][1] = byte(m.Address.Port)
	n += 2
	return writeBytes(w, buf[:n])
}

/*
0x00: request granted
0x01: general failure
0x02: connection not allowed by ruleset
0x03: network unreachable
0x04: host unreachable
0x05: connection refused by destination host
0x06: TTL expired
0x07: command not supported / protocol error
0x08: address type not supported
*/
type Status uint8

const (
	StatusRequestGranted                Status = 0x00
	StatusGeneralFailure                Status = 0x01
	StatusConnectionNotAllowedByRuleSet Status = 0x02
	StatusNetworkUnreachable            Status = 0x03
	StatusHostUnreachable               Status = 0x04
	StatusConnectionRefused             Status = 0x05
	StatusTTLExpired                    Status = 0x06
	StatusCommandNotSupported           Status = 0x07
	StatusAddressTypeNotSupported       Status = 0x08
)

func (s Status) String() string {
	switch s {
	case StatusRequestGranted:
		return "StatusRequestGranted"
	case StatusGeneralFailure:
		return "StatusGeneralFailure"
	case StatusConnectionNotAllowedByRuleSet:
		return "StatusConnectionNotAllowedByRuleSet"
	case StatusNetworkUnreachable:
		return "StatusNetworkUnreachable"
	case StatusHostUnreachable:
		return "StatusHostUnreachable"
	case StatusConnectionRefused:
		return "StatusConnectionRefused"
	case StatusTTLExpired:
		return "StatusTTLExpired"
	case StatusCommandNotSupported:
		return "StatusCommandNotSupported"
	case StatusAddressTypeNotSupported:
		return "StatusAddressTypeNotSupported"
	}
	return "UnknownStatus"
}

type ConnectionResponse struct {
	Version uint8
	Status  Status
	Address struct {
		Type AddressType
		IP   netip.Addr
		Port uint16
	}
}

func (m *ConnectionResponse) Decode(r io.Reader) error {
	var buf [256]byte
	if _, err := io.ReadFull(r, buf[:2]); err != nil {
		return err
	}
	m.Version = buf[0]
	if m.Version != 0x05 {
		return errVersion
	}
	m.Status = Status(buf[1])
	if m.Status.String() == `UnknownStatus` {
		return fmt.Errorf(`unknown status`)
	}
	if _, err := io.ReadFull(r, buf[:1]); err != nil {
		return err
	}
	if buf[0] != 0x00 {
		return fmt.Errorf(`error reserved`)
	}
	if _, err := io.ReadFull(r, buf[:1]); err != nil {
		return err
	}
	m.Address.Type = AddressType(buf[0])
	switch m.Address.Type {
	case IPv4:
		if _, err := io.ReadFull(r, buf[:4]); err != nil {
			return err
		}
		var ip [4]byte
		copy(ip[:], buf[:4])
		m.Address.IP = netip.AddrFrom4(ip)
	case IPv6:
		if _, err := io.ReadFull(r, buf[:16]); err != nil {
			return err
		}
		var ip [16]byte
		copy(ip[:], buf[:16])
		m.Address.IP = netip.AddrFrom16(ip)
	default:
		return fmt.Errorf(`unknown address type: %v`, m.Address.Type)
	}
	if _, err := io.ReadFull(r, buf[:2]); err != nil {
		return err
	}
	m.Address.Port = uint16(buf[0])<<8 + uint16(buf[1])
	return nil
}

func (m *ConnectionResponse) Encode(w io.Writer) error {
	var buf [512]byte
	buf[0] = m.Version
	buf[1] = byte(m.Status)
	buf[2] = 0
	buf[3] = byte(m.Address.Type)
	n := 4
	switch m.Address.Type {
	case IPv4:
		b := m.Address.IP.As4()
		n += copy(buf[n:], b[:])
	case IPv6:
		b := m.Address.IP.As16()
		n += copy(buf[n:], b[:])
	}
	buf[n:][0] = byte(m.Address.Port >> 8)
	buf[n:][1] = byte(m.Address.Port)
	n += 2
	return writeBytes(w, buf[:n])
}

func Logger(local, remote io.ReadWriter) {
	var helloRequest HelloRequest
	if err := helloRequest.Decode(local); err != nil {
		log.Println(err)
		return
	}
	if err := helloRequest.Encode(remote); err != nil {
		log.Println(err)
		return
	}

	var helloResponse HelloResponse
	if err := helloResponse.Decode(remote); err != nil {
		log.Println(err)
		return
	}
	if err := helloResponse.Encode(local); err != nil {
		log.Println(err)
		return
	}

	var connectionRequest ConnectionRequest
	if err := connectionRequest.Decode(local); err != nil {
		log.Println(err)
		return
	}
	if err := connectionRequest.Encode(remote); err != nil {
		log.Println(err)
		return
	}

	var connectionResponse ConnectionResponse
	if err := connectionResponse.Decode(remote); err != nil {
		log.Println(err)
		return
	}
	if err := connectionResponse.Encode(local); err != nil {
		log.Println(err)
		return
	}

	log.Println(`Connection:`, connectionRequest, connectionResponse)
}
