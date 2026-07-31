// SPDX-License-Identifier: Apache-2.0
package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const serviceAccountDir = "/var/run/secrets/kubernetes.io/serviceaccount"

const inClusterKubeconfigTemplate = `apiVersion: v1
kind: Config
clusters:
- name: in-cluster
  cluster:
    server: https://%s:%s
    certificate-authority: %s
contexts:
- name: in-cluster
  context:
    cluster: in-cluster
    user: in-cluster
current-context: in-cluster
users:
- name: in-cluster
  user:
    tokenFile: %s
`

// ensureKubeconfig makes the kubectl binary this resolver shells out to
// usable from inside a Pod. Unlike client-go's rest.InClusterConfig,
// kubectl does not auto-discover the mounted service account — without
// this, a Helm-deployed agent would build, pass RBAC, and then silently
// fail every "kubectl get" call with "no configuration has been provided."
//
// If the caller already has a kubeconfig (KUBECONFIG set, or ~/.kube/config
// present — the normal case for local dev against a kind/k3d cluster, and
// exactly what's already working on this box), this is a no-op. Otherwise,
// if the standard in-cluster service account files are mounted, it
// synthesizes a minimal kubeconfig from them and points KUBECONFIG at it.
// tokenFile (not a baked-in token) is used deliberately: kubectl re-reads
// it on every call, so projected service account token rotation is
// handled for free.
func ensureKubeconfig() error {
	if os.Getenv("KUBECONFIG") != "" {
		return nil
	}

	if home, err := os.UserHomeDir(); err == nil {
		if _, statErr := os.Stat(filepath.Join(home, ".kube", "config")); statErr == nil {
			return nil
		}
	}

	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	port := os.Getenv("KUBERNETES_SERVICE_PORT")

	if host == "" || port == "" {
		return nil // not running in a Pod; nothing to synthesize
	}

	tokenFile := filepath.Join(serviceAccountDir, "token")
	caFile := filepath.Join(serviceAccountDir, "ca.crt")

	if _, err := os.Stat(tokenFile); err != nil {
		return nil // no mounted service account; let kubectl fail with its normal error
	}

	f, err := os.CreateTemp("", "chidrixx-kubeconfig-*.yaml")
	if err != nil {
		return fmt.Errorf("create in-cluster kubeconfig: %w", err)
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, inClusterKubeconfigTemplate, host, port, caFile, tokenFile); err != nil {
		return fmt.Errorf("write in-cluster kubeconfig: %w", err)
	}

	return os.Setenv("KUBECONFIG", f.Name())
}

type KubeWorkload struct {
	Namespace   string
	Pod         string
	PodUID      string
	Container   string
	ContainerID string
	PodIP       string
	Node        string
}

func (k *KubeWorkload) DisplayName() string {
	if k == nil {
		return ""
	}

	if k.Namespace != "" && k.Pod != "" {
		return k.Namespace + "/" + k.Pod
	}

	return k.Pod
}

// KubeService represents a Kubernetes Service address.
type KubeService struct {
	Namespace string
	Name      string
	ClusterIP string
	Type      string
	Ports     []KubeServicePort
}

type KubeServicePort struct {
	Name       string
	Protocol   string
	Port       int32
	TargetPort string
}

func (s *KubeService) DisplayName() string {
	if s == nil {
		return ""
	}

	if s.Namespace != "" && s.Name != "" {
		return s.Namespace + "/service/" + s.Name
	}

	return s.Name
}

// KubeEndpoint represents one backend endpoint belonging to a Service.
//
// The EndpointSlice controller normally points TargetRef at the backing Pod.
// Keeping both the endpoint IP and Pod UID lets us resolve the backend even
// if one of those identifiers is temporarily unavailable.
type KubeEndpoint struct {
	Namespace string

	ServiceName string
	SliceName   string

	IP       string
	NodeName string

	PodName string
	PodUID  string

	Ready       bool
	Serving     bool
	Terminating bool

	Ports []KubeEndpointPort
}

type KubeEndpointPort struct {
	Name     string
	Protocol string
	Port     int32
}

func (e *KubeEndpoint) DisplayName() string {
	if e == nil {
		return ""
	}

	if e.Namespace != "" && e.PodName != "" {
		return e.Namespace + "/" + e.PodName
	}

	if e.PodName != "" {
		return e.PodName
	}

	return e.IP
}

