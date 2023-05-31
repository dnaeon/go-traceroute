// Copyright (c) 2023 Marin Atanasov Nikolov <dnaeon@gmail.com>
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions
// are met:
//
//  1. Redistributions of source code must retain the above copyright
//     notice, this list of conditions and the following disclaimer
//     in this position and unchanged.
//  2. Redistributions in binary form must reproduce the above copyright
//     notice, this list of conditions and the following disclaimer in the
//     documentation and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE AUTHOR(S) ``AS IS'' AND ANY EXPRESS OR
// IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES
// OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED.
// IN NO EVENT SHALL THE AUTHOR(S) BE LIABLE FOR ANY DIRECT, INDIRECT,
// INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT
// NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF
// THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package tracer

import (
        "context"
        "net"
        "syscall"
        "time"
        "unsafe"

        "golang.org/x/net/ipv4"
)

// See https://github.com/torvalds/linux/blob/master/include/uapi/linux/errqueue.h#L28
type SockExtendedErrorOrigin uint8

const (
        SockExtendedErrorOriginNone SockExtendedErrorOrigin = iota
        SockExtendedErrorOriginLocal
        SockExtendedErrorOriginICMP
        SockExtendedErrorOriginICMP6
        SockExtendedErrorOriginTxStatus
        SockExtendedErrorOriginZeroCopy
        SockExtendedErrorOriginTxTime
        SockExtendedErrorOriginTimestamp = SockExtendedErrorOriginTxStatus
)

// See https://github.com/torvalds/linux/blob/master/include/uapi/linux/errqueue.h#L15
type SockExtendedErr struct {
        Errno  uint32
        Origin uint8
        Type   uint8
        Code   uint8
        Pad    uint8
        Info   uint32
        Data   uint32
}

// Options provide configuration settings for the Tracer.
type Options struct {
        // "Unlikely" destination port to use when tracing.
        DestinationPort uint16

        // Specifies the maximum number of hops (max time-to-live) the
        // Tracer will probe.
        MaxHops int

        // Specifies the number of probes to send per hop.
        NumProbes uint

        // Specifies how long to wait for a response to a probe.
        ProbeMaxWaitDuration time.Duration

        // PacketLength represents the size of the probe packets
        PacketLength int
}

// Default options for the Tracer
var DefaultOptions = &Options{
        DestinationPort:      33434,
        MaxHops:              30,
        NumProbes:            3,
        ProbeMaxWaitDuration: 500 * time.Millisecond,
        PacketLength:         60,
}

// Tracer implements the traditional, ancient method of tracerouting,
// which uses probes as UDP datagram packets and an "unlikely"
// destination port.
type Tracer struct {
        opts *Options
}

// New creates a new Tracer with the given options.
func New(opts *Options) *Tracer {
        if opts == nil {
                opts = DefaultOptions
        }

        tracer := &Tracer{
                opts: opts,
        }

        return tracer
}

// Probe represents a trace probe
type Probe struct {
        // Start time of the probe
        Start time.Time

        // End time of the probe
        End time.Time

        // IP of the discovered hop
        Hop net.IP

        // TTL of the probe
        TTL int

        // Error provides the error which may have occurred during
        // tracing
        Error error
}

// Trace traces the hops between us and the destination.
func (t *Tracer) Trace(ctx context.Context, dest net.IP) <-chan Probe {
        ch := make(chan Probe)

        prober := func() {
                ttl := 0
        L:
                for {
                        select {
                        case <-ctx.Done():
                                break L
                        default:
                                // Emit probes
                                ttl += 1
                                probes, err := t.sendProbes(dest, ttl)
                                if err != nil {
                                        ch <- Probe{Error: err}
                                        break L
                                }

                                // Send probe results
                                destReached := false
                                for _, probe := range probes {
                                        ch <- probe
                                        if probe.Hop.Equal(dest) {
                                                destReached = true
                                        }
                                }

                                // Are we there yet?
                                if destReached || ttl >= t.opts.MaxHops {
                                        break L
                                }
                        }
                }
                close(ch)
        }

        go prober()
        return ch
}

