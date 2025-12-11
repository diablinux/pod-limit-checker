package analyzer

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"

	"pod-limit-checker/pkg/kubernetes"
)

type PodAnalysis struct {
	Namespace     string
	PodName       string
	ContainerName string
	HasLimits     bool
	HasRequests   bool
	CurrentLimits v1.ResourceList
	CurrentUsage  *ResourceUsage
	Suggestions   []string
	RiskLevel     string
	Age           string
	// Add fields for specific recommendations
	RecommendedCPULimit      string
	RecommendedCPURequest    string
	RecommendedMemoryLimit   string
	RecommendedMemoryRequest string
	ExampleYAML              string
}

type ResourceUsage struct {
	CPU    *resource.Quantity
	Memory *resource.Quantity
}

type PodAnalyzer struct {
	client *kubernetes.Client
}

func NewPodAnalyzer(client *kubernetes.Client) *PodAnalyzer {
	return &PodAnalyzer{client: client}
}

func (a *PodAnalyzer) GetPodsWithoutLimits(ctx context.Context, namespace string) ([]v1.Pod, error) {
	pods, err := a.client.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	return pods.Items, nil
}

func (a *PodAnalyzer) GetPodMetrics(ctx context.Context, namespace string) ([]metricsv1beta1.PodMetrics, error) {
	metrics, err := a.client.MetricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	return metrics.Items, nil
}

func (a *PodAnalyzer) AnalyzePods(pods []v1.Pod, podMetrics []metricsv1beta1.PodMetrics, threshold float64) []PodAnalysis {
	var results []PodAnalysis

	// Create a map of pod metrics for quick lookup
	metricsMap := make(map[string]metricsv1beta1.PodMetrics)
	for _, pm := range podMetrics {
		metricsMap[fmt.Sprintf("%s/%s", pm.Namespace, pm.Name)] = pm
	}

	for _, pod := range pods {
		podAge := duration.ShortHumanDuration(time.Since(pod.CreationTimestamp.Time))

		for _, container := range pod.Spec.Containers {
			analysis := PodAnalysis{
				Namespace:     pod.Namespace,
				PodName:       pod.Name,
				ContainerName: container.Name,
				Age:           podAge,
				CurrentLimits: container.Resources.Limits,
			}

			// Check for limits and requests
			hasLimits := len(container.Resources.Limits) > 0
			hasRequests := len(container.Resources.Requests) > 0

			analysis.HasLimits = hasLimits
			analysis.HasRequests = hasRequests

			// Get current usage from metrics
			if pm, exists := metricsMap[fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)]; exists {
				for _, cm := range pm.Containers {
					if cm.Name == container.Name {
						analysis.CurrentUsage = &ResourceUsage{
							CPU:    cm.Usage.Cpu(),
							Memory: cm.Usage.Memory(),
						}
						break
					}
				}
			}

			// Generate suggestions and specific recommendations
			analysis.Suggestions = a.generateSuggestions(container, analysis.CurrentUsage, threshold)
			analysis.RiskLevel = a.calculateRiskLevel(container, analysis.CurrentUsage)

			// Generate specific recommendations based on actual usage
			a.generateSpecificRecommendations(&analysis, container)

			// Generate example YAML if no limits
			if !analysis.HasLimits && analysis.CurrentUsage != nil {
				analysis.ExampleYAML = a.generateExampleYAML(&analysis, container)
			}

			results = append(results, analysis)
		}
	}

	return results
}

func (a *PodAnalyzer) generateSpecificRecommendations(analysis *PodAnalysis, container v1.Container) {
	// Only provide specific recommendations if we have usage data
	if analysis.CurrentUsage == nil || analysis.CurrentUsage.CPU == nil || analysis.CurrentUsage.Memory == nil {
		return
	}

	cpuUsageMilli := analysis.CurrentUsage.CPU.MilliValue()
	memUsageBytes := analysis.CurrentUsage.Memory.Value()

	// Calculate recommended values based on current usage
	// For limits: 2.5x current usage with minimum values
	// For requests: 1.2x current usage with minimum values

	// CPU recommendations
	recommendedCPULimit := cpuUsageMilli * 5 / 2 // 2.5x
	if recommendedCPULimit < 100 {               // Minimum 100m
		recommendedCPULimit = 100
	}
	analysis.RecommendedCPULimit = fmt.Sprintf("%dm", recommendedCPULimit)

	recommendedCPURequest := cpuUsageMilli * 6 / 5 // 1.2x
	if recommendedCPURequest < 50 {                // Minimum 50m
		recommendedCPURequest = 50
	}
	analysis.RecommendedCPURequest = fmt.Sprintf("%dm", recommendedCPURequest)

	// Memory recommendations (convert to Mi)
	recommendedMemLimit := memUsageBytes * 5 / 2 // 2.5x
	if recommendedMemLimit < 128*1024*1024 {     // Minimum 128Mi
		recommendedMemLimit = 128 * 1024 * 1024
	}
	analysis.RecommendedMemoryLimit = fmt.Sprintf("%dMi", recommendedMemLimit/(1024*1024))

	recommendedMemRequest := memUsageBytes * 6 / 5 // 1.2x
	if recommendedMemRequest < 64*1024*1024 {      // Minimum 64Mi
		recommendedMemRequest = 64 * 1024 * 1024
	}
	analysis.RecommendedMemoryRequest = fmt.Sprintf("%dMi", recommendedMemRequest/(1024*1024))
}

