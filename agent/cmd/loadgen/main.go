// SPDX-License-Identifier: Apache-2.0

// loadgen is a purpose-built flow generator for validating NFR-1 (agent
// overhead at 10k concurrent flows) against a real cluster. The existing
// chidrixx-test fixtures (a single nginx pod) can't sustain that many
// concurrent connections — nginx's default worker_connections caps out far
// below 10k on one replica. loadgen sidesteps that entirely: -mode=serve is
// a bare TCP sink with no per-connection processing, and -mode=drive holds
// open N concurrent TCP connections (each a distinct 5-tuple, i.e. a
// distinct flow in the agent's map) for a fixed duration, trickling a byte
// or two down each every couple of seconds so the agent's cgroup_skb hooks
// see real traffic on every one of them, not just an idle SYN.
//
// Concurrency is meant to be split across many replicas on both ends
// (Deployment for serve, Job with parallelism for drive) rather than one
// giant process, to stay comfortably under typical per-container
// open-file-descriptor limits.
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	mode := flag.String("mode", "", "serve | drive")
	addr := flag.String("addr", ":9500", "address to listen on (serve mode)")
	target := flag.String("target", "", "host:port to connect to (drive mode)")
	concurrency := flag.Int("concurrency", 500, "concurrent TCP connections to hold open (drive mode)")
	duration := flag.Duration("duration", 45*time.Second, "how long to hold connections open (drive mode)")
	flag.Parse()

	switch *mode {
	case "serve":
		serve(*addr)
	case "drive":
		if *target == "" {
			fmt.Fprintln(os.Stderr, "-target is required in drive mode")
			os.Exit(1)
		}
		drive(*target, *concurrency, *duration)
	default:
		fmt.Fprintln(os.Stderr, "-mode must be 'serve' or 'drive'")
		os.Exit(1)
	}
}

// serve accepts connections and discards whatever they send, forever. No
// per-connection buffering beyond a small read buffer — the point is to
// hold the connection open, not to do anything with the bytes.
func serve(addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen %s: %v\n", addr, err)
		os.Exit(1)
	}

	fmt.Printf("loadgen sink listening on %s\n", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}

		go func() {
			defer conn.Close()
			buf := make([]byte, 64)
			for {
				if _, err := conn.Read(buf); err != nil {
					return
				}
			}
		}()
	}
}

// drive opens `concurrency` concurrent TCP connections to target, sends a
// byte down each every 2s to generate ongoing on-wire traffic, and holds
// them open for `duration` before releasing everything.
func drive(target string, concurrency int, duration time.Duration) {
	var connected, failed int64

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < concurrency; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			conn, err := net.DialTimeout("tcp", target, 5*time.Second)
			if err != nil {
				atomic.AddInt64(&failed, 1)
				return
			}
			defer conn.Close()

			atomic.AddInt64(&connected, 1)

			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
					if _, err := conn.Write([]byte{'x'}); err != nil {
						return
					}
				}
			}
		}()

		// Stagger dials in batches so this doesn't look like a connect
		// storm to the kernel/conntrack and start timing out its own
		// dials before the target side has accepted the earlier ones.
		if i%200 == 199 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	time.Sleep(duration)
	close(stop)
	wg.Wait()

	fmt.Printf("requested=%d connected=%d failed=%d\n", concurrency, atomic.LoadInt64(&connected), atomic.LoadInt64(&failed))
}
