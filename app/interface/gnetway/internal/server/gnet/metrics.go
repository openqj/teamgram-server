//  Copyright (c) 2026 The Teamgram Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gnet

import "github.com/zeromicro/go-zero/core/metric"

const namespace = "gnetway"

var (
	metricConnOpen = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: namespace,
		Subsystem: "conn",
		Name:      "open_total",
		Help:      "Total number of connections opened.",
		Labels:    []string{"proto"}, // tcp, websocket
	})

	metricConnClose = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: namespace,
		Subsystem: "conn",
		Name:      "close_total",
		Help:      "Total number of connections closed.",
		Labels:    []string{"proto"},
	})

	metricConnActive = metric.NewGaugeVec(&metric.GaugeVecOpts{
		Namespace: namespace,
		Subsystem: "conn",
		Name:      "active",
		Help:      "Current number of open connections.",
		Labels:    []string{"proto"},
	})

	metricConnPending = metric.NewGaugeVec(&metric.GaugeVecOpts{
		Namespace: namespace,
		Subsystem: "conn",
		Name:      "pending",
		Help:      "Connections accepted by the socket but not yet assigned a transport.",
		Labels:    []string{},
	})

	metricAdmissionRejected = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: namespace,
		Subsystem: "admission",
		Name:      "rejected_total",
		Help:      "Connections or authenticated sessions rejected by a resource limit.",
		Labels:    []string{"reason"},
	})

	metricHandshakeInFlight = metric.NewGaugeVec(&metric.GaugeVecOpts{
		Namespace: namespace,
		Subsystem: "handshake",
		Name:      "in_flight",
		Help:      "Handshake entrypoints currently holding an admission slot.",
		Labels:    []string{},
	})

	metricHandshakeRejected = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: namespace,
		Subsystem: "handshake",
		Name:      "rejected_total",
		Help:      "Handshake attempts rejected by rate or concurrency limits.",
		Labels:    []string{"reason"},
	})

	metricWorkerRejected = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: namespace,
		Subsystem: "worker",
		Name:      "rejected_total",
		Help:      "Asynchronous gateway tasks rejected because the bounded worker pool is full.",
		Labels:    []string{},
	})

	metricDrain = metric.NewGaugeVec(&metric.GaugeVecOpts{
		Namespace: namespace,
		Subsystem: "server",
		Name:      "draining",
		Help:      "Whether the gateway is draining connections for graceful shutdown.",
		Labels:    []string{},
	})

	metricConnTimeout = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: namespace,
		Subsystem: "conn",
		Name:      "timeout_total",
		Help:      "Total number of connections closed by timeout.",
		Labels:    []string{},
	})

	metricHandshake = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: namespace,
		Subsystem: "handshake",
		Name:      "total",
		Help:      "Total number of handshake attempts.",
		Labels:    []string{"result"}, // ok, error
	})

	metricMsgProcess = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: namespace,
		Subsystem: "msg",
		Name:      "process_duration_ms",
		Help:      "Message processing duration in milliseconds.",
		Labels:    []string{"type"}, // encrypted, unencrypted
		Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 5000},
	})

	metricQuickAck = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: namespace,
		Subsystem: "msg",
		Name:      "quick_ack_total",
		Help:      "Total number of quick ACK responses sent.",
		Labels:    []string{},
	})

	metricCodecDecodeError = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: namespace,
		Subsystem: "codec",
		Name:      "decode_errors_total",
		Help:      "Total number of MTProto codec decode errors, labeled by transport and reason.",
		Labels:    []string{"proto", "reason"}, // proto: tcp, websocket; reason: bad_magic, bad_len, bad_crc, bad_seq, decrypt, transport_unsupported, unexpected_eof, other
	})

	metricGatewayWriteQueueWait = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: namespace,
		Subsystem: "gateway_write",
		Name:      "queue_wait_ms",
		Help:      "Time spent waiting in a connection's bounded gateway write queue.",
		Labels:    []string{"proto"},
		Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2000, 5000, 10000},
	})

	metricGatewayWriteRejected = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: namespace,
		Subsystem: "gateway_write",
		Name:      "rejected_total",
		Help:      "Gateway writes rejected because the per-connection queue was unavailable or full.",
		Labels:    []string{"reason"},
	})
	metricGatewayWriteFailed = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: namespace,
		Subsystem: "gateway_write",
		Name:      "failed_total",
		Help:      "Gateway write batches that failed and caused the connection to close.",
		Labels:    []string{"reason"},
	})
	metricGatewayWriteFlushDuration = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: namespace,
		Subsystem: "gateway_write",
		Name:      "flush_duration_ms",
		Help:      "Time spent encoding and submitting one gateway write batch on the gnet event loop.",
		Labels:    []string{"proto"},
		Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 25, 50, 100, 250, 500, 1000},
	})
	metricGatewayWriteSocketBuffered = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: namespace,
		Subsystem: "gateway_write",
		Name:      "socket_buffered_bytes",
		Help:      "Outbound bytes remaining in the gnet socket buffer after a gateway write batch.",
		Labels:    []string{"proto"},
		Buckets:   []float64{0, 128, 512, 1024, 4096, 16384, 65536, 262144, 1048576, 4194304, 16777216},
	})
)