// KubeNode carries the topology facts the classifier needs: which zone a
// node sits in (for SAME_AZ/CROSS_AZ/CROSS_REGION) and its internal IP (so
// traffic to a node directly — hostNetwork pods, NodePort — resolves too).
type KubeNode struct {
	Name       string
	Zone       string
	InternalIP string
}

// kubeAPIClient talks to the API server directly over HTTPS, reusing one
// keep-alive connection across refresh cycles. It exists because shelling
// out to kubectl four times per refresh (pods/services/endpointslices/nodes)
// forks a whole new process and TLS handshake each time — measured as the
// dominant share of the agent's CPU overhead in practice, far more than the
// eBPF map scrape itself. Only usable in-cluster, where the projected
// service account token/CA are mounted; local dev without them falls back
// to the kubectl-exec path unchanged.
type kubeAPIClient struct {
	baseURL    string
	tokenFile  string
	httpClient *http.Client
}

func newKubeAPIClient() *kubeAPIClient {
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	port := os.Getenv("KUBERNETES_SERVICE_PORT")

	if host == "" || port == "" {
		return nil
	}

	tokenFile := filepath.Join(serviceAccountDir, "token")
	caFile := filepath.Join(serviceAccountDir, "ca.crt")

	if _, err := os.Stat(tokenFile); err != nil {
		return nil
	}

	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCert) {
		return nil
	}

	return &kubeAPIClient{
		baseURL:   fmt.Sprintf("https://%s:%s", host, port),
		tokenFile: tokenFile,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig:     &tls.Config{RootCAs: pool},
				MaxIdleConnsPerHost: 4,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// get fetches a path from the API server. The token is re-read on every
// call, not cached, so projected service account token rotation is handled
// for free — same rationale as tokenFile in ensureKubeconfig.
func (c *kubeAPIClient) get(path string) ([]byte, error) {
	token, err := os.ReadFile(c.tokenFile)
	if err != nil {
		return nil, fmt.Errorf("read service account token: %w", err)
	}

	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: unexpected status %d: %s", path, resp.StatusCode, string(body))
	}

	return body, nil
}

type KubernetesResolver struct {
	mu sync.RWMutex

	api *kubeAPIClient

	byPodUID      map[string]*KubeWorkload
	byContainerID map[string]*KubeWorkload
	byIP          map[string]*KubeWorkload
	byServiceIP   map[string]*KubeService

	// namespace/service -> EndpointSlice backends
	byServiceBackends map[string][]*KubeEndpoint

	// backend IP -> endpoint metadata
	byEndpointIP map[string]*KubeEndpoint

	byNodeName map[string]*KubeNode
	byNodeIP   map[string]*KubeNode

	lastRefresh time.Time
}

func NewKubernetesResolver() *KubernetesResolver {
	if err := ensureKubeconfig(); err != nil {
		fmt.Printf("in-cluster kubeconfig setup: %v\n", err)
	}

	r := &KubernetesResolver{
		api:               newKubeAPIClient(),
		byPodUID:          make(map[string]*KubeWorkload),
		byContainerID:     make(map[string]*KubeWorkload),
		byIP:              make(map[string]*KubeWorkload),
		byServiceIP:       make(map[string]*KubeService),
		byServiceBackends: make(map[string][]*KubeEndpoint),
		byEndpointIP:      make(map[string]*KubeEndpoint),
		byNodeName:        make(map[string]*KubeNode),
		byNodeIP:          make(map[string]*KubeNode),
	}

	if err := r.Refresh(); err != nil {
		fmt.Printf("kubernetes metadata unavailable: %v\n", err)
	}

	return r
}

type kubePodList struct {
	Items []struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
			UID       string `json:"uid"`
		} `json:"metadata"`

		Spec struct {
			NodeName string `json:"nodeName"`
		} `json:"spec"`

		Status struct {
			PodIP string `json:"podIP"`

			ContainerStatuses []struct {
				Name        string `json:"name"`
				ContainerID string `json:"containerID"`
			} `json:"containerStatuses"`
		} `json:"status"`
	} `json:"items"`
}

