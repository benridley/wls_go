package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"

	"github.com/benridley/wls_go/exporter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v2"
)

// Config represents the main application config
type Config struct {
	Queries exporter.MbeanQuery `yaml:"queries"`
}

func main() {
	configPath := flag.String("config-file", "config.yaml", "Configuration file path")
	flag.Parse()

	configBytes, err := ioutil.ReadFile(*configPath)
	if err != nil {
		log.Fatalf("Failed to read config file: %s", err.Error())
	}

	config := Config{}
	err = yaml.Unmarshal(configBytes, &config)
	if err != nil {
		log.Fatal(err)
	}

	exporter, err := exporter.New(config.Queries)
	if err != nil {
		log.Fatalf("Unable to start exporter: %s", err.Error())
	}

	http.HandleFunc("/probe", func(resp http.ResponseWriter, req *http.Request) {
		probeHandler(resp, req, &exporter)
	})
	log.Fatal(http.ListenAndServe(":9235", nil))
}

func probeHandler(resp http.ResponseWriter, req *http.Request, e *exporter.Exporter) {
	params := req.URL.Query()
	host := params.Get("host")
	port := params.Get("port")

	portInt, err := strconv.Atoi(port)
	if host == "" || port == "" {
		http.Error(resp, "Missing required parameter: Please provide host and port parameters.", 400)
		return
	} else if err != nil {
		http.Error(resp, "Unable to convert port to integer, please provide a valid value for port", 400)
		return
	}

	username, password, ok := req.BasicAuth()
	if !ok {
		http.Error(resp, "Missing authentication information. Please provide basic authentication credentials.", 400)
		return
	}

	probeSuccessGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_success",
		Help: "Displays whether or not the probe was a success",
	})
	registry := prometheus.NewRegistry()
	metrics, err := e.DoQuery(host, portInt, username, password)
	if err != nil {
		probeSuccessGauge.Set(0)
		log.Printf("Failed to probe weblogic instance %s:%s: %v", host, port, err.Error())
		registry.MustRegister(probeSuccessGauge)
	} else {
		probeSuccessGauge.Set(1)
		for _, metric := range metrics {
			registry.MustRegister(metric)
		}
	}

	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(resp, req)
}