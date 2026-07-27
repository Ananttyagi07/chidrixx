package main

var ratePerGB = map[PathClass]float64{
	PathLoopback: 0.0,
	PathLAN:      0.0,
	PathInternet: 7.50,
}

func CostINR(class PathClass, bytes uint64) float64 {
	return (float64(bytes) / 1e9) * ratePerGB[class]
}
