//go:build linux

package route

import (
	"context"
	"encoding/binary"
	"fmt"
	"golang.org/x/sys/unix"
	"net"
	"net/netip"
	"syscall"
)

type Native struct{}

var endian = binary.NativeEndian

func attrs(data []byte) (map[uint16][]byte, error) {
	out := map[uint16][]byte{}
	for len(data) > 0 {
		if len(data) < 4 {
			return nil, fmt.Errorf("truncated route attr")
		}
		n := int(endian.Uint16(data))
		if n < 4 || n > len(data) {
			return nil, fmt.Errorf("invalid attr length")
		}
		k := endian.Uint16(data[2:]) & 0x3fff
		out[k] = append([]byte{}, data[4:n]...)
		pad := (n + 3) &^ 3
		if pad > len(data) {
			return nil, fmt.Errorf("invalid attr padding")
		}
		data = data[pad:]
	}
	return out, nil
}
func attr(k uint16, v []byte) []byte {
	b := make([]byte, (len(v)+7)&^3)
	endian.PutUint16(b, uint16(len(v)+4))
	endian.PutUint16(b[2:], k)
	copy(b[4:], v)
	return b
}
func u32(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return endian.Uint32(b)
}
func word(v uint32) []byte { b := make([]byte, 4); endian.PutUint32(b, v); return b }
func decode(raw []byte) (Route, error) {
	if len(raw) < 12 {
		return Route{}, fmt.Errorf("short route")
	}
	a, e := attrs(raw[12:])
	if e != nil {
		return Route{}, e
	}
	family := raw[0]
	bits := int(raw[1])
	var ip netip.Addr
	dst := a[unix.RTA_DST]
	if family == unix.AF_INET {
		var b [4]byte
		copy(b[:], dst)
		ip = netip.AddrFrom4(b)
	} else if family == unix.AF_INET6 {
		var b [16]byte
		copy(b[:], dst)
		ip = netip.AddrFrom16(b)
	} else {
		return Route{}, fmt.Errorf("unknown route family")
	}
	if bits > ip.BitLen() {
		return Route{}, fmt.Errorf("bad prefix")
	}
	table := uint32(raw[4])
	if a[unix.RTA_TABLE] != nil {
		table = u32(a[unix.RTA_TABLE])
	}
	index := u32(a[unix.RTA_OIF])
	iface := ""
	if v, e := net.InterfaceByIndex(int(index)); e == nil {
		iface = v.Name
	}
	metrics, e := attrs(a[unix.RTA_METRICS])
	if e != nil {
		return Route{}, e
	}
	algo := string(metrics[unix.RTAX_CC_ALGO])
	for len(algo) > 0 && algo[len(algo)-1] == 0 {
		algo = algo[:len(algo)-1]
	}
	return Route{netip.PrefixFrom(ip, bits).Masked(), table, iface, index, raw[5], algo, u32(metrics[unix.RTAX_LOCK])&(1<<unix.RTAX_CC_ALGO) != 0, append([]byte{}, raw...)}, nil
}
func (Native) Read(ctx context.Context) ([]Route, error) {
	out := []Route{}
	for _, family := range []int{unix.AF_INET, unix.AF_INET6} {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		b, e := syscall.NetlinkRIB(unix.RTM_GETROUTE, family)
		if e != nil {
			return nil, e
		}
		msgs, e := syscall.ParseNetlinkMessage(b)
		if e != nil {
			return nil, e
		}
		for _, m := range msgs {
			if m.Header.Flags&unix.NLM_F_DUMP_INTR != 0 {
				return nil, fmt.Errorf("interrupted route dump")
			}
			if m.Header.Type == unix.RTM_NEWROUTE {
				r, e := decode(m.Data)
				if e != nil {
					return nil, e
				}
				if len(r.Raw) >= 12 && r.Raw[7] == unix.RTN_UNICAST && r.Index != 0 {
					out = append(out, r)
				}
			}
		}
	}
	return out, nil
}
func sameIdentity(a, b Route) bool {
	return a.Prefix == b.Prefix && a.Table == b.Table && a.Index == b.Index && a.Protocol == b.Protocol
}
func (Native) Change(ctx context.Context, old Route, algo string, lock bool) error {
	all, e := (Native{}).Read(ctx)
	if e != nil {
		return e
	}
	var found *Route
	for i := range all {
		if sameIdentity(all[i], old) {
			if found != nil {
				return fmt.Errorf("ambiguous route")
			}
			found = &all[i]
		}
	}
	if found == nil {
		return fmt.Errorf("route identity changed")
	}
	if found.Algorithm != old.Algorithm || found.Locked != old.Locked {
		return fmt.Errorf("route attributes changed")
	}
	raw, e := patch(found.Raw, algo, lock)
	if e != nil {
		return e
	}
	fd, e := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if e != nil {
		return e
	}
	defer unix.Close(fd)
	unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &unix.Timeval{Sec: 1})
	if e = unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); e != nil {
		return e
	}
	msg := make([]byte, 16+len(raw))
	endian.PutUint32(msg, uint32(len(msg)))
	endian.PutUint16(msg[4:], unix.RTM_NEWROUTE)
	endian.PutUint16(msg[6:], unix.NLM_F_REQUEST|unix.NLM_F_ACK|unix.NLM_F_REPLACE)
	endian.PutUint32(msg[8:], 1)
	copy(msg[16:], raw)
	if e = unix.Sendto(fd, msg, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); e != nil {
		return e
	}
	buf := make([]byte, 65536)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, _, er := unix.Recvfrom(fd, buf, 0)
		if er == unix.EAGAIN || er == unix.EINTR {
			continue
		}
		if er != nil {
			return er
		}
		ms, er := syscall.ParseNetlinkMessage(buf[:n])
		if er != nil {
			return er
		}
		for _, m := range ms {
			if m.Header.Type == unix.NLMSG_ERROR && len(m.Data) >= 4 {
				code := int32(endian.Uint32(m.Data))
				if code != 0 {
					return syscall.Errno(-code)
				}
				return nil
			}
		}
	}
}
func rewriteAttrs(raw []byte, changes map[uint16][]byte) ([]byte, error) {
	out := []byte{}
	done := map[uint16]bool{}
	for len(raw) > 0 {
		if len(raw) < 4 {
			return nil, fmt.Errorf("truncated attribute")
		}
		n := int(endian.Uint16(raw))
		padded := (n + 3) &^ 3
		if n < 4 || padded > len(raw) {
			return nil, fmt.Errorf("invalid attribute")
		}
		fullType := endian.Uint16(raw[2:])
		key := fullType & 0x3fff
		if value, changed := changes[key]; changed {
			if !done[key] && value != nil {
				out = append(out, attr(fullType, value)...)
			}
			done[key] = true
		} else {
			out = append(out, raw[:padded]...)
		}
		raw = raw[padded:]
	}
	for key, value := range changes {
		if !done[key] && value != nil {
			out = append(out, attr(key, value)...)
		}
	}
	return out, nil
}
func patch(raw []byte, algo string, lock bool) ([]byte, error) {
	a, e := attrs(raw[12:])
	if e != nil {
		return nil, e
	}
	metrics, e := attrs(a[unix.RTA_METRICS])
	if e != nil {
		return nil, e
	}
	bits := u32(metrics[unix.RTAX_LOCK])
	bits &^= 1 << unix.RTAX_CC_ALGO
	if lock {
		bits |= 1 << unix.RTAX_CC_ALGO
	}
	changes := map[uint16][]byte{unix.RTAX_CC_ALGO: nil, unix.RTAX_LOCK: nil}
	if algo != "" {
		changes[unix.RTAX_CC_ALGO] = append([]byte(algo), 0)
	}
	if bits != 0 {
		changes[unix.RTAX_LOCK] = word(bits)
	}
	m, e := rewriteAttrs(a[unix.RTA_METRICS], changes)
	if e != nil {
		return nil, e
	}
	if len(m) == 0 {
		m = nil
	}
	body, e := rewriteAttrs(raw[12:], map[uint16][]byte{unix.RTA_METRICS: m})
	if e != nil {
		return nil, e
	}
	return append(append([]byte{}, raw[:12]...), body...), nil
}
func (Native) Watch(ctx context.Context, ch chan<- struct{}) error {
	fd, e := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if e != nil {
		return e
	}
	defer unix.Close(fd)
	if e = unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK, Groups: unix.RTMGRP_IPV4_ROUTE | unix.RTMGRP_IPV6_ROUTE | unix.RTMGRP_LINK}); e != nil {
		return e
	}
	unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &unix.Timeval{Sec: 1})
	b := make([]byte, 1<<20)
	for ctx.Err() == nil {
		_, _, e = unix.Recvfrom(fd, b, 0)
		if e == unix.EAGAIN || e == unix.EINTR {
			continue
		}
		if e != nil {
			return e
		}
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	return ctx.Err()
}
