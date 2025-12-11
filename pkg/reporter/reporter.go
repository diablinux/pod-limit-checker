package reporter

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v2"

	"pod-limit-checker/pkg/analyzer"

	v1 "k8s.io/api/core/v1"
)

type Reporter struct {
	format       string
	verbose      bool
	showExamples bool
}

func NewReporter(format string) *Reporter {
	return &Reporter{format: format, showExamples: true}
}

func (r *Reporter) SetVerbose(verbose bool) {
	r.verbose = verbose
}

func (r *Reporter) SetShowExamples(showExamples bool) {
	r.showExamples = showExamples
}

func (r *Reporter) GenerateReport(results []analyzer.PodAnalysis, showAll bool) error {
	// Filter results if not showing all
	filteredResults := results
	if !showAll {
		filteredResults = []analyzer.PodAnalysis{}
		for _, result := range results {
			if !result.HasLimits || result.RiskLevel == "HIGH" || result.RiskLevel == "MEDIUM" {
				filteredResults = append(filteredResults, result)
			}
		}
	}

	switch strings.ToLower(r.format) {
	case "json":
		return r.generateJSON(filteredResults)
	case "yaml":
		return r.generateYAML(filteredResults)
	case "table":
		fallthrough
	default:
		return r.generateTable(filteredResults)
	}
}

