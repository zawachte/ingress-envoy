package controllers

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	ingressHitsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ingress_hits_total",
		Help: "The total number of processed events",
	})
)

func init() {
	metrics.Registry.MustRegister(ingressHitsProcessed)
}