type kubeServiceList struct {
	Items []struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`

		Spec struct {
			ClusterIP string `json:"clusterIP"`
			Type      string `json:"type"`

			Ports []struct {
				Name       string      `json:"name"`
				Protocol   string      `json:"protocol"`
				Port       int32       `json:"port"`
				TargetPort interface{} `json:"targetPort"`
			} `json:"ports"`
		} `json:"spec"`
	} `json:"items"`
}

type kubeEndpointSliceList struct {
	Items []struct {
		Metadata struct {
			Name      string            `json:"name"`
			Namespace string            `json:"namespace"`
			Labels    map[string]string `json:"labels"`
		} `json:"metadata"`

		AddressType string `json:"addressType"`

		Ports []struct {
			Name     *string `json:"name"`
			Protocol *string `json:"protocol"`
			Port     *int32  `json:"port"`
		} `json:"ports"`

		Endpoints []struct {
			Addresses []string `json:"addresses"`

			Conditions struct {
				Ready       *bool `json:"ready"`
				Serving     *bool `json:"serving"`
				Terminating *bool `json:"terminating"`
			} `json:"conditions"`

			NodeName *string `json:"nodeName"`

			TargetRef *struct {
				Kind      string `json:"kind"`
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
				UID       string `json:"uid"`
			} `json:"targetRef"`
		} `json:"endpoints"`
	} `json:"items"`
}

const zoneLabel = "topology.kubernetes.io/zone"

type kubeNodeList struct {
	Items []struct {
		Metadata struct {
			Name   string            `json:"name"`
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`

		Status struct {
			Addresses []struct {
				Type    string `json:"type"`
				Address string `json:"address"`
			} `json:"addresses"`
		} `json:"status"`
	} `json:"items"`
}

func normalizeContainerID(id string) string {
	if i := strings.Index(id, "://"); i >= 0 {
		return id[i+3:]
	}

	return id
}

func targetPortString(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return fmt.Sprintf("%.0f", x)
	default:
		return ""
	}
}

func serviceKey(namespace, name string) string {
	return namespace + "/" + name
}

func boolValue(v *bool) bool {
	return v != nil && *v
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}

	return *v
}

func int32Value(v *int32) int32 {
	if v == nil {
		return 0
	}

	return *v
}

// fetch returns the raw JSON for one resource list, preferring a direct
// API server call (apiPath) when in-cluster credentials are available and
// falling back to shelling out to kubectl (kubectlArgs) otherwise.
func (r *KubernetesResolver) fetch(apiPath string, kubectlArgs ...string) ([]byte, error) {
	if r.api != nil {
		return r.api.get(apiPath)
	}

	out, err := exec.Command("kubectl", kubectlArgs...).Output()
	if err != nil {
		return nil, fmt.Errorf("kubectl %s: %w", strings.Join(kubectlArgs, " "), err)
	}

	return out, nil
}

