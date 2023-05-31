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

package main

import (
        "context"
        "fmt"
        "log"
        "net"
        "os"

        "gopkg.in/dnaeon/go-traceroute.v1/tracer"
)

func main() {
        if len(os.Args) != 2 {
                fmt.Fprintf(os.Stderr, "Usage: traceroute <host>\n")
                os.Exit(64)
        }

        host := os.Args[1]
        dest, err := net.ResolveIPAddr("ip", host)
        if err != nil {
                log.Fatal(err)
        }

        ctx := context.Background()
        opts := tracer.DefaultOptions
        t := tracer.New(opts)
        ch := t.Trace(ctx, dest.IP)

        fmt.Printf("traceroute to %s (%s), %d hops max, %d byte packets", host, dest.IP, opts.MaxHops, opts.PacketLength)

        oldHop := net.IPv4zero
        oldTtl := 0
        for probe := range ch {
                ttlChanged := false
                diff := probe.End.Sub(probe.Start).String()

                if probe.Error != nil {
                        fmt.Printf("%-3d %s\n", probe.TTL, probe.Error)
                        continue
                }

                // TTL has changed, we are now processing probes from the next hop
                if probe.TTL != oldTtl {
                        oldTtl = probe.TTL
                        ttlChanged = true
                        fmt.Printf("\n%-3d ", probe.TTL)
                }

                // Did we discover anything at all?
                if probe.Hop.Equal(net.IPv4zero) {
                        fmt.Printf("%-15s ", "*")
                        continue
                }

                // Hop has changed
                if !probe.Hop.Equal(oldHop) || ttlChanged {
                        fmt.Printf("%-15s ", probe.Hop)
                }
                oldHop = probe.Hop

                fmt.Printf("%-15s ", diff)
        }
        fmt.Println()
}
