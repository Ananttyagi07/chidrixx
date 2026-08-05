// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
)

// parseManagedCIDRs parses a comma-separated CIDR list (the -managed-cidrs
// flag) into the []*net.IPNet the classifier checks remote IPs against.
func parseManagedCIDRs(csv string) ([]*net.IPNet, error) {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return nil, nil
	}

	var nets []*net.IPNet

	for _, part := range strings.Split(csv, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		_, n, err := net.ParseCIDR(part)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", part, err)
		}

		nets = append(nets, n)
	}

	return nets, nil
}

// resolveNodeName picks this node's name: the flag if set, else $NODE_NAME
// (the standard Downward API env var a DaemonSet sets from
// spec.nodeName), else the OS hostname as a last resort for bare/local
// runs.
func resolveNodeName(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}

	if env := os.Getenv("NODE_NAME"); env != "" {
		return env
	}

	host, err := os.Hostname()
	if err != nil {
		return ""
	}

	return host
}

// FlowKey must exactly match the key layout used by the eBPF flows map.
type FlowKey struct {
	CgroupID uint64
	Saddr    uint32
	Daddr    uint32
	Sport    uint16
	Dport    uint16
	Proto    uint8
	Pad      [3]byte
}

// FlowStat must exactly match the value layout used by the eBPF flows map.
type FlowStat struct {
	BytesTx   uint64
	PacketsTx uint64
	BytesRx   uint64
	PacketsRx uint64
}

// ipString converts the IPv4 uint32 stored by eBPF
// into a human-readable IPv4 address.
func ipString(v uint32) string {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return net.IP(b).String()
}

// delta calculates traffic generated since the previous scrape.
//
// If counters become smaller than the previous values, assume the
// LRU map entry was evicted/reset and treat the current counters
// as the new delta.
func delta(prev, cur FlowStat) FlowStat {
	if cur.BytesTx < prev.BytesTx || cur.BytesRx < prev.BytesRx {
		return cur
	}

	return FlowStat{
		BytesTx:   cur.BytesTx - prev.BytesTx,
		PacketsTx: cur.PacketsTx - prev.PacketsTx,
		BytesRx:   cur.BytesRx - prev.BytesRx,
		PacketsRx: cur.PacketsRx - prev.PacketsRx,
	}
}

// scrape reads the current per-CPU eBPF flow map,
// calculates traffic deltas, resolves the originating workload,
// resolves Kubernetes destination metadata, and feeds everything
// into the cumulative aggregator.
func scrape(
	m *ebpf.Map,
	prev map[FlowKey]FlowStat,
	agg *Aggregate,
	resolver *WorkloadResolver,
	kube *KubernetesResolver,
) map[FlowKey]FlowStat {
	start := time.Now()
	defer func() {
		scrapeLagSeconds.Set(time.Since(start).Seconds())
	}()

	cur := make(map[FlowKey]FlowStat)

	var key FlowKey
	var perCPU []FlowStat

	iter := m.Iterate()

	for iter.Next(&key, &perCPU) {
		var sum FlowStat

		// The eBPF map is PERCPU, so combine counters
		// from every CPU into one FlowStat.
		for _, s := range perCPU {
			sum.BytesTx += s.BytesTx
			sum.PacketsTx += s.PacketsTx
			sum.BytesRx += s.BytesRx
			sum.PacketsRx += s.PacketsRx
		}

		cur[key] = sum

		// Calculate traffic added since the previous scrape.
		d := delta(prev[key], sum)

		if d.BytesTx == 0 && d.BytesRx == 0 {
			continue
		}

		// Daddr currently represents the normalized remote endpoint
		// produced by our eBPF flow accounting logic.
		remote := net.ParseIP(ipString(key.Daddr))

		// Resolve the originating cgroup into either:
		//
		//   Kubernetes workload metadata
		//
		// or:
		//
		//   host/system cgroup metadata.
		source := resolver.Resolve(key.CgroupID)

		var destination *KubeWorkload
		var service *KubeService
		var endpoint *KubeEndpoint
		var serviceBackends []*KubeEndpoint

		if kube != nil {
			// -------------------------------------------------
			// 1. Direct Pod-IP destination
			// -------------------------------------------------
			//
			// Example:
			//
			//   10.42.0.11 -> 10.42.0.12
			//
			destination = kube.ResolveIP(remote)

			// -------------------------------------------------
			// 2. Kubernetes Service ClusterIP
			// -------------------------------------------------
			//
			// Example:
			//
			//   10.43.63.100 -> chidrixx-test/service/server
			//
			service = kube.ResolveServiceIP(remote)

			// -------------------------------------------------
			// 3. EndpointSlice backend IP
			// -------------------------------------------------
			//
			// The observed remote address may already be the
			// backend Pod IP after Kubernetes service NAT.
			//
			endpoint = kube.ResolveEndpointIP(remote)

			// -------------------------------------------------
			// 4. Service -> candidate EndpointSlice backends
			// -------------------------------------------------
			//
			// IMPORTANT:
			//
			// EndpointSlice tells us which Pods are registered
			// behind a Service. It does NOT by itself prove which
			// backend handled this specific connection.
			//
			if service != nil {
				serviceBackends = kube.ResolveServiceBackends(service)
			}

			// -------------------------------------------------
			// 5. Endpoint -> Kubernetes workload
			// -------------------------------------------------
			//
			// If the destination wasn't resolved directly through
			// the Pod-IP cache, but the observed address matches an
			// EndpointSlice backend, resolve that endpoint to its
			// backing Pod/container metadata.
			//
			if destination == nil && endpoint != nil {
				destination = kube.ResolveEndpointWorkload(endpoint)
			}
		}

		// Feed this scrape window into the cumulative report.
		agg.Add(
			source,
			remote,
			destination,
			service,
			endpoint,
			serviceBackends,
			d.BytesTx,
			d.BytesRx,
		)
	}

	if err := iter.Err(); err != nil {
		log.Printf("iterate error: %v", err)
	}

	// MaxEntries() is read from the real loaded map rather than repeating
	// bpf/flow_cgroup.c's max_entries literal, so resizing the map there
	// keeps this ratio (and any alert bound to it) correct automatically.
	recordMapUtilization(len(cur), m.MaxEntries())

	return cur
}

