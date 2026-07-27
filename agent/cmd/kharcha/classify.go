package main

import "net"

type PathClass string

const (
	PathLoopback PathClass = "LOOPBACK" // same machine — free (stands in for SAME_NODE)
	PathLAN      PathClass = "LAN"      // private network — free (stands in for SAME_AZ)
	PathInternet PathClass = "INTERNET" // public internet — real cost (INTERNET_EGRESS)
)

// Classify mirrors the LLD's decision tree, scoped to laptop-visible signals:
// no K8s zones exist here, so IP range stands in for cluster topology.
func Classify(remote net.IP) PathClass {
	if remote == nil {
		return PathInternet
	}
	if remote.IsLoopback() {
		return PathLoopback
	}
	if isPrivate(remote) {
		return PathLAN
	}
	return PathInternet
}

func isPrivate(ip net.IP) bool {
	for _, cidr := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(ip) {
			return true
		}
	}
	return false
}
