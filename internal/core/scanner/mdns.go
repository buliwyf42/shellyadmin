package scanner

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"

	"shellyadmin/internal/models"
)

func ScanMDNS(ctx context.Context, timeout time.Duration, logFn func(level, msg string)) []models.Device {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	names := discoverMDNSNames(ctx, timeout, logFn)
	if len(names) == 0 {
		return nil
	}
	results := make([]models.Device, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		host := strings.TrimSuffix(name, ".")
		addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip4", host)
		if err != nil {
			logFn("DEBUG", fmt.Sprintf("[scan] mdns resolve failed for %s: %v", host, err))
			continue
		}
		for _, addr := range addrs {
			ip := addr.String()
			if seen[ip] {
				continue
			}
			seen[ip] = true
			if device := ProbeDevice(ctx, ip, timeout, logFn); device != nil {
				results = append(results, *device)
			}
		}
	}
	return results
}

func MergeDevices(primary, extra []models.Device) []models.Device {
	if len(extra) == 0 {
		return primary
	}
	seen := make(map[string]bool, len(primary))
	out := make([]models.Device, 0, len(primary)+len(extra))
	for _, device := range primary {
		key := scanKey(device)
		if key != "" {
			seen[key] = true
		}
		out = append(out, device)
	}
	for _, device := range extra {
		keys := []string{scanValue(device.MAC), scanValue(device.IP)}
		duplicate := false
		for _, key := range keys {
			if key != "" && seen[key] {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		for _, key := range keys {
			if key != "" {
				seen[key] = true
			}
		}
		out = append(out, device)
	}
	return out
}

func scanKey(device models.Device) string {
	if device.MAC != "" {
		return device.MAC
	}
	return device.IP
}

func scanValue(value string) string {
	return strings.TrimSpace(value)
}

func discoverMDNSNames(ctx context.Context, timeout time.Duration, logFn func(level, msg string)) []string {
	serviceNames := []string{"_http._tcp.local.", "_shelly._tcp.local."}
	collected := map[string]bool{}
	for _, serviceName := range serviceNames {
		names, err := browseMDNS(ctx, serviceName, timeout)
		if err != nil {
			logFn("DEBUG", fmt.Sprintf("[scan] mdns browse failed for %s: %v", serviceName, err))
			continue
		}
		for _, name := range names {
			if looksLikeShellyName(name) {
				collected[name] = true
			}
		}
	}
	out := make([]string, 0, len(collected))
	for name := range collected {
		out = append(out, name)
	}
	return out
}

func browseMDNS(ctx context.Context, serviceName string, timeout time.Duration) ([]string, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	udpConn := conn.(*net.UDPConn)
	_ = udpConn.SetDeadline(time.Now().Add(timeout))
	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{RecursionDesired: false})
	builder.EnableCompression()
	_ = builder.StartQuestions()
	name, err := dnsmessage.NewName(serviceName)
	if err != nil {
		return nil, err
	}
	if err := builder.Question(dnsmessage.Question{Name: name, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET}); err != nil {
		return nil, err
	}
	packet, err := builder.Finish()
	if err != nil {
		return nil, err
	}
	dst := &net.UDPAddr{IP: net.ParseIP("224.0.0.251"), Port: 5353}
	if _, err := udpConn.WriteToUDP(packet, dst); err != nil {
		return nil, err
	}
	results := map[string]bool{}
	buf := make([]byte, 1500)
	for {
		select {
		case <-ctx.Done():
			return mapKeys(results), nil
		default:
		}
		n, _, err := udpConn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return mapKeys(results), nil
			}
			return nil, err
		}
		parser := dnsmessage.Parser{}
		if _, err := parser.Start(buf[:n]); err != nil {
			continue
		}
		if err := parser.SkipAllQuestions(); err != nil {
			continue
		}
		for {
			answer, err := parser.AnswerHeader()
			if err == dnsmessage.ErrSectionDone {
				break
			}
			if err != nil {
				break
			}
			switch answer.Type {
			case dnsmessage.TypePTR:
				ptr, err := parser.PTRResource()
				if err == nil {
					results[ptr.PTR.String()] = true
				}
			case dnsmessage.TypeSRV:
				srv, err := parser.SRVResource()
				if err == nil {
					results[srv.Target.String()] = true
				}
			default:
				if err := parser.SkipAnswer(); err != nil {
					break
				}
			}
		}
	}
}

func looksLikeShellyName(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "shelly")
}

func mapKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	return out
}