func main() {
	bpfObject := flag.String(
		"bpf-object",
		"bpf/flow_cgroup.o",
		"path to the compiled eBPF object (built from bpf/flow_cgroup.c)",
	)
	cgroupPath := flag.String(
		"cgroup-path",
		"/sys/fs/cgroup",
		"cgroup v2 mount to attach the egress/ingress programs to",
	)
	pricebookPath := flag.String(
		"pricebook",
		"pricebook/aws.yaml",
		"path to the YAML price book (build manual §9.1/§12)",
	)
	managedCIDRsFlag := flag.String(
		"managed-cidrs",
		"",
		"comma-separated CIDRs of managed-service endpoints (RDS/MSK/ES/...) to classify as MANAGED_SERVICE",
	)
	nodeHasPublicIP := flag.Bool(
		"node-has-public-ip",
		true,
		"whether this node has its own path to the internet; false enables the NAT-egress heuristic for off-cluster traffic",
	)
	nodeNameFlag := flag.String(
		"node-name",
		"",
		"this node's name, for SAME_NODE/zone classification of host-level traffic; defaults to $NODE_NAME then the OS hostname",
	)
	htmlOut := flag.String(
		"html-out",
		"report.html",
		"path to write the HTML report to after every scrape; empty disables it",
	)
	metricsAddr := flag.String(
		"metrics-addr",
		":9300",
		"address to serve Prometheus /metrics on; empty disables it",
	)
	alertWebhook := flag.String(
		"alert-webhook",
		"",
		"Slack-compatible incoming webhook URL for cost alerts (FR-R3); empty disables alerting",
	)
	alertThresholdINR := flag.Float64(
		"alert-threshold-inr",
		100.0,
		"minimum estimated cost (INR, high end of range) before a finding can alert",
	)
	alertGrowthRatio := flag.Float64(
		"alert-growth-ratio",
		2.0,
		"re-alert on a finding once its cost has grown by this multiple since the last alert",
	)
	controlPlaneURL := flag.String(
		"controlplane-url",
		"",
		"control plane ingest endpoint (e.g. http://chidrixx-controlplane:8090/api/v1/ingest); empty disables shipping",
	)
	clusterID := flag.String(
		"cluster-id",
		"",
		"this cluster's name, reported to the control plane; defaults to -node-name if unset",
	)
	k8sRefreshInterval := flag.Duration(
		"k8s-refresh-interval",
		10*time.Second,
		"how often to refresh Kubernetes pod/service/endpoint/node metadata (FR-E1 target: <=10s); "+
			"shorter means fresher destination attribution for newly-created pods but more kubectl exec load on the API server",
	)
	flag.Parse()

	// Load and attach the eBPF programs ourselves instead of assuming
	// something outside the agent (bpftool, a shell script) already pinned
	// the map — the agent now owns the full load/attach/detach lifecycle.
	loader, m, err := Load(*bpfObject, *cgroupPath)
	if err != nil {
		log.Fatalf("load eBPF programs: %v", err)
	}
	defer loader.Close()

	priceBook, err := LoadPriceBook(*pricebookPath)
	if err != nil {
		log.Fatalf("load price book: %v", err)
	}

	managedNets, err := parseManagedCIDRs(*managedCIDRsFlag)
	if err != nil {
		log.Fatalf("parse -managed-cidrs: %v", err)
	}

	nodeName := resolveNodeName(*nodeNameFlag)

	fmt.Println("chidrixx-reader: Kubernetes-aware attribution enabled")
	fmt.Println("scraping every 15s, Ctrl+C to stop")

	// ---------------------------------------------------------
	// Kubernetes metadata cache
	// ---------------------------------------------------------
	//
	// Resolver currently understands:
	//
	//   Pod UID      -> workload
	//   Container ID -> workload
	//   Pod IP       -> workload
	//   Service IP   -> Service
	//   Endpoint IP  -> EndpointSlice backend
	//   Service      -> candidate EndpointSlice backends
	//
	kube := NewKubernetesResolver()

	// Cancelled on SIGINT/SIGTERM so both loops exit and we print a final
	// report instead of dying mid-scrape.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *metricsAddr != "" {
		serveMetrics(ctx, *metricsAddr)
		fmt.Printf("prometheus metrics on %s/metrics\n", *metricsAddr)
	}

	// Keep Kubernetes metadata synchronized with cluster changes.
	go func() {
		ticker := time.NewTicker(*k8sRefreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := kube.Refresh(); err != nil {
					log.Printf(
						"kubernetes metadata refresh: %v",
						err,
					)
				}
				kube.refreshDeployEvents()
			}
		}
	}()

	// Previous eBPF map snapshot used for delta calculation.
	prev := make(map[FlowKey]FlowStat)

	// Cumulative traffic/cost aggregator.
	agg := NewAggregate(priceBook, kube.Zone, managedNets, *nodeHasPublicIP, nodeName)

	alerter := NewAlerter(*alertWebhook, *alertThresholdINR, *alertGrowthRatio)

	effectiveClusterID := *clusterID
	if effectiveClusterID == "" {
		effectiveClusterID = nodeName
	}
	// Read from the environment, not a flag, so the token never shows up
	// in `ps`/container specs — same reasoning as the control plane side.
	shipper := NewShipper(*controlPlaneURL, effectiveClusterID, os.Getenv("CHIDRIXX_AUTH_TOKEN"))

	// Resolves cgroup IDs into Kubernetes or host workload identities.
	resolver := NewWorkloadResolver(kube)

	// Scrape the eBPF flow map every 15 seconds.
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nshutting down, final report:")
			agg.PrintTop(100)
			writeHTMLReport(agg, *htmlOut)
			return
		case <-ticker.C:
			prev = scrape(
				m,
				prev,
				agg,
				resolver,
				kube,
			)

			// Print the highest-value findings currently observed.
			agg.PrintTop(100)
			writeHTMLReport(agg, *htmlOut)
			recordCostMetrics(agg)
			alerter.Check(agg.Findings())

			if err := shipper.Ship(ctx, agg.Findings(), kube.DrainDeployEvents()); err != nil {
				log.Printf("ship to control plane: %v", err)
			}
		}
	}
}

// writeHTMLReport writes the HTML export (build manual Step 10.1) if
// htmlOut is non-empty. A write failure is logged, not fatal — the CLI
// report already succeeded, and an unwritable report path shouldn't kill
// the agent.
func writeHTMLReport(agg *Aggregate, htmlOut string) {
	if htmlOut == "" {
		return
	}

	if err := agg.WriteHTMLFile(htmlOut, 100); err != nil {
		log.Printf("write HTML report %s: %v", htmlOut, err)
	}
}
