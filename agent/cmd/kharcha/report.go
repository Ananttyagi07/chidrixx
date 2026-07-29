// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

type BackendFinding struct {
	Name      string
	Namespace string
	Pod       string
	PodUID    string
	IP        string
	Node      string
	Ready     bool
	Serving   bool
	Port      string
}

type Finding struct {
	CgroupID uint64

	Source     string
	SourcePod  string
	SourceIP   string
	SourceNode string
	Container  string

	Destination string
	DestPod     string
	DestIP      string
	DestNode    string

	DestService     string
	DestServiceType string
	DestServicePort string

	ObservedEndpoint string

	Backends []BackendFinding

	Class      PathClass
	Confidence string
	FixHint    string

	BytesTx     uint64
	BytesRx     uint64
	CostLowINR  float64
	CostHighINR float64
}

// fixHints maps a wasteful PathClass to the concrete remediation the build
// manual's fix engine (§5.4/Step 9.3) calls for. Paths that are already
// free or unavoidable (SAME_NODE, SAME_AZ, PRIVATE_OFFCLUSTER) carry no
// hint — there's nothing to fix.
var fixHints = map[PathClass]string{
	PathCrossAZ:        "Enable Topology Aware Routing on this Service, or co-locate these two workloads in the same zone.",
	PathCrossRegion:    "Confirm this traffic needs to cross regions at all; if it must, batch or compress it.",
	PathNATEgress:      "Add a VPC/private endpoint for this destination instead of routing it through the NAT gateway.",
	PathInternetEgress: "Confirm this traffic needs to leave the cluster; cache or compress it if it's chatty.",
	PathManagedService: "Pin this client and the managed-service endpoint to the same zone.",
}

// Aggregate accumulates per-flow deltas into cumulative findings, pricing
// each one with the injected price book and classifying each one with the
// injected zone lookup / managed-service list — nothing about the report
// layer talks to Kubernetes or a YAML file directly.
type Aggregate struct {
	byKey map[string]*Finding

	priceBook       *PriceBook
	zoneOf          ZoneLookup
	managedNets     []*net.IPNet
	nodeHasPublicIP bool

	// localNodeName is the fallback SourceNode for traffic whose cgroup
	// didn't resolve to a Kubernetes pod (host-level processes) — we still
	// know it left from this node, even without pod metadata.
	localNodeName string
}

func NewAggregate(priceBook *PriceBook, zoneOf ZoneLookup, managedNets []*net.IPNet, nodeHasPublicIP bool, localNodeName string) *Aggregate {
	return &Aggregate{
		byKey:           make(map[string]*Finding),
		priceBook:       priceBook,
		zoneOf:          zoneOf,
		managedNets:     managedNets,
		nodeHasPublicIP: nodeHasPublicIP,
		localNodeName:   localNodeName,
	}
}

func (a *Aggregate) isManaged(ip net.IP) bool {
	for _, n := range a.managedNets {
		if n.Contains(ip) {
			return true
		}
	}

	return false
}

func endpointPortString(endpoint *KubeEndpoint) string {
	if endpoint == nil || len(endpoint.Ports) == 0 {
		return ""
	}

	p := endpoint.Ports[0]

	if p.Port == 0 {
		return ""
	}

	if p.Protocol != "" {
		return fmt.Sprintf("%d/%s", p.Port, p.Protocol)
	}

	return fmt.Sprintf("%d", p.Port)
}

func backendFinding(endpoint *KubeEndpoint) BackendFinding {
	if endpoint == nil {
		return BackendFinding{}
	}

	name := endpoint.DisplayName()

	return BackendFinding{
		Name:      name,
		Namespace: endpoint.Namespace,
		Pod:       endpoint.PodName,
		PodUID:    endpoint.PodUID,
		IP:        endpoint.IP,
		Node:      endpoint.NodeName,
		Ready:     endpoint.Ready,
		Serving:   endpoint.Serving,
		Port:      endpointPortString(endpoint),
	}
}

