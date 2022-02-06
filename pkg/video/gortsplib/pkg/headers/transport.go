package headers

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"nvr/pkg/video/gortsplib/pkg/base"
	"strconv"
	"strings"
)

// TransportProtocol is a transport protocol.
type TransportProtocol int

// standard transport protocols.
const (
	TransportProtocolUDP TransportProtocol = iota
	TransportProtocolTCP
)

// TransportDelivery is a delivery method.
type TransportDelivery int

// TransportMode is a transport mode.
type TransportMode int

const (
	// TransportModePlay is the "play" transport mode.
	TransportModePlay TransportMode = iota

	// TransportModeRecord is the "record" transport mode.
	TransportModeRecord
)

// Transport is a Transport header.
type Transport struct {
	// protocol of the stream
	Protocol TransportProtocol

	// (optional) destination IP
	Destination *net.IP

	// (optional) interleaved frame ids
	InterleavedIDs *[2]int

	// (optional) TTL
	TTL *uint

	// (optional) ports
	Ports *[2]int

	// (optional) client ports
	ClientPorts *[2]int

	// (optional) server ports
	ServerPorts *[2]int

	// (optional) SSRC of the packets of the stream
	SSRC *uint32

	// (optional) mode
	Mode *TransportMode
}

// ErrPortsInvalid invalid ports.
var ErrPortsInvalid = errors.New("invalid ports")

func parsePorts(val string) (*[2]int, error) {
	ports := strings.Split(val, "-")
	if len(ports) == 2 {
		port1, err := strconv.ParseInt(ports[0], 10, 64)
		if err != nil {
			return &[2]int{0, 0}, fmt.Errorf("%w (%v)", ErrPortsInvalid, val)
		}

		port2, err := strconv.ParseInt(ports[1], 10, 64)
		if err != nil {
			return &[2]int{0, 0}, fmt.Errorf("%w (%v)", ErrPortsInvalid, val)
		}

		return &[2]int{int(port1), int(port2)}, nil
	}

	if len(ports) == 1 {
		port1, err := strconv.ParseInt(ports[0], 10, 64)
		if err != nil {
			return &[2]int{0, 0}, fmt.Errorf("%w (%v)", ErrPortsInvalid, val)
		}

		return &[2]int{int(port1), int(port1 + 1)}, nil
	}

	return &[2]int{0, 0}, fmt.Errorf("%w (%v)", ErrPortsInvalid, val)
}

// Transport errors.
var (
	ErrTransportValueMissing       = errors.New("value not provided")
	ErrTransportMultipleValues     = errors.New("value provided multiple times")
	ErrTransportInvalidDestination = errors.New("invalid destination")
	ErrTransportInvalidSSRC        = errors.New("invalid SSRC")
	ErrTransportInvalidMode        = errors.New("invalid transport mode")
	ErrTransportProtocolNotFound   = errors.New("protocol not found")
)