func (r *KubernetesResolver) Refresh() error {
	// --------------------------------------------------
	// Pods
	// --------------------------------------------------

	podOut, err := r.fetch("/api/v1/pods", "get", "pods", "-A", "-o", "json")
	if err != nil {
		return fmt.Errorf("list pods: %w", err)
	}

	var podList kubePodList

	if err := json.Unmarshal(podOut, &podList); err != nil {
		return fmt.Errorf("decode kubernetes pod list: %w", err)
	}

	byPodUID := make(map[string]*KubeWorkload)
	byContainerID := make(map[string]*KubeWorkload)
	byIP := make(map[string]*KubeWorkload)

	for _, pod := range podList.Items {
		// A Pod may theoretically be visible before container status exists.
		// We still want IP/UID attribution in that case.
		if len(pod.Status.ContainerStatuses) == 0 {
			w := &KubeWorkload{
				Namespace: pod.Metadata.Namespace,
				Pod:       pod.Metadata.Name,
				PodUID:    pod.Metadata.UID,
				PodIP:     pod.Status.PodIP,
				Node:      pod.Spec.NodeName,
			}

			if w.PodUID != "" {
				byPodUID[w.PodUID] = w
			}

			if w.PodIP != "" {
				byIP[w.PodIP] = w
			}

			continue
		}

		for _, container := range pod.Status.ContainerStatuses {
			id := normalizeContainerID(container.ContainerID)

			w := &KubeWorkload{
				Namespace:   pod.Metadata.Namespace,
				Pod:         pod.Metadata.Name,
				PodUID:      pod.Metadata.UID,
				Container:   container.Name,
				ContainerID: id,
				PodIP:       pod.Status.PodIP,
				Node:        pod.Spec.NodeName,
			}

			if w.PodUID != "" {
				byPodUID[w.PodUID] = w
			}

			if w.ContainerID != "" {
				byContainerID[w.ContainerID] = w
			}

			if w.PodIP != "" {
				byIP[w.PodIP] = w
			}
		}
	}

	// --------------------------------------------------
	// Services
	// --------------------------------------------------

	serviceOut, err := r.fetch("/api/v1/services", "get", "services", "-A", "-o", "json")
	if err != nil {
		return fmt.Errorf("list services: %w", err)
	}

	var serviceList kubeServiceList

	if err := json.Unmarshal(serviceOut, &serviceList); err != nil {
		return fmt.Errorf("decode kubernetes service list: %w", err)
	}

	byServiceIP := make(map[string]*KubeService)

	for _, svc := range serviceList.Items {
		ip := svc.Spec.ClusterIP

		// Headless services have no virtual ClusterIP.
		if ip == "" || ip == "None" {
			continue
		}

		s := &KubeService{
			Namespace: svc.Metadata.Namespace,
			Name:      svc.Metadata.Name,
			ClusterIP: ip,
			Type:      svc.Spec.Type,
		}

		for _, port := range svc.Spec.Ports {
			s.Ports = append(s.Ports, KubeServicePort{
				Name:       port.Name,
				Protocol:   port.Protocol,
				Port:       port.Port,
				TargetPort: targetPortString(port.TargetPort),
			})
		}

		byServiceIP[ip] = s
	}

	// --------------------------------------------------
	// EndpointSlices
	// --------------------------------------------------

	endpointOut, err := r.fetch(
		"/apis/discovery.k8s.io/v1/endpointslices",
		"get", "endpointslices", "-A", "-o", "json",
	)
	if err != nil {
		return fmt.Errorf("list endpointslices: %w", err)
	}

	var endpointList kubeEndpointSliceList

	if err := json.Unmarshal(endpointOut, &endpointList); err != nil {
		return fmt.Errorf("decode kubernetes endpointslice list: %w", err)
	}

	byServiceBackends := make(map[string][]*KubeEndpoint)
	byEndpointIP := make(map[string]*KubeEndpoint)

	for _, slice := range endpointList.Items {
		serviceName := slice.Metadata.Labels["kubernetes.io/service-name"]

		if serviceName == "" {
			continue
		}

		var ports []KubeEndpointPort

		for _, port := range slice.Ports {
			ports = append(ports, KubeEndpointPort{
				Name:     stringValue(port.Name),
				Protocol: stringValue(port.Protocol),
				Port:     int32Value(port.Port),
			})
		}

		for _, endpoint := range slice.Endpoints {
			for _, address := range endpoint.Addresses {
				e := &KubeEndpoint{
					Namespace:   slice.Metadata.Namespace,
					ServiceName: serviceName,
					SliceName:   slice.Metadata.Name,
					IP:          address,
					NodeName:    stringValue(endpoint.NodeName),

					Ready:       boolValue(endpoint.Conditions.Ready),
					Serving:     boolValue(endpoint.Conditions.Serving),
					Terminating: boolValue(endpoint.Conditions.Terminating),

					Ports: append([]KubeEndpointPort(nil), ports...),
				}

				if endpoint.TargetRef != nil &&
					endpoint.TargetRef.Kind == "Pod" {

					e.PodName = endpoint.TargetRef.Name
					e.PodUID = endpoint.TargetRef.UID

					// Usually the EndpointSlice namespace and Pod
					// namespace are identical, but use TargetRef if
					// Kubernetes supplied it.
					if endpoint.TargetRef.Namespace != "" {
						e.Namespace = endpoint.TargetRef.Namespace
					}
				}

				key := serviceKey(
					slice.Metadata.Namespace,
					serviceName,
				)

				byServiceBackends[key] =
					append(byServiceBackends[key], e)

				if e.IP != "" {
					byEndpointIP[e.IP] = e
				}
			}
		}
	}

	// --------------------------------------------------
	// Nodes (topology: zone + internal IP)
	// --------------------------------------------------

	nodeOut, err := r.fetch("/api/v1/nodes", "get", "nodes", "-o", "json")
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}

	var nodeList kubeNodeList

	if err := json.Unmarshal(nodeOut, &nodeList); err != nil {
		return fmt.Errorf("decode kubernetes node list: %w", err)
	}

	byNodeName := make(map[string]*KubeNode)
	byNodeIP := make(map[string]*KubeNode)

	for _, node := range nodeList.Items {
		n := &KubeNode{
			Name: node.Metadata.Name,
			Zone: node.Metadata.Labels[zoneLabel],
		}

		for _, addr := range node.Status.Addresses {
			if addr.Type == "InternalIP" {
				n.InternalIP = addr.Address
				break
			}
		}

		byNodeName[n.Name] = n

		if n.InternalIP != "" {
			byNodeIP[n.InternalIP] = n
		}
	}

	// --------------------------------------------------
	// Atomically replace metadata snapshot
	// --------------------------------------------------

	r.mu.Lock()

	r.byPodUID = byPodUID
	r.byContainerID = byContainerID
	r.byIP = byIP
	r.byServiceIP = byServiceIP
	r.byServiceBackends = byServiceBackends
	r.byEndpointIP = byEndpointIP
	r.byNodeName = byNodeName
	r.byNodeIP = byNodeIP
	r.lastRefresh = time.Now()

	r.mu.Unlock()

	return nil
}