func (a *Aggregate) Add(
	identity WorkloadIdentity,
	remote net.IP,
	destination *KubeWorkload,
	service *KubeService,
	endpoint *KubeEndpoint,
	serviceBackends []*KubeEndpoint,
	tx uint64,
	rx uint64,
) {
	remoteIP := "?"
	if remote != nil {
		remoteIP = remote.String()
	}

	// -------------------------
	// Source metadata
	// -------------------------

	source := identity.DisplayName()

	sourcePod := ""
	sourceIP := ""
	sourceNode := ""
	container := ""

	if identity.Kubernetes != nil {
		sourcePod = identity.Kubernetes.Pod
		sourceIP = identity.Kubernetes.PodIP
		sourceNode = identity.Kubernetes.Node
		container = identity.Kubernetes.Container
	}

	if sourceNode == "" {
		sourceNode = a.localNodeName
	}

	// -------------------------
	// Destination metadata
	// -------------------------

	destName := remoteIP
	destPod := ""
	destNode := ""

	destService := ""
	destServiceType := ""
	destServicePort := ""

	observedEndpoint := ""

	// Direct Pod IP / EndpointSlice backend.
	if destination != nil {
		destName = destination.DisplayName()
		destPod = destination.Pod
		destNode = destination.Node
	}

	// EndpointSlice confirms that the address observed by eBPF
	// corresponds to a backend endpoint.
	if endpoint != nil {
		observedEndpoint = endpoint.DisplayName()

		if destName == remoteIP && observedEndpoint != "" {
			destName = observedEndpoint
		}

		if destPod == "" {
			destPod = endpoint.PodName
		}

		if destNode == "" {
			destNode = endpoint.NodeName
		}
	}

	// Logical Kubernetes Service.
	if service != nil {
		destName = service.DisplayName()
		destService = service.Name
		destServiceType = service.Type

		if len(service.Ports) > 0 {
			p := service.Ports[0]

			destServicePort = fmt.Sprintf(
				"%d/%s",
				p.Port,
				p.Protocol,
			)

			if p.TargetPort != "" {
				destServicePort += " -> " + p.TargetPort
			}
		}
	}

	// -------------------------
	// Classification + pricing
	// -------------------------
	//
	// Destination metadata (destNode above) must be resolved before we
	// classify, since SAME_NODE/SAME_AZ/CROSS_AZ/CROSS_REGION all depend
	// on knowing which node the remote end runs on.

	class, confidence := Classify(ClassifyInput{
		Remote:          remote,
		SourceNode:      sourceNode,
		DestNode:        destNode,
		Managed:         remote != nil && a.isManaged(remote),
		NodeHasPublicIP: a.nodeHasPublicIP,
	}, a.zoneOf)

	recordFlowBytes(source, remoteIP, class, tx, rx)

	// -------------------------
	// Candidate backends
	// -------------------------

	var backends []BackendFinding

	for _, backend := range serviceBackends {
		if backend == nil {
			continue
		}

		backends = append(
			backends,
			backendFinding(backend),
		)
	}

	sort.Slice(backends, func(i, j int) bool {
		if backends[i].IP == backends[j].IP {
			return backends[i].Name < backends[j].Name
		}

		return backends[i].IP < backends[j].IP
	})

	// -------------------------
	// Aggregation key
	// -------------------------

	key := fmt.Sprintf(
		"%d|%s|%s",
		identity.CgroupID,
		class,
		remoteIP,
	)

	f := a.byKey[key]

	if f == nil {
		f = &Finding{
			CgroupID: identity.CgroupID,

			Source:     source,
			SourcePod:  sourcePod,
			SourceIP:   sourceIP,
			SourceNode: sourceNode,
			Container:  container,

			Destination: destName,
			DestPod:     destPod,
			DestIP:      remoteIP,
			DestNode:    destNode,

			DestService:     destService,
			DestServiceType: destServiceType,
			DestServicePort: destServicePort,

			ObservedEndpoint: observedEndpoint,

			Backends: backends,

			Class:      class,
			Confidence: confidence,
			FixHint:    fixHints[class],
		}

		a.byKey[key] = f
	} else {
		// Kubernetes metadata (and classification) can change after the
		// finding was originally created, so refresh descriptive fields.
		f.Source = source
		f.SourcePod = sourcePod
		f.SourceIP = sourceIP
		f.SourceNode = sourceNode
		f.Container = container

		f.Destination = destName
		f.DestPod = destPod
		f.DestIP = remoteIP
		f.DestNode = destNode

		f.DestService = destService
		f.DestServiceType = destServiceType
		f.DestServicePort = destServicePort

		f.ObservedEndpoint = observedEndpoint
		f.Backends = backends

		f.Class = class
		f.Confidence = confidence
		f.FixHint = fixHints[class]
	}

	f.BytesTx += tx
	f.BytesRx += rx

	low, high := a.priceBook.CostINR(class, confidence, tx+rx)
	f.CostLowINR += low
	f.CostHighINR += high
}

func boolStatus(v bool) string {
	if v {
		return "true"
	}

	return "false"
}

