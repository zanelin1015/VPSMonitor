package client

import (
	"time"

	"bridge-core/internal/model"
)

type systemMetricsSampler struct {
	lastCPU     cpuCounters
	hasCPU      bool
	lastNet     netCounters
	hasNet      bool
	lastNetTime time.Time
}

type cpuCounters struct {
	idle  uint64
	total uint64
}

type netCounters struct {
	rx uint64
	tx uint64
}

func newSystemMetricsSampler() *systemMetricsSampler {
	sampler := &systemMetricsSampler{}
	if cpu, ok := readCPUCounters(); ok {
		sampler.lastCPU = cpu
		sampler.hasCPU = true
	}
	if net, ok := readNetCounters(); ok {
		sampler.lastNet = net
		sampler.hasNet = true
		sampler.lastNetTime = time.Now()
	}
	return sampler
}

func (s *systemMetricsSampler) sample() model.VPSSummary {
	return sampleSystemMetrics(s)
}
