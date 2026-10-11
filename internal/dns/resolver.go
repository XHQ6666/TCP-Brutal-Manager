package dns

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"github.com/XHQ6666/TCP-Brutal-Manager/internal/config"
	wire "github.com/miekg/dns"
	"github.com/quic-go/quic-go"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Engine struct {
	Config     *config.Config
	RetryDelay time.Duration
}

func (e Engine) Resolve(ctx context.Context, name string) ([]netip.Addr, error) {
	first, fallback := e.Config.Select(name)
	chain := []config.Resolver{first}
	if fallback {
		for _, r := range e.Config.DNS.Resolvers {
			if r.Name != first.Name {
				chain = append(chain, r)
			}
		}
	}
	var last error
	for _, r := range chain {
		for attempt := 0; attempt < 3; attempt++ {
			addrs, err := e.pair(ctx, r, name)
			if err == nil {
				return addrs, nil
			}
			last = err
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if attempt < 2 {
				d := e.RetryDelay
				if d == 0 {
					d = 3 * time.Second
				}
				t := time.NewTimer(d)
				select {
				case <-ctx.Done():
					t.Stop()
					return nil, ctx.Err()
				case <-t.C:
				}
			}
		}
	}
	return nil, last
}
func (e Engine) pair(ctx context.Context, r config.Resolver, name string) ([]netip.Addr, error) {
	out := []netip.Addr{}
	for _, typ := range []uint16{wire.TypeA, wire.TypeAAAA} {
		a, nx, err := e.query(ctx, r, name, typ)
		if err != nil {
			return nil, err
		}
		if nx {
			return []netip.Addr{}, nil
		}
		out = append(out, a...)
	}
	return out, nil
}
func (e Engine) query(ctx context.Context, r config.Resolver, name string, typ uint16) ([]netip.Addr, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, e.Config.Durations.DNS)
	defer cancel()
	if r.Type == "System" {
		network := "ip4"
		if typ == wire.TypeAAAA {
			network = "ip6"
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, network, name)
		if err != nil {
			if de, ok := err.(*net.DNSError); ok && de.IsNotFound {
				return []netip.Addr{}, false, nil
			}
			return nil, false, err
		}
		out := []netip.Addr{}
		for _, ip := range ips {
			a, ok := netip.AddrFromSlice(ip)
			if ok {
				a = a.Unmap()
				if (typ == wire.TypeA) == a.Is4() {
					out = append(out, a)
				}
			}
		}
		return out, false, nil
	}
	m := new(wire.Msg)
	m.SetQuestion(wire.Fqdn(name), typ)
	if r.Type == "DoQ" {
		m.Id = 0
	}
	var reply *wire.Msg
	var err error
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", r.Address)
	if err != nil || len(ips) == 0 {
		return nil, false, fmt.Errorf("bootstrap %s: %v", r.Address, err)
	}
	endpoint := net.JoinHostPort(ips[0].String(), strconv.Itoa(r.Port))
	tlsConf := &tls.Config{ServerName: r.Address, MinVersion: tls.VersionTLS12}
	switch r.Type {
	case "DoT":
		cl := &wire.Client{Net: "tcp-tls", TLSConfig: tlsConf}
		reply, _, err = cl.ExchangeContext(ctx, m, endpoint)
	case "DoH":
		data, er := m.Pack()
		if er != nil {
			return nil, false, er
		}
		u := url.URL{Scheme: "https", Host: net.JoinHostPort(r.Address, strconv.Itoa(r.Port)), Path: r.Path}
		transport := &http.Transport{TLSClientConfig: tlsConf, DialContext: func(c context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(c, network, endpoint)
		}}
		defer transport.CloseIdleConnections()
		req, er := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(data))
		if er != nil {
			return nil, false, er
		}
		req.Header.Set("Content-Type", "application/dns-message")
		req.Header.Set("Accept", "application/dns-message")
		client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("DoH redirects disabled") }}
		resp, er := client.Do(req)
		if er != nil {
			return nil, false, er
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/dns-message") {
			return nil, false, fmt.Errorf("invalid DoH status/content-type")
		}
		data, err = io.ReadAll(io.LimitReader(resp.Body, 65536))
		if err == nil {
			reply = new(wire.Msg)
			err = reply.Unpack(data)
		}
	case "DoQ":
		tlsConf.MinVersion = tls.VersionTLS13
		tlsConf.NextProtos = []string{"doq"}
		conn, er := quic.DialAddr(ctx, endpoint, tlsConf, &quic.Config{})
		if er != nil {
			return nil, false, er
		}
		defer conn.CloseWithError(0, "")
		stream, er := conn.OpenStreamSync(ctx)
		if er != nil {
			return nil, false, er
		}
		if deadline, ok := ctx.Deadline(); ok {
			stream.SetDeadline(deadline)
		}
		data, er := m.Pack()
		if er != nil {
			return nil, false, er
		}
		frame := make([]byte, 2+len(data))
		binary.BigEndian.PutUint16(frame, uint16(len(data)))
		copy(frame[2:], data)
		if _, er = stream.Write(frame); er != nil {
			return nil, false, er
		}
		if er = stream.Close(); er != nil {
			return nil, false, er
		}
		var size [2]byte
		if _, er = io.ReadFull(stream, size[:]); er != nil {
			return nil, false, er
		}
		data = make([]byte, binary.BigEndian.Uint16(size[:]))
		_, err = io.ReadFull(stream, data)
		if err == nil {
			reply = new(wire.Msg)
			err = reply.Unpack(data)
		}
	default:
		return nil, false, fmt.Errorf("unsupported resolver")
	}
	if err != nil {
		return nil, false, err
	}
	if reply == nil || !reply.Response || reply.Id != m.Id || reply.Truncated || len(reply.Question) != 1 || !strings.EqualFold(reply.Question[0].Name, m.Question[0].Name) || reply.Question[0].Qtype != typ {
		return nil, false, fmt.Errorf("malformed DNS response")
	}
	if reply.Rcode == wire.RcodeNameError {
		return []netip.Addr{}, true, nil
	}
	if reply.Rcode != wire.RcodeSuccess {
		return nil, false, fmt.Errorf("DNS rcode %d", reply.Rcode)
	}
	valid := map[string]bool{strings.ToLower(wire.Fqdn(name)): true}
	for i := 0; i < 16; i++ {
		changed := false
		for _, rr := range reply.Answer {
			if c, ok := rr.(*wire.CNAME); ok && valid[strings.ToLower(c.Hdr.Name)] && !valid[strings.ToLower(c.Target)] {
				valid[strings.ToLower(c.Target)] = true
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	out := []netip.Addr{}
	for _, rr := range reply.Answer {
		if !valid[strings.ToLower(rr.Header().Name)] {
			continue
		}
		var ip net.IP
		switch v := rr.(type) {
		case *wire.A:
			if typ == wire.TypeA {
				ip = v.A
			}
		case *wire.AAAA:
			if typ == wire.TypeAAAA {
				ip = v.AAAA
			}
		}
		if a, ok := netip.AddrFromSlice(ip); ok {
			out = append(out, a.Unmap())
		}
	}
	return out, false, nil
}
