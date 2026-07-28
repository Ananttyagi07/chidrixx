package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// PriceBook is the overridable rate table from the build manual (§9.1/§12):
// list-price defaults, per cloud/region, swappable for negotiated rates
// without touching code. LastVerified is surfaced in every report so a
// stale price is visible rather than silently trusted.
type PriceBook struct {
	Cloud        string             `yaml:"cloud"`
	Region       string             `yaml:"region"`
	LastVerified string             `yaml:"last_verified"`
	Rates        map[string]float64 `yaml:"rates"`
	FX           struct {
		UsdToInr float64 `yaml:"usd_to_inr"`
	} `yaml:"fx"`
}

// pathClassToRateKey maps a PathClass to its price-book rate key.
var pathClassToRateKey = map[PathClass]string{
	PathSameNode:          "same_node",
	PathSameAZ:            "same_az",
	PathCrossAZ:           "cross_az",
	PathCrossRegion:       "cross_region",
	PathManagedService:    "managed_service",
	PathPrivateOffCluster: "private_offcluster",
	PathNATEgress:         "nat_egress",
	PathInternetEgress:    "internet_egress",
}

// LoadPriceBook reads a YAML price book from path.
func LoadPriceBook(path string) (*PriceBook, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read price book %s: %w", path, err)
	}

	var pb PriceBook
	if err := yaml.Unmarshal(data, &pb); err != nil {
		return nil, fmt.Errorf("parse price book %s: %w", path, err)
	}

	return &pb, nil
}

// CostINR converts bytes moved on a classified path into a [low, high] INR
// range, widening the band around lower-confidence classifications instead
// of stating a false-precision single number (build manual §12, "never a
// false-precision single number").
func (pb *PriceBook) CostINR(class PathClass, confidence string, bytes uint64) (low, high float64) {
	rateUSD := pb.Rates[pathClassToRateKey[class]]
	gb := float64(bytes) / 1e9
	inr := gb * rateUSD * pb.FX.UsdToInr

	band := 0.15
	switch confidence {
	case ConfMed:
		band = 0.35
	case ConfLow:
		band = 0.60
	}

	return inr * (1 - band), inr * (1 + band)
}
