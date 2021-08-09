/*
 * Export rsyslog counters as prometheus metrics
 *
 * Copyright (c) 2021, Yury Bushmelev <jay4mail@gmail.com>
 * All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"

	_ "net/http/pprof"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/mcuadros/go-syslog.v2"
	"gopkg.in/mcuadros/go-syslog.v2/format"
)

// Init syslog server
func syslogServerInit(syslogFormat string, conn string) (*syslog.Server, syslog.LogPartsChannel, error) {
	channel := make(syslog.LogPartsChannel)
	handler := syslog.NewChannelHandler(channel)
	server := syslog.NewServer()

	var format format.Format

	switch syslogFormat {
	case "rfc3164":
		format = syslog.RFC3164
	case "rfc5424":
		format = syslog.RFC5424
	default:
		return nil, nil, fmt.Errorf("format %s is not supported", syslogFormat)
	}

	server.SetFormat(format)
	server.SetHandler(handler)

	url, err := url.Parse(conn)
	if err != nil {
		return nil, nil, err
	}

	switch url.Scheme {
	case "udp":
		err = server.ListenUDP(url.Host)
	case "tcp":
		err = server.ListenTCP(url.Host)
	default:
		err = fmt.Errorf("Wrong syslog address: %s", conn)
	}
	if err != nil {
		return nil, nil, err
	}

	err = server.Boot()
	if err != nil {
		return nil, nil, err
	}
	return server, channel, nil
}

func processSyslogMessages(rs *RsyslogStats, channel syslog.LogPartsChannel) {
	for line := range channel {
		rs.Parse(line["content"].(string))
	}
}

// Collector
type RsyslogStatsCollector struct {
	RS *RsyslogStats
}

func NewRsyslogStatsCollector(rs *RsyslogStats) *RsyslogStatsCollector {
	return &RsyslogStatsCollector{RS: rs}
}

func (rsc *RsyslogStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	log.Print("-- Describing metrics...")
}

func (rsc *RsyslogStatsCollector) Collect(ch chan<- prometheus.Metric) {
	log.Print("-- Collecting metrics...")
	var mType prometheus.ValueType
	rsc.RS.RLock()
	for metricName, labeledValues := range rsc.RS.Metrics {
		for labels, value := range labeledValues {
			//log.Printf("%s{\"%s\"=\"%s\"} %d", metricName, labels.Name, labels.Value, value)
			switch metricName {
			case "rsyslog_core_queue_size":
				mType = prometheus.GaugeValue
			default:
				mType = prometheus.CounterValue
			}
			desc := prometheus.NewDesc(metricName, "", []string{labels.Name}, nil)
			ch <- prometheus.MustNewConstMetric(desc, mType, float64(value), labels.Value)
		}
	}
	rsc.RS.RUnlock()

	// export internal counters
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(
			"rsyslog_exporter_parser_failures",
			"Amount of rsyslog stats parsing failures",
			nil, nil,
		),
		prometheus.CounterValue,
		float64(rsc.RS.ParserFailures),
	)

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(
			"rsyslog_exporter_parsed_messages",
			"Amount of rsyslog stat messages parsed",
			nil, nil,
		),
		prometheus.CounterValue,
		float64(rsc.RS.ParsedMessages),
	)

	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(
			"rsyslog_exporter_parse_timestamp",
			"Latest parse Unix timestamp",
			nil, nil,
		),
		prometheus.CounterValue,
		float64(rsc.RS.ParseTimestamp),
	)
}

func main() {
	var metricsAddr = flag.String("listen-address", ":9292", "IP:port at which to serve metrics")
	var metricsPath = flag.String("metrics-endpoint", "/metrics", "URL path at which to serve metrics")
	var syslogAddr = flag.String("syslog-listen-address", "udp://0.0.0.0:5145", "Where to serve syslog input")
	var syslogFormat = flag.String("syslog-format", "rfc3164", "Which syslog version to use (rfc3164, rfc5424)")
	flag.Parse()

	_, channel, err := syslogServerInit(*syslogFormat, *syslogAddr)
	if err != nil {
		log.Fatal(err)
	}

	// RsyslogStats structure
	rs := NewRsyslogStats()

	// RsyslogStatsCollector
	rsc := NewRsyslogStatsCollector(rs)

	// Prometheus registry
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		prometheus.NewGoCollector(),
		prometheus.NewBuildInfoCollector(),
		rsc,
	)

	// Expose the registered metrics via HTTP.
	http.Handle(*metricsPath, promhttp.HandlerFor(
		reg,
		promhttp.HandlerOpts{
			// Opt into OpenMetrics to support exemplars.
			EnableOpenMetrics: true,
		},
	))

	// Read and print syslog messages
	go processSyslogMessages(rs, channel)

	// start prometheus web-server
	log.Fatal(http.ListenAndServe(*metricsAddr, nil))
}