// Sends the probes to the destination with the given TTL.
func (t *Tracer) sendProbes(dest net.IP, ttl int) ([]Probe, error) {
        var dstAddr4 [4]byte
        copy(dstAddr4[:], dest.To4())
        soAddr4 := &syscall.SockaddrInet4{
                Port: int(t.opts.DestinationPort),
                Addr: dstAddr4,
        }

        fd, err := t.createSocket(ttl)
        if err != nil {
                return nil, err
        }
        defer syscall.Close(fd)

        epollFd, err := syscall.EpollCreate(1)
        if err != nil {
                return nil, err
        }
        defer syscall.Close(epollFd)

        var epollEvent syscall.EpollEvent
        if err := syscall.EpollCtl(epollFd, syscall.EPOLL_CTL_ADD, fd, &epollEvent); err != nil {
                return nil, err
        }

        probes := make([]Probe, 0)
        for i := 0; i < int(t.opts.NumProbes); i++ {
                start := time.Now()
                b := make([]byte, t.opts.PacketLength)
                if err != nil {
                        return nil, err
                }

                if err := syscall.Sendto(fd, b, 0, soAddr4); err != nil {
                        return nil, err
                }

                // https://datatracker.ietf.org/doc/html/rfc1812
                p := make([]byte, 1500)
                oob := make([]byte, 1500)
                hopIp := net.IPv4zero
                var probeError error
                for {
                        now := time.Now()
                        timeout := now.Add(t.opts.ProbeMaxWaitDuration).Sub(now).Nanoseconds() / int64(time.Millisecond)
                        syscall.EpollWait(epollFd, []syscall.EpollEvent{epollEvent}, int(timeout))
                        _, _, _, _, err := syscall.Recvmsg(fd, p, oob, syscall.MSG_ERRQUEUE)
                        if err != nil {
                                break
                        }

                        cMsgHdr := (*syscall.Cmsghdr)(unsafe.Pointer(&oob[0]))
                        if cMsgHdr.Level != syscall.IPPROTO_IP {
                                continue
                        }

                        se := (*SockExtendedErr)(unsafe.Pointer(&oob[syscall.SizeofCmsghdr]))
                        if se.Origin != uint8(SockExtendedErrorOriginICMP) {
                                continue
                        }

                        switch cMsgHdr.Type {
                        case int32(ipv4.ICMPTypeTimeExceeded), int32(ipv4.ICMPTypeDestinationUnreachable):
                                src := (*syscall.RawSockaddrInet4)(unsafe.Pointer(&oob[syscall.SizeofCmsghdr+int(unsafe.Sizeof(*se))]))
                                hopIp = net.IP([]byte(src.Addr[:]))
                        }
                        break
                }

                end := time.Now()
                probe := Probe{
                        Start: start,
                        End:   end,
                        Hop:   hopIp,
                        TTL:   ttl,
                        Error: probeError,
                }
                probes = append(probes, probe)
        }

        return probes, nil
}

// Creates a socket with the given TTL.
func (t *Tracer) createSocket(ttl int) (int, error) {
        fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP)
        if err != nil {
                return fd, err
        }

        timeout := syscall.NsecToTimeval(int64(t.opts.ProbeMaxWaitDuration * 1000 * 1000 * 1000))
        if err := syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &timeout); err != nil {
                return fd, err
        }

        if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
                return fd, err
        }

        if err := syscall.SetsockoptInt(fd, syscall.SOL_IP, syscall.IP_TTL, ttl); err != nil {
                return fd, err
        }

        // Set IP_RECVERR here, so that we can receive the ICMP
        // control messages in the error queue
        if err := syscall.SetsockoptInt(fd, syscall.SOL_IP, syscall.IP_RECVERR, 1); err != nil {
                return fd, err
        }

        return fd, nil
}
