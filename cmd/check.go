package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"pod-limit-checker/pkg/analyzer"
	"pod-limit-checker/pkg/kubernetes"
	"pod-limit-checker/pkg/reporter"
)

var (
	kubeconfig string
	output     string
	threshold  float64
	showAll    bool
	namespace  string
	verbose    bool
	noExamples bool // Add this
)

func Execute() error {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	flag.StringVar(&output, "output", "table", "output format: table, json, yaml")
	flag.Float64Var(&threshold, "threshold", 0.8, "usage threshold for suggestions (0.0-1.0)")
	flag.BoolVar(&showAll, "all", false, "show all pods including those with limits")
	flag.StringVar(&namespace, "namespace", "", "specific namespace to check (default: all namespaces)")
	flag.BoolVar(&verbose, "verbose", false, "show all suggestions in table output")
	flag.BoolVar(&noExamples, "no-examples", false, "don't show example YAML fixes") // Add this
	flag.Parse()

	// Initialize Kubernetes client
	client, err := kubernetes.NewClient(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %v", err)
	}

	// Create analyzer
	podAnalyzer := analyzer.NewPodAnalyzer(client)

	// Set up context
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get pods without limits
	pods, err := podAnalyzer.GetPodsWithoutLimits(ctx, namespace)
	if err != nil {
		return fmt.Errorf("failed to get pods: %v", err)
	}

	// Get usage metrics
	fmt.Println("Fetching pod metrics...")
	podMetrics, err := podAnalyzer.GetPodMetrics(ctx, namespace)
	if err != nil {
		fmt.Printf("Warning: Could not fetch metrics: %v\n", err)
		fmt.Println("Continuing without metric-based suggestions...")
	}

	// Analyze pods and generate suggestions
	results := podAnalyzer.AnalyzePods(pods, podMetrics, threshold)

	// Create reporter and generate output
	rep := reporter.NewReporter(output)
	rep.SetVerbose(verbose)
	rep.SetShowExamples(!noExamples) // Set based on flag
	if err := rep.GenerateReport(results, showAll); err != nil {
		return fmt.Errorf("failed to generate report: %v", err)
	}

	return nil
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