func (r *Reporter) generateTable(results []analyzer.PodAnalysis) error {
	if len(results) == 0 {
		fmt.Println("✅ All pods have proper resource limits configured.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)

	if r.verbose {
		// Verbose mode - detailed output
		for i, result := range results {
			if i > 0 {
				fmt.Println()
			}
			r.printPodDetails(&result, w)
		}
	} else {
		// Compact mode - just the table
		fmt.Fprintln(w, "NAMESPACE\tPOD\tCONTAINER\tAGE\tLIMITS\tREQUESTS\tRISK\tSUGGESTIONS")
		fmt.Fprintln(w, "---------\t---\t---------\t---\t------\t--------\t----\t----------")

		for _, result := range results {
			limitsStr := "None"
			if len(result.CurrentLimits) > 0 {
				var limitParts []string
				if cpu, ok := result.CurrentLimits[v1.ResourceCPU]; ok {
					limitParts = append(limitParts, fmt.Sprintf("CPU:%s", cpu.String()))
				}
				if mem, ok := result.CurrentLimits[v1.ResourceMemory]; ok {
					limitParts = append(limitParts, fmt.Sprintf("Mem:%s", mem.String()))
				}
				limitsStr = strings.Join(limitParts, ", ")
			}

			requestsStr := "No"
			if result.HasRequests {
				requestsStr = "Yes"
			}

			// Get first suggestion or empty
			suggestion := ""
			if len(result.Suggestions) > 0 {
				suggestion = result.Suggestions[0]
				if len(result.Suggestions) > 1 {
					suggestion += fmt.Sprintf(" (+%d more)", len(result.Suggestions)-1)
				}
			}

			riskIcon := "✅"
			switch result.RiskLevel {
			case "HIGH":
				riskIcon = "🔴"
			case "MEDIUM":
				riskIcon = "🟡"
			case "LOW":
				riskIcon = "🟢"
			}

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s%s\t%s\n",
				result.Namespace,
				result.PodName,
				result.ContainerName,
				result.Age,
				limitsStr,
				requestsStr,
				riskIcon,
				result.RiskLevel,
				suggestion,
			)
		}
	}

	w.Flush()

	// Print summary
	r.printSummary(results)

	// Print examples for pods without limits (if requested and we have usage data)
	if r.showExamples {
		r.printSpecificExamples(results)
	}

	if !r.verbose && len(results) > 0 {
		fmt.Printf("\n💡 Tip: Use --verbose flag to see detailed recommendations\n")
	}

	return nil
}

func (r *Reporter) printPodDetails(result *analyzer.PodAnalysis, w *tabwriter.Writer) {
	fmt.Printf("📦 Pod: %s/%s\n", result.Namespace, result.PodName)
	fmt.Printf("  Container: %s (Age: %s)\n", result.ContainerName, result.Age)

	// Current configuration
	fmt.Printf("  Current configuration:\n")
	if len(result.CurrentLimits) > 0 {
		fmt.Printf("    Limits:\n")
		if cpu, ok := result.CurrentLimits[v1.ResourceCPU]; ok {
			fmt.Printf("      CPU: %s\n", cpu.String())
		} else {
			fmt.Printf("      CPU: ❌ Not set\n")
		}
		if mem, ok := result.CurrentLimits[v1.ResourceMemory]; ok {
			fmt.Printf("      Memory: %s\n", mem.String())
		} else {
			fmt.Printf("      Memory: ❌ Not set\n")
		}
	} else {
		fmt.Printf("    Limits: ❌ None\n")
	}

	if result.HasRequests {
		fmt.Printf("    Requests: ✅ Set\n")
	} else {
		fmt.Printf("    Requests: ⚠️ Not set\n")
	}

	// Current usage if available
	if result.CurrentUsage != nil && result.CurrentUsage.CPU != nil && result.CurrentUsage.Memory != nil {
		fmt.Printf("  Current usage:\n")
		fmt.Printf("    CPU: %s\n", result.CurrentUsage.CPU.String())
		fmt.Printf("    Memory: %s\n", result.CurrentUsage.Memory.String())
	}

	// Risk level
	riskIcon := "✅"
	switch result.RiskLevel {
	case "HIGH":
		riskIcon = "🔴"
	case "MEDIUM":
		riskIcon = "🟡"
	case "LOW":
		riskIcon = "🟢"
	}
	fmt.Printf("  Risk level: %s%s\n", riskIcon, result.RiskLevel)

	// Suggestions
	if len(result.Suggestions) > 0 {
		fmt.Printf("  Suggestions:\n")
		for _, suggestion := range result.Suggestions {
			fmt.Printf("    - %s\n", suggestion)
		}
	}

	// Specific recommendations if we have usage data
	if result.RecommendedCPULimit != "" && result.RecommendedMemoryLimit != "" {
		fmt.Printf("  Recommended limits (based on current usage):\n")
		fmt.Printf("    CPU: %s (request: %s)\n",
			result.RecommendedCPULimit, result.RecommendedCPURequest)
		fmt.Printf("    Memory: %s (request: %s)\n",
			result.RecommendedMemoryLimit, result.RecommendedMemoryRequest)

		if result.ExampleYAML != "" {
			fmt.Printf("  Example YAML to add to container spec:\n")
			fmt.Printf("%s\n", result.ExampleYAML)
		}
	}
}

func (r *Reporter) printSummary(results []analyzer.PodAnalysis) {
	fmt.Printf("\n📊 Summary:\n")
	fmt.Printf("  Total containers analyzed: %d\n", len(results))

	highRisk := 0
	mediumRisk := 0
	lowRisk := 0
	noLimits := 0
	noRequests := 0
	withUsageData := 0

	for _, result := range results {
		switch result.RiskLevel {
		case "HIGH":
			highRisk++
		case "MEDIUM":
			mediumRisk++
		case "LOW":
			lowRisk++
		}
		if !result.HasLimits {
			noLimits++
		}
		if !result.HasRequests {
			noRequests++
		}
		if result.CurrentUsage != nil && result.CurrentUsage.CPU != nil && result.CurrentUsage.Memory != nil {
			withUsageData++
		}
	}

	fmt.Printf("  🔴 High risk (no limits): %d\n", highRisk)
	fmt.Printf("  🟡 Medium risk: %d\n", mediumRisk)
	fmt.Printf("  🟢 Low risk: %d\n", lowRisk)
	fmt.Printf("  ❌ No limits set: %d\n", noLimits)
	fmt.Printf("  ⚠️  No requests set: %d\n", noRequests)
	fmt.Printf("  📊 With usage metrics: %d\n", withUsageData)
}

func (r *Reporter) printSpecificExamples(results []analyzer.PodAnalysis) {
	// Only show examples for pods that actually need them (no limits and have usage data)
	podsNeedingExamples := []*analyzer.PodAnalysis{}
	for i := range results {
		if !results[i].HasLimits && results[i].CurrentUsage != nil &&
			results[i].CurrentUsage.CPU != nil && results[i].CurrentUsage.Memory != nil {
			podsNeedingExamples = append(podsNeedingExamples, &results[i])
		}
	}

	if len(podsNeedingExamples) > 0 {
		fmt.Printf("\n🔧 Specific fixes for pods without limits (based on current usage):\n")
		for _, result := range podsNeedingExamples {
			fmt.Printf("\n  %s/%s/%s:\n",
				result.Namespace, result.PodName, result.ContainerName)
			fmt.Printf("    Current CPU usage: %s → Suggested: limit=%s, request=%s\n",
				result.CurrentUsage.CPU.String(),
				result.RecommendedCPULimit,
				result.RecommendedCPURequest)
			fmt.Printf("    Current memory usage: %s → Suggested: limit=%s, request=%s\n",
				result.CurrentUsage.Memory.String(),
				result.RecommendedMemoryLimit,
				result.RecommendedMemoryRequest)
		}
	}
}

func (r *Reporter) generateJSON(results []analyzer.PodAnalysis) error {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func (r *Reporter) generateYAML(results []analyzer.PodAnalysis) error {
	data, err := yaml.Marshal(results)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
