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
	"os"
	"time"

	_ "net/http/pprof"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/mcuadros/go-syslog.v2"
	"gopkg.in/mcuadros/go-syslog.v2/format"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = time.Now().Format(time.RFC3339)
	builtBy = "unknown"
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
		err = fmt.Errorf("wrong syslog address: %s", conn)
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

func printVersionAndExit() {
	const versionInfo = `
Version: %s
Commit:  %s
Date:    %s
BuiltBy: %s
`
	fmt.Fprintf(os.Stderr, versionInfo, version, commit, date, builtBy)
	os.Exit(0)
}

func main() {
	var (
		metricsAddr  = flag.String("listen-address", ":9292", "ip:port to serve metrics on")
		metricsPath  = flag.String("metrics-endpoint", "/metrics", "URL path to serve metrics on")
		syslogAddr   = flag.String("syslog-listen-address", "udp://0.0.0.0:5145", "proto://ip:port to listen on for the syslog input")
		syslogFormat = flag.String("syslog-format", "rfc3164", "Syslog version to use (rfc3164, rfc5424)")
		versionFlag  = false
	)

	flag.BoolVar(&versionFlag, "V", false, "Print the version and exit")
	flag.BoolVar(&versionFlag, "version", false, "Print the version and exit")

	flag.Parse()

	if versionFlag {
		printVersionAndExit()
	}

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
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewGoCollector(),
		collectors.NewBuildInfoCollector(),
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