// Findings returns every accumulated finding, most expensive first — the
// same ordering PrintTop uses, exposed so other output formats (HTML,
// Prometheus) don't have to re-sort.
func (a *Aggregate) Findings() []*Finding {
	var all []*Finding

	for _, f := range a.byKey {
		all = append(all, f)
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].CostHighINR == all[j].CostHighINR {
			return all[i].BytesTx+all[i].BytesRx >
				all[j].BytesTx+all[j].BytesRx
		}

		return all[i].CostHighINR > all[j].CostHighINR
	})

	return all
}

func (a *Aggregate) PrintTop(n int) {
	all := a.Findings()

	if n > len(all) {
		n = len(all)
	}

	fmt.Println()
	fmt.Println("=== Chidrixx Kubernetes Network Attribution ===")

	if n == 0 {
		fmt.Println("No traffic observed yet.")
		return
	}

	for i, f := range all[:n] {
		fmt.Printf("\n#%d\n", i+1)

		fmt.Printf("SOURCE      : %s\n", f.Source)

		if f.SourcePod != "" {
			fmt.Printf("  pod       : %s\n", f.SourcePod)
		}

		if f.Container != "" {
			fmt.Printf("  container : %s\n", f.Container)
		}

		if f.SourceIP != "" {
			fmt.Printf("  ip        : %s\n", f.SourceIP)
		}

		if f.SourceNode != "" {
			fmt.Printf("  node      : %s\n", f.SourceNode)
		}

		fmt.Printf("DESTINATION : %s\n", f.Destination)

		if f.DestService != "" {
			fmt.Printf("  service   : %s\n", f.DestService)
		}

		if f.DestServiceType != "" {
			fmt.Printf("  type      : %s\n", f.DestServiceType)
		}

		if f.DestServicePort != "" {
			fmt.Printf("  port      : %s\n", f.DestServicePort)
		}

		if f.DestPod != "" {
			fmt.Printf("  pod       : %s\n", f.DestPod)
		}

		fmt.Printf("  ip        : %s\n", f.DestIP)

		if f.DestNode != "" {
			fmt.Printf("  node      : %s\n", f.DestNode)
		}

		// If eBPF directly observed an EndpointSlice backend,
		// this is stronger evidence than merely knowing the
		// candidate backends of a Service.
		if f.ObservedEndpoint != "" {
			fmt.Printf(
				"OBSERVED BACKEND : %s\n",
				f.ObservedEndpoint,
			)
		}

		if len(f.Backends) > 0 {
			fmt.Printf(
				"CANDIDATE BACKENDS (%d):\n",
				len(f.Backends),
			)

			for j, backend := range f.Backends {
				name := backend.Name
				if strings.TrimSpace(name) == "" {
					name = backend.IP
				}

				fmt.Printf(
					"  [%d] %s\n",
					j+1,
					name,
				)

				if backend.Pod != "" {
					fmt.Printf(
						"      pod     : %s\n",
						backend.Pod,
					)
				}

				if backend.IP != "" {
					fmt.Printf(
						"      ip      : %s\n",
						backend.IP,
					)
				}

				if backend.Node != "" {
					fmt.Printf(
						"      node    : %s\n",
						backend.Node,
					)
				}

				if backend.Port != "" {
					fmt.Printf(
						"      port    : %s\n",
						backend.Port,
					)
				}

				fmt.Printf(
					"      ready   : %s\n",
					boolStatus(backend.Ready),
				)

				fmt.Printf(
					"      serving : %s\n",
					boolStatus(backend.Serving),
				)
			}

			fmt.Println(
				"  note      : candidates only; selected backend is not inferred from Service metadata",
			)
		}

		fmt.Printf("CLASS       : %s (confidence: %s)\n", f.Class, f.Confidence)
		fmt.Printf("TX          : %d bytes\n", f.BytesTx)
		fmt.Printf("RX          : %d bytes\n", f.BytesRx)
		fmt.Printf("COST        : ₹%.4f - ₹%.4f\n", f.CostLowINR, f.CostHighINR)

		if f.FixHint != "" {
			fmt.Printf("FIX         : %s\n", f.FixHint)
		}
	}

	fmt.Printf(
		"\nprices: %s/%s . list rates last verified %s . USD->INR %.1f\n",
		a.priceBook.Cloud, a.priceBook.Region, a.priceBook.LastVerified, a.priceBook.FX.UsdToInr,
	)
	fmt.Println("note: estimates from list prices unless you loaded negotiated rates; ranges widen for lower-confidence classifications.")
}
