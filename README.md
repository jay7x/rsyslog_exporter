# rsyslog prometheus exporter

This software acts like a syslog server and prometheus exporter at the same
time. It listens on a network socket for `impstats` syslog messages and exposes
them as a prometheus metrics.

NOTE: This exporter is different from
[soundcloud](https://github.com/soundcloud/rsyslog_exporter) kind of exporters.
Metrics are exposed in different format (closer to the `impstats`).

## How to setup

1. Download the `rsyslog_exporter` binary (TBD) and put it to the `/usr/local/bin/rsyslog_exporter` path e.g.

2. Run `rsyslog_exporter`. By default it'll listen for RFC3164 syslog messages on UDP port 5145 on all interfaces. Prometheus metrics will be exposed on port 9292.

3. Configure `impstats` to send counters data as syslog messages to the rsyslog_exporter syslog listener:

```
module(load="impstats"
  interval="60"
  resetCounters="off"
  format="json"
  ruleset="stats"
)

ruleset(name="stats") {
  action(type="omfwd" name="stats_fwd"
    target="127.0.0.1"
    port="5145"
    protocol="udp"
  )
}
```

3. Check rsyslog configuration systax by running `rsyslogd -N 1`

4. Restart rsyslog if no errors found (`systemctl restart rsyslog` e.g.)

5. Go to <http://localhost:9292/metrics> to see metrics

## Command-line parameters

```
  -listen-address string
      IP:port at which to serve metrics (default ":9292")
  -metrics-endpoint string
      URL path at which to serve metrics (default "/metrics")
  -syslog-format string
      Which syslog version to use (rfc3164, rfc5424) (default "rfc3164")
  -syslog-listen-address string
      Where to serve syslog input (default "udp://0.0.0.0:5145")
```

## TODO

- add custom global labels
- support collecting metrics from multiple rsyslog instances
- better tests
- better errors
