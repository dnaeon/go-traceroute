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
        "io"
        "log"
        "math"
        "net"
        "os"
        "strconv"

        "gopkg.in/dnaeon/go-traceroute.v1/tracer"
)

func main() {
        if len(os.Args) != 2 {
                fmt.Fprintf(os.Stderr, "Usage: traceroute-dot <host>\n")
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

        // A mapping between TTL and list of probes
        maxTtl := math.MinInt
        minTtl := math.MaxInt
        probes := make(map[int][]*tracer.Probe)
        for p := range ch {
                if p.TTL > maxTtl {
                        maxTtl = p.TTL
                }
                if p.TTL < minTtl {
                        minTtl = p.TTL
                }
                probe := p
                probes[p.TTL] = append(probes[p.TTL], &probe)
        }

        nodeAttrs := `[color=lightblue fillcolor=lightblue fontcolor=black shape=record style="filled, rounded"]`
        fmt.Fprintln(os.Stdout, "digraph {")
        fmt.Fprintf(os.Stdout, "\tnode %s\n", nodeAttrs)

        // Handle the case when we have only a single hop
        if minTtl == maxTtl {
                nodes := uniqueHops(probes[minTtl])
                for _, node := range nodes {
                        writeHop(os.Stdout, node)
                }
        }

        // Will only be invoked when we have more than 1 hop to the
        // destination
        for ttl := minTtl + 1; ttl < maxTtl; ttl++ {
                currNodes := uniqueHops(probes[ttl])
                prevNodes := uniqueHops(probes[ttl-1])
                for _, prevNode := range prevNodes {
                        writeHop(os.Stdout, prevNode)
                        for _, currNode := range currNodes {
                                writeHop(os.Stdout, currNode)
                                fmt.Fprintf(os.Stdout, "\t%d -> %d\n", dotId(prevNode), dotId(currNode))
                        }
                }
        }
        fmt.Fprintln(os.Stdout, "}")
}

// Writes the hop representation in dot format
func writeHop(w io.Writer, p *tracer.Probe) {
        label := p.Hop.String()
        if p.Hop.Equal(net.IPv4zero) {
                label = "*"
        }
        fmt.Fprintf(w, "\t%d [label=\"%s\"]\n", dotId(p), label)
}

// Returns the unique dot ID for the given probe
func dotId(p *tracer.Probe) int64 {
        addr := fmt.Sprintf("%p", p)
        id, err := strconv.ParseInt(addr[2:], 16, 64)
        if err != nil {
                panic(err)
        }

        return id
}

// Returns the list of unique hops based on the hop
func uniqueHops(probes []*tracer.Probe) []*tracer.Probe {
        items := make(map[string]*tracer.Probe)
        for _, p := range probes {
                if _, ok := items[p.Hop.String()]; ok {
                        continue
                }
                items[p.Hop.String()] = p
        }

        result := make([]*tracer.Probe, 0)
        for _, v := range items {
                result = append(result, v)
        }

        return result
}