func (r *KubernetesResolver) ResolvePodUID(uid string) *KubeWorkload {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.byPodUID[uid]
}

func (r *KubernetesResolver) ResolveContainerID(id string) *KubeWorkload {
	id = normalizeContainerID(id)

	r.mu.RLock()
	defer r.mu.RUnlock()

	if w := r.byContainerID[id]; w != nil {
		return w
	}

	for containerID, workload := range r.byContainerID {
		if strings.HasPrefix(containerID, id) ||
			strings.HasPrefix(id, containerID) {
			return workload
		}
	}

	return nil
}

func (r *KubernetesResolver) ResolveIP(ip net.IP) *KubeWorkload {
	if ip == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.byIP[ip.String()]
}

func (r *KubernetesResolver) ResolveServiceIP(ip net.IP) *KubeService {
	if ip == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.byServiceIP[ip.String()]
}

// ResolveEndpointIP resolves an EndpointSlice backend IP.
func (r *KubernetesResolver) ResolveEndpointIP(ip net.IP) *KubeEndpoint {
	if ip == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.byEndpointIP[ip.String()]
}

// ResolveServiceBackends returns all EndpointSlice backends currently
// associated with a Service.
func (r *KubernetesResolver) ResolveServiceBackends(
	service *KubeService,
) []*KubeEndpoint {

	if service == nil {
		return nil
	}

	key := serviceKey(service.Namespace, service.Name)

	r.mu.RLock()
	defer r.mu.RUnlock()

	backends := r.byServiceBackends[key]

	// Return a new slice so callers cannot mutate our cached slice.
	result := make([]*KubeEndpoint, len(backends))
	copy(result, backends)

	return result
}

// ResolveEndpointWorkload converts an EndpointSlice backend into the
// richer Pod/container metadata already maintained by this resolver.
//
// Prefer Pod UID because it is Kubernetes' stable object identity.
// Fall back to backend IP when necessary.
func (r *KubernetesResolver) ResolveEndpointWorkload(
	endpoint *KubeEndpoint,
) *KubeWorkload {

	if endpoint == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if endpoint.PodUID != "" {
		if workload := r.byPodUID[endpoint.PodUID]; workload != nil {
			return workload
		}
	}

	if endpoint.IP != "" {
		if workload := r.byIP[endpoint.IP]; workload != nil {
			return workload
		}
	}

	return nil
}

// Zone returns the topology.kubernetes.io/zone label for a node name, or ""
// if the node is unknown or carries no zone label (common on single-node
// dev/lab clusters, where SAME_AZ/CROSS_AZ can't be meaningfully told
// apart — callers should treat "" as "topology unknown", not "same zone").
func (r *KubernetesResolver) Zone(nodeName string) string {
	if nodeName == "" {
		return ""
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if n := r.byNodeName[nodeName]; n != nil {
		return n.Zone
	}

	return ""
}

// ResolveNodeIP resolves a node's internal IP directly, for traffic aimed
// at the node itself rather than a pod behind it (hostNetwork, NodePort).
func (r *KubernetesResolver) ResolveNodeIP(ip net.IP) *KubeNode {
	if ip == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.byNodeIP[ip.String()]
}