func (a *PodAnalyzer) generateExampleYAML(analysis *PodAnalysis, container v1.Container) string {
	if analysis.CurrentUsage == nil {
		return ""
	}

	return fmt.Sprintf(`        resources:
          limits:
            cpu: "%s"
            memory: "%s"
          requests:
            cpu: "%s"
            memory: "%s"`,
		analysis.RecommendedCPULimit,
		analysis.RecommendedMemoryLimit,
		analysis.RecommendedCPURequest,
		analysis.RecommendedMemoryRequest,
	)
}

func (a *PodAnalyzer) generateSuggestions(container v1.Container, usage *ResourceUsage, threshold float64) []string {
	var suggestions []string

	// Check for missing limits
	if len(container.Resources.Limits) == 0 {
		suggestions = append(suggestions, "❌ No resource limits set")
	}

	// Check for missing requests
	if len(container.Resources.Requests) == 0 {
		suggestions = append(suggestions, "⚠️ No resource requests set")
	}

	// If we have usage data, provide specific suggestions
	if usage != nil && usage.CPU != nil && usage.Memory != nil {
		cpuUsageMilli := usage.CPU.MilliValue()
		memUsageBytes := usage.Memory.Value()

		// CPU suggestions
		if limitCPU, hasCPULimit := container.Resources.Limits[v1.ResourceCPU]; hasCPULimit {
			cpuLimitMilli := limitCPU.MilliValue()

			if cpuLimitMilli > 0 {
				usagePercent := float64(cpuUsageMilli) / float64(cpuLimitMilli) * 100
				if usagePercent > threshold*100 {
					suggestions = append(suggestions,
						fmt.Sprintf("⚠️ CPU usage at %.1f%% of limit, consider increasing limit", usagePercent))
				} else if usagePercent < 30 {
					suggestions = append(suggestions,
						fmt.Sprintf("💡 CPU usage at %.1f%% of limit, consider decreasing limit", usagePercent))
				}
			}
		}

		// Memory suggestions
		if limitMem, hasMemLimit := container.Resources.Limits[v1.ResourceMemory]; hasMemLimit {
			memLimitBytes := limitMem.Value()

			if memLimitBytes > 0 {
				usagePercent := float64(memUsageBytes) / float64(memLimitBytes) * 100
				if usagePercent > threshold*100 {
					suggestions = append(suggestions,
						fmt.Sprintf("⚠️ Memory usage at %.1f%% of limit, consider increasing limit", usagePercent))
				} else if usagePercent < 50 {
					suggestions = append(suggestions,
						fmt.Sprintf("💡 Memory usage at %.1f%% of limit, consider decreasing limit", usagePercent))
				}
			}
		}
	} else if len(container.Resources.Limits) == 0 {
		// No metrics available and no limits
		suggestions = append(suggestions, "📋 Consider setting limits based on application requirements")
	}

	return suggestions
}

func (a *PodAnalyzer) calculateRiskLevel(container v1.Container, usage *ResourceUsage) string {
	// If no limits at all, high risk
	if len(container.Resources.Limits) == 0 {
		return "HIGH"
	}

	// If limits exist but no CPU or memory specifically
	_, hasCPULimit := container.Resources.Limits[v1.ResourceCPU]
	_, hasMemLimit := container.Resources.Limits[v1.ResourceMemory]

	if !hasCPULimit || !hasMemLimit {
		return "MEDIUM"
	}

	// Check if limits are reasonable compared to usage
	if usage != nil && usage.CPU != nil && usage.Memory != nil {
		if limitCPU, ok := container.Resources.Limits[v1.ResourceCPU]; ok {
			cpuUsageMilli := usage.CPU.MilliValue()
			cpuLimitMilli := limitCPU.MilliValue()

			if cpuLimitMilli > 0 && float64(cpuUsageMilli)/float64(cpuLimitMilli) > 0.9 {
				return "MEDIUM" // High usage of limits
			}
		}
	}

	return "LOW"
}