// Read decodes a Transport header.
func (h *Transport) Read(v base.HeaderValue) error { //nolint:funlen,gocognit
	if len(v) == 0 {
		return ErrTransportValueMissing
	}

	if len(v) > 1 {
		return fmt.Errorf("%w (%v)", ErrTransportMultipleValues, v)
	}

	v0 := v[0]

	kvs, err := keyValParse(v0, ';')
	if err != nil {
		return err
	}

	protocolFound := false

	for k, rv := range kvs {
		v := rv

		switch k {
		case "RTP/AVP", "RTP/AVP/UDP":
			h.Protocol = TransportProtocolUDP
			protocolFound = true

		case "RTP/AVP/TCP":
			h.Protocol = TransportProtocolTCP
			protocolFound = true

		case "destination":
			ip := net.ParseIP(v)
			if ip == nil {
				return fmt.Errorf("%w (%v)", ErrTransportInvalidDestination, v)
			}
			h.Destination = &ip

		case "interleaved":
			ports, err := parsePorts(v)
			if err != nil {
				return err
			}
			h.InterleavedIDs = ports

		case "ttl":
			tmp, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return err
			}
			vu := uint(tmp)
			h.TTL = &vu

		case "port":
			ports, err := parsePorts(v)
			if err != nil {
				return err
			}
			h.Ports = ports

		case "client_port":
			ports, err := parsePorts(v)
			if err != nil {
				return err
			}
			h.ClientPorts = ports

		case "server_port":
			ports, err := parsePorts(v)
			if err != nil {
				return err
			}
			h.ServerPorts = ports

		case "ssrc":
			// replace initial spaces
			var b strings.Builder
			b.Grow(len(v))
			i := 0
			for ; i < len(v) && v[i] == ' '; i++ {
				b.WriteByte('0')
			}
			b.WriteString(v[i:])
			v = b.String()

			// add initial zeros
			if (len(v) % 2) != 0 {
				v = "0" + v
			}

			tmp, err := hex.DecodeString(v)
			if err != nil {
				return err
			}

			if len(tmp) > 4 {
				return ErrTransportInvalidSSRC
			}

			var ssrc [4]byte
			copy(ssrc[4-len(tmp):], tmp)

			v := binary.BigEndian.Uint32(ssrc[:])
			h.SSRC = &v

		case "mode":
			str := strings.ToLower(v)
			str = strings.TrimPrefix(str, "\"")
			str = strings.TrimSuffix(str, "\"")

			switch str {
			case "play":
				v := TransportModePlay
				h.Mode = &v

				// receive is an old alias for record, used by ffmpeg with the
				// -listen flag, and by Darwin Streaming Server
			case "record", "receive":
				v := TransportModeRecord
				h.Mode = &v

			default:
				return fmt.Errorf("%w: '%s'", ErrTransportInvalidMode, str)
			}

		default:
			// ignore non-standard keys
		}
	}

	if !protocolFound {
		return fmt.Errorf("%w (%v)", ErrTransportProtocolNotFound, v[0])
	}

	return nil
}

// Write encodes a Transport header.
func (h Transport) Write() base.HeaderValue {
	var rets []string

	rets = append(rets, "RTP/AVP/TCP")

	if h.Destination != nil {
		rets = append(rets, "destination="+h.Destination.String())
	}

	if h.InterleavedIDs != nil {
		rets = append(rets, "interleaved="+strconv.FormatInt(int64(h.InterleavedIDs[0]), 10)+
			"-"+strconv.FormatInt(int64(h.InterleavedIDs[1]), 10))
	}

	if h.Ports != nil {
		rets = append(rets, "port="+strconv.FormatInt(int64(h.Ports[0]), 10)+
			"-"+strconv.FormatInt(int64(h.Ports[1]), 10))
	}

	if h.TTL != nil {
		rets = append(rets, "ttl="+strconv.FormatUint(uint64(*h.TTL), 10))
	}

	if h.ClientPorts != nil {
		rets = append(rets, "client_port="+strconv.FormatInt(int64(h.ClientPorts[0]), 10)+
			"-"+strconv.FormatInt(int64(h.ClientPorts[1]), 10))
	}

	if h.ServerPorts != nil {
		rets = append(rets, "server_port="+strconv.FormatInt(int64(h.ServerPorts[0]), 10)+
			"-"+strconv.FormatInt(int64(h.ServerPorts[1]), 10))
	}

	if h.SSRC != nil {
		tmp := make([]byte, 4)
		binary.BigEndian.PutUint32(tmp, *h.SSRC)
		rets = append(rets, "ssrc="+strings.ToUpper(hex.EncodeToString(tmp)))
	}

	if h.Mode != nil {
		if *h.Mode == TransportModePlay {
			rets = append(rets, "mode=play")
		} else {
			rets = append(rets, "mode=record")
		}
	}

	return base.HeaderValue{strings.Join(rets, ";")}
}
