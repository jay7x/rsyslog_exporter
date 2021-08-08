/*
 * bla bla bla
 */

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/mcuadros/go-syslog.v2"
	"gopkg.in/mcuadros/go-syslog.v2/format"
)

func syslog_server_init(syslog_format string, conn string) (*syslog.Server, syslog.LogPartsChannel, error) {
	channel := make(syslog.LogPartsChannel)
	handler := syslog.NewChannelHandler(channel)

	server := syslog.NewServer()

	var format format.Format

	switch syslog_format {
	case "rfc3164":
		format = syslog.RFC3164
	case "rfc5424":
		format = syslog.RFC5424
	default:
		return nil, nil, fmt.Errorf("Format %s is not supported!", syslog_format)
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

func process_syslog_messages(rs *RsyslogStats, channel syslog.LogPartsChannel) {
	for line := range channel {
		err := rs.Parse(line["content"].(string))
		if err != nil {
			//log.Println(err)
		}
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
	log.Print("Describing metrics...")
}

func (rsc *RsyslogStatsCollector) Collect(ch chan<- prometheus.Metric) {
	log.Print("Collecting metrics...")
	for metric_name, labeled_values := range rsc.RS.Current {
		for labels, value := range labeled_values {
			log.Printf("%s{name=\"%s\",counter=\"%s\"} %d", metric_name, labels.Name, labels.Counter, value)
		}
	}

	// export internal counters
	// iterate over rs.Current to generate metrics
}

func main() {
	var metrics_addr = flag.String("listen-address", ":9292", "IP:port at which to serve metrics")
	var metrics_path = flag.String("metrics-endpoint", "/metrics", "URL path at which to serve metrics")
	var syslog_addr = flag.String("syslog-listen-address", "udp://0.0.0.0:5145", "Where to serve syslog input")
	var syslog_format = flag.String("syslog-format", "rfc3164", "Which syslog version to use (rfc3164, rfc5424)")
	flag.Parse()

	_, channel, err := syslog_server_init(*syslog_format, *syslog_addr)
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
	http.Handle(*metrics_path, promhttp.HandlerFor(
		reg,
		promhttp.HandlerOpts{
			// Opt into OpenMetrics to support exemplars.
			EnableOpenMetrics: true,
		},
	))

	// Read and print syslog messages
	go process_syslog_messages(rs, channel)

	// start prometheus web-server
	log.Fatal(http.ListenAndServe(*metrics_addr, nil))
}
