## go-traceroute

[![Go Reference](https://pkg.go.dev/badge/gopkg.in/dnaeon/go-traceroute.v1.svg)](https://pkg.go.dev/gopkg.in/dnaeon/go-traceroute.v1)

`go-traceroute` is an implementation of the traditional, ancient
method of tracerouting, which uses probes as UDP datagram packets and
an unlikely destination port.

## Installation

Install `go-traceroute` by executing the command below:

```bash
$ go get -v gopkg.in/dnaeon/go-traceroute.v1/tracer
```

## Usage

A simple example of using `go-traceroute` is provided below.

``` go
package main

import (
        "context"
        "net"

        "gopkg.in/dnaeon/go-traceroute.v1/tracer"
)

func main() {
        dest := net.IPv4(142, 251, 140, 14)
        ctx := context.Background()
        opts := tracer.DefaultOptions
        t := tracer.New(opts)
        ch := t.Trace(ctx, dest)

        for probe := range ch {
                // Process probes ...
        }
}
```

Also, make sure to check the [examples](./examples) directory from
this repository, which provides ready-to-run programs using the
`go-traceroute` package.

Run a trace to a given host.

``` shell
go run examples/traceroute/main.go google.com
```

Generate the Dot representation of a trace.

``` shell
go run examples/traceroute-dot/main.go google.com
```

## License

`go-traceroute` is Open Source and licensed under the
[BSD License](http://opensource.org/licenses/BSD-2-Clause)
